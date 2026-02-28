package openai

import (
	"encoding/json"
	"errors"
	"strings"

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
	jsonText := extractJSONObject(cleaned)
	if jsonText == "" {
		jsonText = cleaned
	}

	var rawMap map[string]json.RawMessage
	if err := json.Unmarshal([]byte(jsonText), &rawMap); err != nil {
		return orchestrator.Consensus{}, err
	}

	requiredKeys := []string{"reached", "score", "summary", "rationale"}
	for _, key := range requiredKeys {
		if _, ok := rawMap[key]; !ok {
			return orchestrator.Consensus{}, errors.New("missing required consensus key: " + key)
		}
	}

	var parsed struct {
		Reached   bool    `json:"reached"`
		Score     float64 `json:"score"`
		Summary   string  `json:"summary"`
		Rationale string  `json:"rationale"`
	}
	if err := json.Unmarshal([]byte(jsonText), &parsed); err != nil {
		return orchestrator.Consensus{}, err
	}

	parsed.Summary = strings.TrimSpace(parsed.Summary)
	parsed.Rationale = strings.TrimSpace(parsed.Rationale)
	parsed.Score = clamp(parsed.Score, 0, 1)

	if parsed.Summary == "" {
		return orchestrator.Consensus{}, errors.New("summary is required")
	}

	return orchestrator.Consensus{
		Reached:   parsed.Reached,
		Score:     parsed.Score,
		Summary:   parsed.Summary,
		Rationale: parsed.Rationale,
	}, nil
}

func stripCodeFence(s string) string {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "```") {
		return s
	}
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSpace(s)
	if idx := strings.IndexByte(s, '\n'); idx >= 0 {
		s = s[idx+1:]
	}
	if end := strings.LastIndex(s, "```"); end >= 0 {
		s = s[:end]
	}
	return strings.TrimSpace(s)
}

func extractJSONObject(s string) string {
	start := strings.IndexByte(s, '{')
	end := strings.LastIndexByte(s, '}')
	if start == -1 || end == -1 || end < start {
		return ""
	}
	return s[start : end+1]
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
