package openai

import (
	"fmt"
	"strings"

	"debate/internal/orchestrator"
)

const (
	moderatorRecentLogLimit     = 12
	moderatorMemoryAnchorLimit  = 3
	moderatorSpeakerClaimLimit  = 4
	moderatorClaimSummaryRunes  = 140
	moderatorTensionSummaryRune = 56
)

type speakerClaim struct {
	speaker string
	claim   string
}

func buildModeratorMemorySnapshot(turns []orchestrator.Turn, previousTurn orchestrator.Turn) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("- window turns considered: %d\n", len(turns)))

	anchors := selectModeratorAnchorTurns(turns, previousTurn, moderatorMemoryAnchorLimit)
	if len(anchors) == 0 {
		b.WriteString("- anchor turns before latest: none\n")
	} else {
		b.WriteString("- anchor turns before latest:\n")
		for _, t := range anchors {
			b.WriteString(fmt.Sprintf("  - [%d][%s][%s] %s\n", t.Index, t.SpeakerName, t.Type, summarizeTurnContent(t.Content, moderatorClaimSummaryRunes)))
		}
	}

	claims := collectLatestSpeakerClaims(turns, moderatorSpeakerClaimLimit)
	if len(claims) == 0 {
		b.WriteString("- latest claim per speaker: unavailable\n")
	} else {
		b.WriteString("- latest claim per speaker:\n")
		for _, claim := range claims {
			b.WriteString(fmt.Sprintf("  - %s: %s\n", claim.speaker, claim.claim))
		}
	}

	if tension := buildTensionCandidate(claims, previousTurn); tension != "" {
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

func collectLatestSpeakerClaims(turns []orchestrator.Turn, limit int) []speakerClaim {
	claims := collectClaimsBySpeaker(turns, limit, true)
	if len(claims) == 0 {
		claims = collectClaimsBySpeaker(turns, limit, false)
	}
	for i, j := 0, len(claims)-1; i < j; i, j = i+1, j-1 {
		claims[i], claims[j] = claims[j], claims[i]
	}
	return claims
}

func collectClaimsBySpeaker(turns []orchestrator.Turn, limit int, personaOnly bool) []speakerClaim {
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
		seenSpeaker[key] = struct{}{}
		claims = append(claims, speakerClaim{
			speaker: speaker,
			claim:   summarizeTurnContent(t.Content, moderatorClaimSummaryRunes),
		})
	}
	return claims
}

func buildTensionCandidate(claims []speakerClaim, previousTurn orchestrator.Turn) string {
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
					summarizeTurnContent(current.claim, moderatorTensionSummaryRune),
					other.speaker,
					summarizeTurnContent(other.claim, moderatorTensionSummaryRune),
				)
			}
		}
	}

	left := claims[len(claims)-2]
	right := claims[len(claims)-1]
	return fmt.Sprintf("%s (%s) vs %s (%s)",
		left.speaker,
		summarizeTurnContent(left.claim, moderatorTensionSummaryRune),
		right.speaker,
		summarizeTurnContent(right.claim, moderatorTensionSummaryRune),
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
	compact := strings.Join(strings.Fields(strings.TrimSpace(content)), " ")
	return truncateRunes(compact, limit)
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
