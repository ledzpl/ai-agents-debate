package output

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"debate/internal/orchestrator"
	"debate/internal/persona"
)

type turnSpeakerGroup struct {
	Speaker string
	Anchor  string
	Turns   []orchestrator.Turn
}

func SaveResult(path string, result orchestrator.Result) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create output dir: %w", err)
	}

	jsonData, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal result: %w", err)
	}
	if err := writeAtomic(path, jsonData, 0o644); err != nil {
		return fmt.Errorf("write json result file: %w", err)
	}

	mdPath := MarkdownPath(path)
	mdData := []byte(formatResultMarkdown(result))
	if err := writeAtomic(mdPath, mdData, 0o644); err != nil {
		return fmt.Errorf("write markdown result file: %w", err)
	}
	return nil
}

func MarkdownPath(path string) string {
	ext := filepath.Ext(path)
	if ext == "" {
		return path + ".md"
	}
	return strings.TrimSuffix(path, ext) + ".md"
}

func writeAtomic(path string, data []byte, perm os.FileMode) error {
	tempPath := path + ".tmp"
	if err := os.WriteFile(tempPath, data, perm); err != nil {
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := os.Rename(tempPath, path); err != nil {
		_ = os.Remove(tempPath)
		return fmt.Errorf("move temp file: %w", err)
	}
	return nil
}

func formatResultMarkdown(result orchestrator.Result) string {
	var b strings.Builder

	b.WriteString("# Debate Result\n\n")
	b.WriteString("- status: " + safeText(result.Status) + "\n")
	b.WriteString(fmt.Sprintf("- consensus_score: %.2f\n", result.Consensus.Score))
	if !result.StartedAt.IsZero() {
		b.WriteString("- started_at: " + result.StartedAt.UTC().Format(time.RFC3339) + "\n")
	}
	if !result.EndedAt.IsZero() {
		b.WriteString("- ended_at: " + result.EndedAt.UTC().Format(time.RFC3339) + "\n")
	}
	if !result.StartedAt.IsZero() && !result.EndedAt.IsZero() {
		b.WriteString("- duration: " + result.EndedAt.Sub(result.StartedAt).Round(time.Millisecond).String() + "\n")
	}
	b.WriteString(fmt.Sprintf("- turns: %d\n", len(result.Turns)))
	b.WriteString("\n## Problem\n\n")
	b.WriteString(markdownBulletedText(result.Problem, "") + "\n\n")

	b.WriteString("## Consensus\n\n")
	b.WriteString(fmt.Sprintf("- reached: %t\n", result.Consensus.Reached))
	b.WriteString(fmt.Sprintf("- score: %.2f\n", result.Consensus.Score))
	if strings.TrimSpace(result.Consensus.Summary) != "" {
		b.WriteString("\n### Summary\n\n")
		b.WriteString(markdownBulletedText(result.Consensus.Summary, "") + "\n")
	}
	if strings.TrimSpace(result.Consensus.Rationale) != "" {
		b.WriteString("\n### Rationale\n\n")
		b.WriteString(markdownBulletedText(result.Consensus.Rationale, "") + "\n")
	}

	b.WriteString("\n## Personas\n\n")
	if len(result.Personas) == 0 {
		b.WriteString("- none\n")
	} else {
		for i, p := range result.Personas {
			line := fmt.Sprintf("%d. **%s** (`%s`) - role: %s, stance: %s",
				i+1, safeText(persona.DisplayName(p)), safeText(p.ID), safeText(p.Role), safeText(p.Stance))
			if strings.TrimSpace(p.MasterName) != "" {
				line += ", master_name: " + safeText(p.MasterName)
			}
			b.WriteString(line + "\n")
		}
	}

	b.WriteString("\n## Turns\n\n")
	b.WriteString(formatTurnsBySpeaker(result.Turns))
	b.WriteString("\n")

	b.WriteString("## Metrics\n\n")
	b.WriteString(fmt.Sprintf("- latency_ms: %d\n", result.Metrics.LatencyMS))
	b.WriteString(fmt.Sprintf("- prompt_tokens: %d\n", result.Metrics.PromptTokens))
	b.WriteString(fmt.Sprintf("- completion_tokens: %d\n", result.Metrics.CompletionTokens))
	b.WriteString(fmt.Sprintf("- total_tokens: %d\n", result.Metrics.TotalTokens))
	return b.String()
}

func safeText(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return "-"
	}
	return strings.ReplaceAll(v, "\n", " ")
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
		if hasListPrefix(trimmed) || strings.HasPrefix(trimmed, "> ") {
			out = append(out, indent+trimmed)
			continue
		}
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

func formatTurnsBySpeaker(turns []orchestrator.Turn) string {
	if len(turns) == 0 {
		return "- no turns\n"
	}

	groups := groupTurnsBySpeaker(turns)
	var b strings.Builder

	b.WriteString("### TOC (by speaker)\n\n")
	for _, group := range groups {
		b.WriteString(fmt.Sprintf("- [%s](#%s) (%d %s)\n",
			safeText(group.Speaker),
			group.Anchor,
			len(group.Turns),
			turnWord(len(group.Turns)),
		))
	}

	for i, group := range groups {
		b.WriteString(fmt.Sprintf("\n<a id=\"%s\"></a>\n", group.Anchor))
		b.WriteString("<details>\n")
		b.WriteString(fmt.Sprintf("<summary><strong>%s</strong> · %d %s</summary>\n\n",
			safeText(group.Speaker),
			len(group.Turns),
			turnWord(len(group.Turns)),
		))

		for _, t := range group.Turns {
			header := fmt.Sprintf("#### Turn %d (%s)", t.Index, safeText(t.Type))
			b.WriteString(header + "\n\n")
			if !t.Timestamp.IsZero() {
				b.WriteString("- timestamp: " + t.Timestamp.UTC().Format(time.RFC3339) + "\n")
			}
			b.WriteString("- content:\n")
			b.WriteString(markdownBulletedText(t.Content, "  ") + "\n\n")
		}

		b.WriteString("</details>\n")
		if i < len(groups)-1 {
			b.WriteString("\n---\n")
		}
	}
	return b.String()
}

func groupTurnsBySpeaker(turns []orchestrator.Turn) []turnSpeakerGroup {
	groups := make([]turnSpeakerGroup, 0, len(turns))
	indexByKey := make(map[string]int, len(turns))

	for _, turn := range turns {
		speaker := strings.TrimSpace(turn.SpeakerName)
		if speaker == "" {
			speaker = strings.TrimSpace(turn.SpeakerID)
		}
		if speaker == "" {
			speaker = "Unknown Speaker"
		}

		key := strings.ToLower(speaker)
		idx, ok := indexByKey[key]
		if !ok {
			idx = len(groups)
			indexByKey[key] = idx
			groups = append(groups, turnSpeakerGroup{
				Speaker: speaker,
				Anchor:  fmt.Sprintf("turns-speaker-%d", idx+1),
			})
		}
		groups[idx].Turns = append(groups[idx].Turns, turn)
	}

	return groups
}

func turnWord(n int) string {
	if n == 1 {
		return "turn"
	}
	return "turns"
}

func NewTimestampPath(dir string, now time.Time) string {
	name := now.UTC().Format("20060102-150405.000000000") + "-debate.json"
	return filepath.Join(dir, name)
}
