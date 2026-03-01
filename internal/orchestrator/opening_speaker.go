package orchestrator

import (
	"strings"
	"unicode"

	"debate/internal/persona"
)

func defaultOpeningSpeakerIndex(problem string, personas []persona.Persona) int {
	if len(personas) == 0 {
		return 0
	}

	problemSet := buildTokenSet(problem)
	if len(problemSet) == 0 {
		return 0
	}

	problemCompact := compactLower(problem)
	bestIdx := 0
	bestScore := -1
	for i, p := range personas {
		score := openingSpeakerScore(problemSet, problemCompact, p)
		if score > bestScore {
			bestScore = score
			bestIdx = i
		}
	}
	if bestScore <= 0 {
		return 0
	}
	return bestIdx
}

func openingSpeakerScore(problemSet map[string]struct{}, problemCompact string, p persona.Persona) int {
	score := 0
	score += overlapScore(problemSet, []string{p.Role}, 12)
	score += overlapScore(problemSet, p.Expertise, 9)
	score += overlapScore(problemSet, p.SignatureLens, 7)
	score += overlapScore(problemSet, p.Constraints, 4)
	score += overlapScore(problemSet, []string{p.Style}, 3)
	score += overlapScore(problemSet, []string{p.Stance}, 3)
	score += overlapScore(problemSet, []string{p.Name, p.MasterName}, 2)
	score += overlapScore(problemSet, []string{p.ID}, 1)

	idCompact := compactLower(p.ID)
	nameCompact := compactLower(p.Name)
	masterCompact := compactLower(p.MasterName)
	if len([]rune(idCompact)) >= 3 && strings.Contains(problemCompact, idCompact) {
		score += 20
	}
	if nameCompact != "" && strings.Contains(problemCompact, nameCompact) {
		score += 10
	}
	if masterCompact != "" && strings.Contains(problemCompact, masterCompact) {
		score += 6
	}
	return score
}

func overlapScore(problemSet map[string]struct{}, fields []string, weight int) int {
	if weight <= 0 || len(problemSet) == 0 {
		return 0
	}
	seen := make(map[string]struct{})
	score := 0
	for _, field := range fields {
		for _, token := range tokenize(field) {
			if _, ok := problemSet[token]; !ok {
				continue
			}
			if _, dup := seen[token]; dup {
				continue
			}
			seen[token] = struct{}{}
			score += weight
		}
	}
	return score
}

func findPersonaIndex(personas []persona.Persona, raw string) int {
	key := normalizeMatchKey(raw)
	if key == "" {
		return -1
	}

	for i, p := range personas {
		if normalizeMatchKey(p.ID) == key {
			return i
		}
	}
	return -1
}

func buildTokenSet(text string) map[string]struct{} {
	set := make(map[string]struct{})
	for _, token := range tokenize(text) {
		set[token] = struct{}{}
	}
	return set
}

func tokenize(text string) []string {
	trimmed := strings.TrimSpace(strings.ToLower(text))
	if trimmed == "" {
		return nil
	}

	parts := strings.FieldsFunc(trimmed, func(r rune) bool {
		return !(unicode.IsLetter(r) || unicode.IsDigit(r))
	})
	out := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, part := range parts {
		p := strings.TrimSpace(part)
		if p == "" {
			continue
		}
		if runeLen(p) < 2 {
			continue
		}
		if _, dup := seen[p]; dup {
			continue
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	return out
}

func compactLower(text string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(text)), ""))
}

func normalizeMatchKey(text string) string {
	return strings.ToLower(strings.TrimSpace(text))
}

func runeLen(s string) int {
	return len([]rune(s))
}
