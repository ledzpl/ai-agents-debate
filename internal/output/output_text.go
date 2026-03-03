package output

import (
	"regexp"
	"strings"
)

var markdownEntityReplacer = strings.NewReplacer(
	"&", "&amp;",
	"<", "&lt;",
	">", "&gt;",
)

var evidenceQualityMetadataLine = regexp.MustCompile(`(?i)^\(?\s*(?:evidence_type\s*=\s*)?[^,\)\s]+(?:\s*,\s*|\s+)\s*confidence\s*=\s*[^,\)\s]+\s*\)?[.!?。．…]*$`)
var evidenceQualityMetadataInline = regexp.MustCompile(`(?i)\(?\s*(?:evidence_type\s*=\s*)?[^,\)\s]+(?:\s*,\s*|\s+)\s*confidence\s*=\s*[^,\)\s]+\s*\)?[.!?。．…]*`)
var technicalTermPlainRewrites = []struct {
	pattern *regexp.Regexp
	repl    string
}{
	{pattern: regexp.MustCompile(`(?i)\bp95\b`), repl: "응답속도 상위 구간(p95)"},
	{pattern: regexp.MustCompile(`(?i)\blatency\b`), repl: "응답 지연"},
	{pattern: regexp.MustCompile(`(?i)\brollback\b`), repl: "되돌리기(rollback)"},
	{pattern: regexp.MustCompile(`(?i)\brollout\b`), repl: "점진 배포(rollout)"},
	{pattern: regexp.MustCompile(`(?i)\bcac\b`), repl: "고객 획득 비용(CAC)"},
	{pattern: regexp.MustCompile(`(?i)\bltv\b`), repl: "고객 생애 가치(LTV)"},
	{pattern: regexp.MustCompile(`(?i)\bconversion\b`), repl: "전환율(conversion)"},
}

func safeText(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return "-"
	}
	return markdownEntityReplacer.Replace(strings.ReplaceAll(v, "\n", " "))
}

func markdownBulletedText(v string, indent string) string {
	v = strings.ReplaceAll(v, "\r\n", "\n")
	v = strings.TrimSpace(v)
	if v == "" {
		return indent + "- (empty)"
	}
	lines := strings.Split(v, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "> ") {
			quoted := strings.TrimPrefix(trimmed, "> ")
			out = append(out, indent+"> "+markdownEntityReplacer.Replace(quoted))
			continue
		}
		if hasListPrefix(trimmed) {
			trimmed = markdownEntityReplacer.Replace(trimmed)
			out = append(out, indent+trimmed)
			continue
		}
		trimmed = markdownEntityReplacer.Replace(trimmed)
		out = append(out, indent+"- "+trimmed)
	}
	if len(out) == 0 {
		return indent + "- (empty)"
	}
	return strings.Join(out, "\n")
}

func hasListPrefix(line string) bool {
	if strings.HasPrefix(line, "- ") || strings.HasPrefix(line, "* ") || strings.HasPrefix(line, "+ ") {
		return true
	}
	i := 0
	for i < len(line) && line[i] >= '0' && line[i] <= '9' {
		i++
	}
	if i == 0 || i+1 >= len(line) {
		return false
	}
	return line[i] == '.' && line[i+1] == ' '
}

func sanitizeTurnContentForDisplay(content string) string {
	if strings.TrimSpace(content) == "" {
		return ""
	}
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	visible := make([]string, 0, len(lines))
	for _, line := range lines {
		cleaned := stripEvidenceQualityMetadata(line)
		trimmed := strings.TrimSpace(cleaned)
		if trimmed == "" {
			continue
		}
		if isListMarkerOnly(trimmed) {
			continue
		}
		if isHiddenDirectiveLine(trimmed) {
			continue
		}
		visible = append(visible, rewriteTechnicalTerms(trimmed))
	}
	return strings.TrimSpace(strings.Join(visible, "\n"))
}

func rewriteTechnicalTerms(text string) string {
	rewritten := strings.TrimSpace(text)
	if rewritten == "" {
		return ""
	}
	for _, rule := range technicalTermPlainRewrites {
		rewritten = rule.pattern.ReplaceAllString(rewritten, rule.repl)
	}
	return rewritten
}

func stripEvidenceQualityMetadata(line string) string {
	return evidenceQualityMetadataInline.ReplaceAllString(line, "")
}

func isListMarkerOnly(line string) bool {
	switch line {
	case "-", "*", "+":
		return true
	}
	i := 0
	for i < len(line) && line[i] >= '0' && line[i] <= '9' {
		i++
	}
	return i > 0 && i == len(line)-1 && line[i] == '.'
}

func isHiddenDirectiveLine(line string) bool {
	normalized := normalizeDirectiveCandidate(line)
	if evidenceQualityMetadataLine.MatchString(normalized) {
		return true
	}
	normalized = strings.ToLower(normalized)
	switch {
	case hasDirectivePrefix(normalized, "handoff_ask"),
		hasDirectivePrefix(normalized, "next"),
		hasDirectivePrefix(normalized, "close"),
		hasDirectivePrefix(normalized, "new_point"),
		hasDirectivePrefix(normalized, "new-point"),
		hasDirectivePrefix(normalized, "issue_update"),
		hasDirectivePrefix(normalized, "persuasion_update"),
		hasDirectivePrefix(normalized, "synthesis"),
		hasDirectivePrefix(normalized, "tension"),
		hasDirectivePrefix(normalized, "ask"),
		hasDirectivePrefix(normalized, "decision_check"),
		hasDirectivePrefix(normalized, "decision-check"),
		hasDirectivePrefix(normalized, "persuasion_check"),
		hasDirectivePrefix(normalized, "persuasion-check"),
		hasDirectivePrefix(normalized, "meta_delta"),
		hasDirectivePrefix(normalized, "self_check"),
		hasDirectivePrefix(normalized, "option_a"),
		hasDirectivePrefix(normalized, "option_b"),
		hasDirectivePrefix(normalized, "scorecard"),
		hasDirectivePrefix(normalized, "scorecard_reason"):
		return true
	default:
		return false
	}
}

func hasDirectivePrefix(line string, key string) bool {
	if !strings.HasPrefix(line, key) {
		return false
	}
	rest := strings.TrimSpace(line[len(key):])
	return strings.HasPrefix(rest, ":") || strings.HasPrefix(rest, "=") || strings.HasPrefix(rest, "：")
}

func normalizeDirectiveCandidate(line string) string {
	s := strings.TrimSpace(line)
	for {
		prev := s
		s = strings.TrimSpace(s)

		if strings.HasPrefix(s, ">") {
			s = strings.TrimSpace(strings.TrimPrefix(s, ">"))
		}

		lower := strings.ToLower(s)
		switch {
		case strings.HasPrefix(lower, "- [ ] "), strings.HasPrefix(lower, "- [x] "):
			s = strings.TrimSpace(s[6:])
		case len(s) >= 2 && (s[0] == '-' || s[0] == '*' || s[0] == '+') && s[1] == ' ':
			s = strings.TrimSpace(s[2:])
		default:
			i := 0
			for i < len(s) && s[i] >= '0' && s[i] <= '9' {
				i++
			}
			if i > 0 && i+1 < len(s) && s[i] == '.' && s[i+1] == ' ' {
				s = strings.TrimSpace(s[i+2:])
			}
		}

		if s == prev {
			break
		}
	}
	return s
}
