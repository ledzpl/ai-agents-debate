package orchestrator

import "strings"

type turnTerminationSignal struct {
	closeVote *bool
	newPoint  *bool
}

type terminationSignalTracker struct {
	latestCloseBySpeaker map[string]bool
	noNewPointStreak     int
	observedPersonaTurns int
}

func newTerminationSignalTracker() terminationSignalTracker {
	return terminationSignalTracker{
		latestCloseBySpeaker: make(map[string]bool),
	}
}

func (t *terminationSignalTracker) observe(turn Turn) {
	if turn.Type != TurnTypePersona {
		return
	}
	t.observedPersonaTurns++

	signal := parseTurnTerminationSignal(turn.Content)
	if signal.closeVote != nil {
		key := normalizeTurnSpeakerKey(turn)
		if key != "" {
			t.latestCloseBySpeaker[key] = *signal.closeVote
		}
	}

	if signal.newPoint == nil {
		// Missing signal should not erase existing stagnation evidence.
		return
	}
	if *signal.newPoint {
		t.noNewPointStreak = 0
		return
	}
	t.noNewPointStreak++
}

func (t *terminationSignalTracker) shouldSuggestStop(personaCount int) bool {
	if personaCount <= 1 {
		return false
	}
	if t.observedPersonaTurns < personaCount {
		return false
	}
	if t.closeYesCount() < requiredCloseVotes(personaCount) {
		return false
	}

	stallNeed := personaCount
	if stallNeed < 2 {
		stallNeed = 2
	}
	return t.noNewPointStreak >= stallNeed
}

func (t *terminationSignalTracker) closeYesCount() int {
	count := 0
	for _, yes := range t.latestCloseBySpeaker {
		if yes {
			count++
		}
	}
	return count
}

func requiredCloseVotes(personaCount int) int {
	if personaCount <= 1 {
		return 1
	}
	return (2*personaCount + 2) / 3 // ceil(2n/3)
}

func parseTurnTerminationSignal(content string) turnTerminationSignal {
	lines := nonEmptyAllLines(content)
	var signal turnTerminationSignal
	for _, line := range lines {
		if value, ok := parseDirectiveBool(line, "CLOSE:", "CLOSE=", "종료:", "토론종료:"); ok {
			v := value
			signal.closeVote = &v
		}
		if value, ok := parseDirectiveBool(line, "NEW_POINT:", "NEW_POINT=", "NEW-POINT:", "신규포인트:", "새논점:"); ok {
			v := value
			signal.newPoint = &v
		}
	}
	return signal
}

func parseDirectiveBool(line string, prefixes ...string) (bool, bool) {
	trimmed := strings.TrimSpace(line)
	for _, prefix := range prefixes {
		rest, ok := trimPrefixFold(trimmed, prefix)
		if !ok {
			continue
		}
		token := firstDirectiveToken(rest)
		if token == "" {
			return false, false
		}
		return parseBoolToken(token)
	}
	return false, false
}

func firstDirectiveToken(text string) string {
	parts := strings.Fields(strings.TrimSpace(text))
	if len(parts) == 0 {
		return ""
	}
	token := parts[0]
	token = strings.TrimPrefix(token, "@")
	token = strings.Trim(token, "\"'`.,;:!?)]}>")
	return strings.ToLower(strings.TrimSpace(token))
}

func parseBoolToken(token string) (bool, bool) {
	switch token {
	case "yes", "y", "true", "1", "예", "네", "종료", "close", "done", "있음":
		return true, true
	case "no", "n", "false", "0", "아니오", "아니요", "계속", "open", "continue", "없음":
		return false, true
	default:
		return false, false
	}
}

func normalizeTurnSpeakerKey(turn Turn) string {
	key := normalizeMatchKey(turn.SpeakerID)
	if key != "" {
		return key
	}
	return strings.ToLower(strings.TrimSpace(turn.SpeakerName))
}

func nonEmptyAllLines(content string) []string {
	lines := strings.Split(strings.TrimSpace(content), "\n")
	out := make([]string, 0, len(lines))
	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		out = append(out, line)
	}
	return out
}
