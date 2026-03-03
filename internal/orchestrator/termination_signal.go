package orchestrator

import "strings"

type turnTerminationSignal struct {
	closeVote         *bool
	newPoint          *bool
	persuasionAdopted bool
	boundedExperiment bool
}

type terminationSignalTracker struct {
	latestCloseBySpeaker map[string]bool
	noNewPointStreak     int
	observedPersonaTurns int
	hasPersuasionSignal  bool
	hasExperimentSignal  bool
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
	if signal.persuasionAdopted {
		t.hasPersuasionSignal = true
	}
	if signal.boundedExperiment {
		t.hasExperimentSignal = true
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
	if !t.hasPersuasionSignal && !t.hasExperimentSignal {
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
	for _, raw := range lines {
		line := normalizeTerminationDirectiveLine(raw)
		if value, ok := parseDirectiveBool(line, "CLOSE:", "CLOSE=", "종료:", "토론종료:"); ok {
			v := value
			signal.closeVote = &v
		}
		if value, ok := parseDirectiveBool(line, "NEW_POINT:", "NEW_POINT=", "NEW-POINT:", "신규포인트:", "새논점:"); ok {
			v := value
			signal.newPoint = &v
		}
		if !signal.persuasionAdopted && hasPersuasionAdoptionLine(line) {
			signal.persuasionAdopted = true
		}
		if !signal.boundedExperiment && hasBoundedExperimentLine(line) {
			signal.boundedExperiment = true
		}
	}
	return signal
}

func hasPersuasionAdoptionLine(line string) bool {
	rest, ok := trimPrefixFold(strings.TrimSpace(line), "PERSUASION_UPDATE:")
	if !ok {
		return false
	}
	changedRaw := extractDirectiveAssignmentValue(rest, "changed")
	changed, changedOK := parseBoolToken(strings.ToLower(strings.TrimSpace(changedRaw)))
	if !changedOK || !changed {
		return false
	}
	adopted := extractDirectiveAssignmentValue(rest, "adopted")
	return !isNoneLikeDirectiveValue(adopted)
}

func hasBoundedExperimentLine(line string) bool {
	lower := strings.ToLower(strings.TrimSpace(line))
	if !strings.Contains(lower, "success_metric=") || !strings.Contains(lower, "stop_condition=") {
		return false
	}
	if !strings.Contains(lower, "owner=") {
		return false
	}
	return strings.Contains(lower, "decide_by=") || strings.Contains(lower, "deadline=")
}

func extractDirectiveAssignmentValue(line string, key string) string {
	if line == "" || key == "" {
		return ""
	}
	lowerLine := strings.ToLower(line)
	target := strings.ToLower(strings.TrimSpace(key)) + "="
	searchFrom := 0
	for {
		relative := strings.Index(lowerLine[searchFrom:], target)
		if relative < 0 {
			return ""
		}
		idx := searchFrom + relative
		if idx > 0 && isDirectiveKeyChar(lowerLine[idx-1]) {
			searchFrom = idx + 1
			continue
		}
		value := strings.TrimSpace(line[idx+len(target):])
		if value == "" {
			return ""
		}
		if cut := strings.IndexAny(value, ";|"); cut >= 0 {
			value = strings.TrimSpace(value[:cut])
		}
		return strings.TrimSpace(strings.Trim(value, "\"'`"))
	}
}

func isDirectiveKeyChar(ch byte) bool {
	return (ch >= 'a' && ch <= 'z') ||
		(ch >= '0' && ch <= '9') ||
		ch == '_' ||
		ch == '-'
}

func isNoneLikeDirectiveValue(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "none", "n/a", "na", "-", "unknown", "tbd", "no":
		return true
	default:
		return false
	}
}

func normalizeTerminationDirectiveLine(line string) string {
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
		if idx := strings.Index(value, ". "); idx > 0 && isShortNumericPrefix(value[:idx]) {
			value = strings.TrimSpace(value[idx+2:])
		}
		if value == prev {
			break
		}
	}
	return value
}

func isShortNumericPrefix(prefix string) bool {
	if prefix == "" || len(prefix) > 3 {
		return false
	}
	for i := 0; i < len(prefix); i++ {
		if prefix[i] < '0' || prefix[i] > '9' {
			return false
		}
	}
	return true
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
