package orchestrator

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"debate/internal/persona"
)

var koreanAddressingSuffixes = []string{
	"에게", "한테", "께", "님", "씨",
	"이", "가", "은", "는", "을", "를", "와", "과", "도",
}

const canonicalNextSpeakerPrefix = "NEXT:"

func selectNextSpeakerIndex(personas []persona.Persona, currentSpeaker persona.Persona, content string, fallbackIndex int) int {
	index, _ := selectNextSpeaker(personas, currentSpeaker, content, fallbackIndex)
	return index
}

func selectNextSpeaker(personas []persona.Persona, currentSpeaker persona.Persona, content string, fallbackIndex int) (int, bool) {
	if len(personas) == 0 {
		return -1, false
	}
	if fallbackIndex < 0 || fallbackIndex >= len(personas) {
		fallbackIndex = 0
	}

	currentSpeakerKey := normalizeMatchKey(currentSpeaker.ID)
	if explicitID := extractExplicitNextSpeakerID(content); explicitID != "" {
		if idx := findPersonaIndex(personas, explicitID); idx >= 0 {
			if currentSpeakerKey == "" || normalizeMatchKey(personas[idx].ID) != currentSpeakerKey {
				return idx, true
			}
		}
	}

	segments := handoffCandidateSegments(content)
	for _, segment := range segments {
		if idx := matchSinglePersonaIndex(personas, currentSpeakerKey, segment); idx >= 0 {
			return idx, true
		}
	}
	return fallbackIndex, false
}

func appendCanonicalNextSpeakerLine(content string, nextSpeaker persona.Persona) string {
	nextID := strings.TrimSpace(nextSpeaker.ID)
	if nextID == "" {
		return strings.TrimSpace(content)
	}

	base := strings.TrimSpace(content)
	line := canonicalNextSpeakerPrefix + " " + nextID
	if existingID := extractExplicitNextSpeakerID(base); existingID != "" && strings.EqualFold(existingID, nextID) {
		return base
	}
	if base == "" {
		return line
	}
	return base + "\n" + line
}

func extractExplicitNextSpeakerID(content string) string {
	lines := strings.Split(strings.TrimSpace(content), "\n")
	checked := 0
	for i := len(lines) - 1; i >= 0 && checked < 3; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		checked++
		if id := parseExplicitNextSpeakerLine(line); id != "" {
			return id
		}
	}
	return ""
}

func parseExplicitNextSpeakerLine(line string) string {
	prefixes := []string{
		"NEXT:",
		"NEXT=",
		"NEXT_SPEAKER:",
		"NEXT_SPEAKER=",
		"다음 화자:",
		"다음화자:",
	}
	for _, prefix := range prefixes {
		rest, ok := trimPrefixFold(line, prefix)
		if !ok {
			continue
		}
		rest = strings.TrimSpace(rest)
		rest, ok = trimPrefixFold(rest, "persona_id=")
		if ok {
			rest = strings.TrimSpace(rest)
		}
		if rest == "" {
			return ""
		}
		token := strings.Fields(rest)[0]
		token = strings.TrimPrefix(token, "@")
		token = strings.Trim(token, "\"'`.,;:!?)]}>")
		return strings.TrimSpace(token)
	}
	return ""
}

func trimPrefixFold(text string, prefix string) (string, bool) {
	if len(text) < len(prefix) {
		return text, false
	}
	if !strings.EqualFold(text[:len(prefix)], prefix) {
		return text, false
	}
	return text[len(prefix):], true
}

func handoffCandidateSegments(content string) []string {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return nil
	}

	out := make([]string, 0, 2)
	if line := lastNonEmptyLine(trimmed); line != "" {
		out = append(out, line)
	}
	if sentence := lastSentence(trimmed); sentence != "" && !containsFold(out, sentence) {
		out = append(out, sentence)
	}
	return out
}

func matchSinglePersonaIndex(personas []persona.Persona, currentSpeakerKey string, text string) int {
	matchIndex := -1
	for i, p := range personas {
		if currentSpeakerKey != "" && normalizeMatchKey(p.ID) == currentSpeakerKey {
			continue
		}
		if !mentionsPersona(text, p) {
			continue
		}
		if matchIndex >= 0 {
			return -1
		}
		matchIndex = i
	}
	return matchIndex
}

func mentionsPersona(text string, p persona.Persona) bool {
	for _, alias := range personaMentionAliases(p) {
		if mentionsAlias(text, alias) {
			return true
		}
	}
	return false
}

func personaMentionAliases(p persona.Persona) []string {
	raw := []string{
		strings.TrimSpace(p.ID),
		strings.TrimSpace(p.Name),
		strings.TrimSpace(persona.DisplayName(p)),
	}

	out := make([]string, 0, len(raw))
	seen := make(map[string]struct{}, len(raw))
	for _, alias := range raw {
		if alias == "" {
			continue
		}
		key := normalizeMatchKey(alias)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, alias)
	}
	return out
}

func mentionsAlias(text string, alias string) bool {
	normalizedText := strings.ToLower(strings.TrimSpace(text))
	normalizedAlias := strings.ToLower(strings.TrimSpace(alias))
	if normalizedText == "" || normalizedAlias == "" {
		return false
	}
	if strings.Contains(normalizedText, "@"+normalizedAlias) {
		return true
	}
	for _, suffix := range koreanAddressingSuffixes {
		if strings.Contains(normalizedText, normalizedAlias+suffix) {
			return true
		}
	}
	// One- or two-character aliases (for example "a", "x") are too ambiguous in free text.
	// Require explicit markers (@alias or Korean addressing suffix) for these short aliases.
	if utf8.RuneCountInString(normalizedAlias) <= 2 {
		return false
	}
	if containsWithBoundary(normalizedText, normalizedAlias) {
		return true
	}
	return false
}

func containsWithBoundary(text string, alias string) bool {
	start := 0
	for {
		offset := strings.Index(text[start:], alias)
		if offset < 0 {
			return false
		}
		idx := start + offset
		end := idx + len(alias)

		beforeOK := idx == 0
		if !beforeOK {
			r, _ := utf8.DecodeLastRuneInString(text[:idx])
			beforeOK = !isWordRune(r)
		}

		afterOK := end == len(text)
		if !afterOK {
			r, _ := utf8.DecodeRuneInString(text[end:])
			afterOK = !isWordRune(r)
		}

		if beforeOK && afterOK {
			return true
		}
		start = end
		if start >= len(text) {
			return false
		}
	}
}

func isWordRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_'
}

func lastNonEmptyLine(text string) string {
	lines := strings.Split(text, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line != "" {
			return line
		}
	}
	return ""
}

func lastSentence(text string) string {
	parts := strings.FieldsFunc(text, func(r rune) bool {
		switch r {
		case '.', '?', '!', '\n', '。', '？', '！':
			return true
		default:
			return false
		}
	})
	for i := len(parts) - 1; i >= 0; i-- {
		part := strings.TrimSpace(parts[i])
		if part != "" {
			return part
		}
	}
	return ""
}

func containsFold(values []string, needle string) bool {
	for _, v := range values {
		if strings.EqualFold(v, needle) {
			return true
		}
	}
	return false
}
