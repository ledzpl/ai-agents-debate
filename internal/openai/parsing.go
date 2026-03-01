package openai

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode"

	"debate/internal/orchestrator"
)

func toUsage(u apiUsage) orchestrator.Usage {
	return orchestrator.Usage{
		PromptTokens:     u.InputTokens,
		CompletionTokens: u.OutputTokens,
		TotalTokens:      u.TotalTokens,
	}
}

func extractOutputText(resp responseBody) string {
	if strings.TrimSpace(resp.OutputText) != "" {
		return strings.TrimSpace(resp.OutputText)
	}

	parts := make([]string, 0, len(resp.Output))
	for _, item := range resp.Output {
		if strings.TrimSpace(item.Text) != "" {
			parts = append(parts, strings.TrimSpace(item.Text))
		}
		for _, content := range item.Content {
			if strings.TrimSpace(content.Text) != "" {
				parts = append(parts, strings.TrimSpace(content.Text))
			}
		}
	}
	return strings.TrimSpace(strings.Join(parts, "\n"))
}

func parseConsensus(raw string) (orchestrator.Consensus, error) {
	cleaned := strings.TrimSpace(raw)
	if cleaned == "" {
		return orchestrator.Consensus{}, errors.New("empty consensus output")
	}

	cleaned = stripCodeFence(cleaned)
	candidates := extractJSONObjectCandidates(cleaned)
	if len(candidates) == 0 {
		candidates = []string{cleaned}
	}
	var firstErr error
	for _, jsonText := range candidates {
		consensus, err := parseConsensusObject(jsonText)
		if err == nil {
			return consensus, nil
		}
		if firstErr == nil {
			firstErr = err
		}
	}
	if firstErr != nil {
		return orchestrator.Consensus{}, fmt.Errorf("failed to parse consensus JSON: %w", firstErr)
	}
	return orchestrator.Consensus{}, errors.New("failed to parse consensus JSON")
}

func parseConsensusObject(jsonText string) (orchestrator.Consensus, error) {
	var rawMap map[string]json.RawMessage
	if err := json.Unmarshal([]byte(jsonText), &rawMap); err != nil {
		return orchestrator.Consensus{}, err
	}

	requiredKeys := []string{"reached", "score", "summary", "rationale", "open_risks", "required_next_action"}
	for _, key := range requiredKeys {
		if _, ok := rawMap[key]; !ok {
			return orchestrator.Consensus{}, errors.New("missing required consensus key: " + key)
		}
	}

	var parsed struct {
		Reached            bool            `json:"reached"`
		Score              float64         `json:"score"`
		Summary            string          `json:"summary"`
		Rationale          string          `json:"rationale"`
		OpenRisks          json.RawMessage `json:"open_risks"`
		RequiredNextAction string          `json:"required_next_action"`
	}
	if err := json.Unmarshal([]byte(jsonText), &parsed); err != nil {
		return orchestrator.Consensus{}, err
	}

	parsed.Summary = strings.TrimSpace(parsed.Summary)
	parsed.Rationale = strings.TrimSpace(parsed.Rationale)
	parsed.RequiredNextAction = strings.TrimSpace(parsed.RequiredNextAction)
	parsed.Score = clamp(parsed.Score, 0, 1)
	openRisks, err := parseOpenRisks(parsed.OpenRisks)
	if err != nil {
		return orchestrator.Consensus{}, err
	}

	if parsed.Summary == "" {
		return orchestrator.Consensus{}, errors.New("summary is required")
	}
	if parsed.RequiredNextAction == "" {
		return orchestrator.Consensus{}, errors.New("required_next_action is required")
	}

	return orchestrator.Consensus{
		Reached:            parsed.Reached,
		Score:              parsed.Score,
		Summary:            parsed.Summary,
		Rationale:          parsed.Rationale,
		OpenRisks:          openRisks,
		RequiredNextAction: parsed.RequiredNextAction,
	}, nil
}

func parseOpenRisks(raw json.RawMessage) ([]string, error) {
	if len(raw) == 0 {
		return nil, nil
	}

	rawText := strings.TrimSpace(string(raw))
	if rawText == "" || rawText == "null" {
		return nil, nil
	}

	var list []string
	if err := json.Unmarshal(raw, &list); err == nil {
		return normalizeNonEmpty(list), nil
	}

	var single string
	if err := json.Unmarshal(raw, &single); err == nil {
		single = strings.TrimSpace(single)
		if single == "" {
			return nil, nil
		}
		parts := strings.Split(single, ",")
		if len(parts) == 1 {
			parts = strings.Split(single, ";")
		}
		return normalizeNonEmpty(parts), nil
	}
	return nil, errors.New("open_risks must be an array or string")
}

func normalizeNonEmpty(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	for _, v := range values {
		trimmed := strings.TrimSpace(v)
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func parseOpeningSpeakerID(raw string) (string, error) {
	cleaned := strings.TrimSpace(raw)
	if cleaned == "" {
		return "", errors.New("empty opening speaker output")
	}

	cleaned = stripCodeFence(cleaned)
	candidates := extractJSONObjectCandidates(cleaned)
	if len(candidates) == 0 && strings.HasPrefix(strings.TrimSpace(cleaned), "{") {
		candidates = []string{cleaned}
	}
	for _, jsonText := range candidates {
		var payload struct {
			PersonaID  string `json:"persona_id"`
			PersonaID2 string `json:"personaId"`
		}
		if err := json.Unmarshal([]byte(jsonText), &payload); err == nil {
			id := strings.TrimSpace(payload.PersonaID)
			if id == "" {
				id = strings.TrimSpace(payload.PersonaID2)
			}
			if id != "" {
				return id, nil
			}
		}
	}

	firstLine := cleaned
	if idx := strings.IndexByte(firstLine, '\n'); idx >= 0 {
		firstLine = firstLine[:idx]
	}
	firstLine = strings.Trim(firstLine, " \t\r\n\"'`")
	if firstLine == "" {
		return "", errors.New("persona_id is required")
	}
	if strings.ContainsAny(firstLine, " \t") {
		return "", errors.New("persona_id is required")
	}
	return firstLine, nil
}

func stripCodeFence(s string) string {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "```") {
		return s
	}

	lines := strings.Split(s, "\n")
	if len(lines) == 1 {
		line := strings.TrimSpace(strings.TrimPrefix(lines[0], "```"))
		if end := strings.LastIndex(line, "```"); end >= 0 {
			line = line[:end]
		}
		line = strings.TrimSpace(line)
		fields := strings.Fields(line)
		if len(fields) >= 2 && looksLikeFenceLanguage(fields[0]) {
			return strings.TrimSpace(strings.TrimPrefix(line, fields[0]))
		}
		return line
	}

	first := strings.TrimSpace(lines[0])
	if strings.HasPrefix(first, "```") {
		lines = lines[1:]
	}
	if len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "```" {
		lines = lines[:len(lines)-1]
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func extractJSONObject(s string) string {
	candidates := extractJSONObjectCandidates(s)
	if len(candidates) == 0 {
		return ""
	}
	return candidates[0]
}

func extractJSONObjectCandidates(s string) []string {
	start := -1
	depth := 0
	inString := false
	escaped := false
	candidates := make([]string, 0, 2)

	for i := 0; i < len(s); i++ {
		ch := s[i]

		if inString {
			if escaped {
				escaped = false
				continue
			}
			if ch == '\\' {
				escaped = true
				continue
			}
			if ch == '"' {
				inString = false
			}
			continue
		}

		switch ch {
		case '"':
			inString = true
		case '{':
			if depth == 0 {
				start = i
			}
			depth++
		case '}':
			if depth == 0 {
				continue
			}
			depth--
			if depth == 0 && start >= 0 {
				candidates = append(candidates, s[start:i+1])
				start = -1
			}
		}
	}
	return candidates
}

func looksLikeFenceLanguage(token string) bool {
	if token == "" || len(token) > 24 {
		return false
	}
	for _, r := range token {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			continue
		}
		switch r {
		case '_', '-', '+', '#', '.':
			continue
		default:
			return false
		}
	}
	return true
}

func clamp(v, min, max float64) float64 {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}
