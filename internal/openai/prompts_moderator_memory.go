package openai

import (
	"fmt"
	"strings"

	"debate/internal/orchestrator"
)

const (
	moderatorRecentLogLimit     = 10
	moderatorMemoryAnchorLimit  = 3
	moderatorSpeakerClaimLimit  = 4
	moderatorClaimSummaryRunes  = 120
	moderatorTensionSummaryRune = 48
)

type speakerClaim struct {
	speaker string
	claim   string
}

type moderatorMemoryBudget struct {
	anchorLimit         int
	speakerClaimLimit   int
	claimSummaryRunes   int
	tensionSummaryRunes int
}

func defaultModeratorMemoryBudget() moderatorMemoryBudget {
	return moderatorMemoryBudget{
		anchorLimit:         moderatorMemoryAnchorLimit,
		speakerClaimLimit:   moderatorSpeakerClaimLimit,
		claimSummaryRunes:   moderatorClaimSummaryRunes,
		tensionSummaryRunes: moderatorTensionSummaryRune,
	}
}

func deriveModeratorMemoryBudget(compressionLevel int) moderatorMemoryBudget {
	budget := defaultModeratorMemoryBudget()
	budget.anchorLimit = shrinkInt(budget.anchorLimit, compressionLevel/2, 2)
	budget.speakerClaimLimit = shrinkInt(budget.speakerClaimLimit, compressionLevel, 2)
	budget.claimSummaryRunes = shrinkInt(budget.claimSummaryRunes, 12*compressionLevel, 72)
	budget.tensionSummaryRunes = shrinkInt(budget.tensionSummaryRunes, 6*compressionLevel, 24)
	return budget
}

func normalizeModeratorMemoryBudget(budget moderatorMemoryBudget) moderatorMemoryBudget {
	defaults := defaultModeratorMemoryBudget()
	if budget.anchorLimit <= 0 {
		budget.anchorLimit = defaults.anchorLimit
	}
	if budget.speakerClaimLimit <= 0 {
		budget.speakerClaimLimit = defaults.speakerClaimLimit
	}
	if budget.claimSummaryRunes <= 0 {
		budget.claimSummaryRunes = defaults.claimSummaryRunes
	}
	if budget.tensionSummaryRunes <= 0 {
		budget.tensionSummaryRunes = defaults.tensionSummaryRunes
	}
	return budget
}

func buildModeratorMemorySnapshot(turns []orchestrator.Turn, previousTurn orchestrator.Turn, budget moderatorMemoryBudget) string {
	budget = normalizeModeratorMemoryBudget(budget)

	var b strings.Builder
	b.WriteString(fmt.Sprintf("- window turns considered: %d\n", len(turns)))

	anchors := selectModeratorAnchorTurns(turns, previousTurn, budget.anchorLimit)
	if len(anchors) == 0 {
		b.WriteString("- anchor turns before latest: none\n")
	} else {
		b.WriteString("- anchor turns before latest:\n")
		writtenAnchors := 0
		for _, t := range anchors {
			summary := summarizeTurnWithType(t, budget.claimSummaryRunes)
			if summary == "" {
				continue
			}
			b.WriteString(fmt.Sprintf("  - [%d][%s][%s] %s\n", t.Index, t.SpeakerName, t.Type, summary))
			writtenAnchors++
		}
		if writtenAnchors == 0 {
			b.WriteString("  - none after control-line filtering\n")
		}
	}

	claims := collectLatestSpeakerClaims(turns, budget.speakerClaimLimit, budget.claimSummaryRunes)
	if len(claims) == 0 {
		b.WriteString("- latest claim per speaker: unavailable\n")
	} else {
		b.WriteString("- latest claim per speaker:\n")
		for _, claim := range claims {
			b.WriteString(fmt.Sprintf("  - %s: %s\n", claim.speaker, claim.claim))
		}
	}

	if tension := buildTensionCandidate(claims, previousTurn, budget.tensionSummaryRunes); tension != "" {
		b.WriteString("- tension candidate: " + tension + "\n")
	}
	return b.String()
}

func selectModeratorAnchorTurns(turns []orchestrator.Turn, previousTurn orchestrator.Turn, limit int) []orchestrator.Turn {
	if limit <= 0 || len(turns) == 0 {
		return nil
	}
	anchors := make([]orchestrator.Turn, 0, limit)
	seenSpeaker := make(map[string]struct{}, limit)

	for i := len(turns) - 1; i >= 0 && len(anchors) < limit; i-- {
		t := turns[i]
		if isSameTurn(t, previousTurn) {
			continue
		}
		if strings.TrimSpace(t.Content) == "" {
			continue
		}
		speakerKey := normalizeSpeakerKey(t)
		if speakerKey == "" {
			continue
		}
		if _, exists := seenSpeaker[speakerKey]; exists {
			continue
		}
		seenSpeaker[speakerKey] = struct{}{}
		anchors = append(anchors, t)
	}

	for i, j := 0, len(anchors)-1; i < j; i, j = i+1, j-1 {
		anchors[i], anchors[j] = anchors[j], anchors[i]
	}
	return anchors
}

func collectLatestSpeakerClaims(turns []orchestrator.Turn, limit int, summaryRunes int) []speakerClaim {
	claims := collectClaimsBySpeaker(turns, limit, summaryRunes, true)
	if len(claims) == 0 {
		claims = collectClaimsBySpeaker(turns, limit, summaryRunes, false)
	}
	for i, j := 0, len(claims)-1; i < j; i, j = i+1, j-1 {
		claims[i], claims[j] = claims[j], claims[i]
	}
	return claims
}

func collectClaimsBySpeaker(turns []orchestrator.Turn, limit int, summaryRunes int, personaOnly bool) []speakerClaim {
	if limit <= 0 {
		return nil
	}
	claims := make([]speakerClaim, 0, limit)
	seenSpeaker := make(map[string]struct{}, limit)

	for i := len(turns) - 1; i >= 0 && len(claims) < limit; i-- {
		t := turns[i]
		if personaOnly && t.Type != orchestrator.TurnTypePersona {
			continue
		}
		speaker := strings.TrimSpace(t.SpeakerName)
		if speaker == "" {
			speaker = strings.TrimSpace(t.SpeakerID)
		}
		if speaker == "" {
			continue
		}
		key := strings.ToLower(speaker)
		if _, exists := seenSpeaker[key]; exists {
			continue
		}
		summary := summarizeTurnWithType(t, summaryRunes)
		if summary == "" {
			continue
		}
		seenSpeaker[key] = struct{}{}
		claims = append(claims, speakerClaim{
			speaker: speaker,
			claim:   summary,
		})
	}
	return claims
}

func buildTensionCandidate(claims []speakerClaim, previousTurn orchestrator.Turn, summaryRunes int) string {
	if len(claims) < 2 {
		return ""
	}
	latestSpeaker := strings.TrimSpace(previousTurn.SpeakerName)
	if latestSpeaker == "" {
		latestSpeaker = strings.TrimSpace(previousTurn.SpeakerID)
	}
	if latestSpeaker != "" {
		for _, current := range claims {
			if !strings.EqualFold(current.speaker, latestSpeaker) {
				continue
			}
			for _, other := range claims {
				if strings.EqualFold(other.speaker, current.speaker) {
					continue
				}
				return fmt.Sprintf("%s (%s) vs %s (%s)",
					current.speaker,
					summarizeTurnContent(current.claim, summaryRunes),
					other.speaker,
					summarizeTurnContent(other.claim, summaryRunes),
				)
			}
		}
	}

	left := claims[len(claims)-2]
	right := claims[len(claims)-1]
	return fmt.Sprintf("%s (%s) vs %s (%s)",
		left.speaker,
		summarizeTurnContent(left.claim, summaryRunes),
		right.speaker,
		summarizeTurnContent(right.claim, summaryRunes),
	)
}

func isSameTurn(a orchestrator.Turn, b orchestrator.Turn) bool {
	return a.Index == b.Index &&
		a.Type == b.Type &&
		strings.TrimSpace(a.SpeakerName) == strings.TrimSpace(b.SpeakerName) &&
		strings.TrimSpace(a.SpeakerID) == strings.TrimSpace(b.SpeakerID)
}

func normalizeSpeakerKey(t orchestrator.Turn) string {
	name := strings.TrimSpace(t.SpeakerName)
	if name == "" {
		name = strings.TrimSpace(t.SpeakerID)
	}
	return strings.ToLower(name)
}

func summarizeTurnContent(content string, limit int) string {
	clean := stripMachineControlLines(content)
	return summarizeSanitizedContent(clean, limit)
}

func summarizeTurnWithType(turn orchestrator.Turn, limit int) string {
	if turn.Type == orchestrator.TurnTypeModerator {
		clean := stripMachineControlLinesPreserveModeratorCore(turn.Content)
		return summarizeSanitizedContent(clean, limit)
	}
	return summarizeTurnContent(turn.Content, limit)
}

func summarizeSanitizedContent(content string, limit int) string {
	compact := strings.Join(strings.Fields(strings.TrimSpace(content)), " ")
	return truncateRunes(compact, limit)
}

func stripMachineControlLines(content string) string {
	text := strings.ReplaceAll(content, "\r\n", "\n")
	lines := strings.Split(text, "\n")
	filtered := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		candidate := normalizeDirectiveLineCandidate(trimmed)
		upper := strings.ToUpper(candidate)
		switch {
		case strings.HasPrefix(upper, "ISSUE_UPDATE:"),
			strings.HasPrefix(upper, "SELF_CHECK:"),
			strings.HasPrefix(upper, "META_DELTA:"),
			strings.HasPrefix(upper, "HANDOFF_ASK:"),
			strings.HasPrefix(upper, "NEXT:"),
			strings.HasPrefix(upper, "CLOSE:"),
			strings.HasPrefix(upper, "NEW_POINT:"),
			strings.HasPrefix(upper, "NEW_POINT="),
			strings.HasPrefix(upper, "SYNTHESIS:"),
			strings.HasPrefix(upper, "TENSION:"),
			strings.HasPrefix(upper, "ASK:"),
			strings.HasPrefix(upper, "DECISION_CHECK:"),
			strings.HasPrefix(upper, "OPTION_A:"),
			strings.HasPrefix(upper, "OPTION_B:"),
			strings.HasPrefix(upper, "SCORECARD:"),
			strings.HasPrefix(upper, "SCORECARD_REASON:"):
			continue
		}
		filtered = append(filtered, line)
	}
	return strings.Join(filtered, "\n")
}

func stripMachineControlLinesPreserveModeratorCore(content string) string {
	text := strings.ReplaceAll(content, "\r\n", "\n")
	lines := strings.Split(text, "\n")
	filtered := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		candidate := normalizeDirectiveLineCandidate(trimmed)
		upper := strings.ToUpper(candidate)
		switch {
		case strings.HasPrefix(upper, "ISSUE_UPDATE:"),
			strings.HasPrefix(upper, "SELF_CHECK:"),
			strings.HasPrefix(upper, "META_DELTA:"),
			strings.HasPrefix(upper, "HANDOFF_ASK:"),
			strings.HasPrefix(upper, "NEXT:"),
			strings.HasPrefix(upper, "CLOSE:"),
			strings.HasPrefix(upper, "NEW_POINT:"),
			strings.HasPrefix(upper, "NEW_POINT="):
			continue
		case strings.HasPrefix(upper, "SYNTHESIS:"),
			strings.HasPrefix(upper, "TENSION:"),
			strings.HasPrefix(upper, "ASK:"),
			strings.HasPrefix(upper, "DECISION_CHECK:"),
			strings.HasPrefix(upper, "OPTION_A:"),
			strings.HasPrefix(upper, "OPTION_B:"),
			strings.HasPrefix(upper, "SCORECARD:"),
			strings.HasPrefix(upper, "SCORECARD_REASON:"):
			payload := extractDirectiveLinePayload(candidate)
			if payload != "" {
				filtered = append(filtered, payload)
			}
			continue
		}
		filtered = append(filtered, line)
	}
	return strings.Join(filtered, "\n")
}

func extractDirectiveLinePayload(candidate string) string {
	idx := strings.Index(candidate, ":")
	if idx < 0 {
		return ""
	}
	return strings.TrimSpace(candidate[idx+1:])
}

func normalizeDirectiveLineCandidate(line string) string {
	value := strings.TrimSpace(line)
	for value != "" {
		prev := value
		lower := strings.ToLower(value)

		if strings.HasPrefix(lower, "- [ ] ") || strings.HasPrefix(lower, "- [x] ") {
			value = strings.TrimSpace(value[6:])
		}
		if strings.HasPrefix(value, ">") {
			value = strings.TrimSpace(value[1:])
		}
		if strings.HasPrefix(value, "- ") || strings.HasPrefix(value, "* ") || strings.HasPrefix(value, "+ ") {
			value = strings.TrimSpace(value[2:])
		}
		if idx := strings.Index(value, ". "); idx > 0 && isLikelyOrderedListPrefix(value[:idx]) {
			value = strings.TrimSpace(value[idx+2:])
		}
		if value == prev {
			break
		}
	}
	return value
}

func isDigits(text string) bool {
	if text == "" {
		return false
	}
	for i := 0; i < len(text); i++ {
		if text[i] < '0' || text[i] > '9' {
			return false
		}
	}
	return true
}

func isLikelyOrderedListPrefix(prefix string) bool {
	if !isDigits(prefix) {
		return false
	}
	// Limit to short ordered-list markers (e.g. "1.", "12.", "123.").
	// Avoid stripping date-like strings such as "2026. 3월".
	return len(prefix) <= 3
}

func truncateRunes(text string, limit int) string {
	if limit <= 0 {
		return ""
	}
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	if limit == 1 {
		return "…"
	}
	return string(runes[:limit-1]) + "…"
}
