package tui

import (
	"fmt"
	"strings"

	"debate/internal/persona"
)

func (m *model) buildPersonaPanel(width int, maxLines int) string {
	if maxLines <= 0 {
		maxLines = 1
	}
	if len(m.personas) == 0 {
		return buildEmptyPersonaPanel(width, maxLines)
	}
	if m.shouldUseCompactPersonaPanel(width, maxLines) {
		return m.buildCompactPersonaPanel(width, maxLines)
	}

	lines := make([]string, 0, maxLines)
	nameWidth := maxInt(10, width-15)
	metaWidth := maxInt(10, width-6)
	lensWidth := maxInt(10, width-8)
	rendered := 0
	maxTurns := maxSpeakerTurns(m.speakerTurns)

	for i, p := range m.personas {
		displayName := persona.DisplayName(p)
		marker := " "
		if strings.TrimSpace(m.lastSpeakerName) != "" && displayName == m.lastSpeakerName {
			marker = ">"
		}

		turns := m.speakerTurns[p.ID]
		block := []string{
			fmt.Sprintf("%s %2d) %s [%dT] %s", marker, i+1, truncateText(displayName, nameWidth), turns, miniMeter(turns, maxTurns, 4)),
			fmt.Sprintf("    %s", truncateText("role: "+p.Role+" | stance: "+p.Stance, metaWidth)),
		}

		if strings.TrimSpace(p.MasterName) != "" {
			block = append(block, "    "+truncateText("master: "+p.MasterName, metaWidth))
		}
		if len(p.SignatureLens) > 0 {
			block = append(block, "    "+truncateText("lens: "+p.SignatureLens[0], lensWidth))
		}
		block = append(block, "")

		if len(lines)+len(block) > maxLines {
			break
		}
		lines = append(lines, block...)
		rendered = i + 1
	}

	if strings.TrimSpace(m.lastSpeakerName) != "" {
		lines = appendOverflowLine(lines, "last speaker: "+truncateText(m.lastSpeakerName, width), maxLines, width)
	}
	if rendered < len(m.personas) {
		lines = appendOverflowLine(lines, fmt.Sprintf("... +%d more personas", len(m.personas)-rendered), maxLines, width)
	}
	return strings.Join(lines, "\n")
}

func buildEmptyPersonaPanel(width int, maxLines int) string {
	if maxLines <= 1 {
		return truncateText("no personas loaded (try /load)", maxInt(12, width))
	}
	lines := []string{
		truncateText("(no personas loaded)", maxInt(12, width)),
		truncateText("try /load", maxInt(12, width)),
	}
	if len(lines) > maxLines {
		lines = lines[:maxLines]
	}
	return strings.Join(lines, "\n")
}

func (m model) shouldUseCompactPersonaPanel(width int, maxLines int) bool {
	if width < 34 {
		return true
	}
	return len(m.personas)*3 > maxLines
}

func (m model) buildCompactPersonaPanel(width int, maxLines int) string {
	if maxLines <= 0 {
		return ""
	}
	lines := make([]string, 0, maxLines)
	nameWidth := maxInt(10, width-15)
	overflow := len(m.personas) > maxLines
	visible := maxLines
	maxTurns := maxSpeakerTurns(m.speakerTurns)
	if overflow {
		visible = maxLines - 1
	}
	if visible < 0 {
		visible = 0
	}

	for i := 0; i < len(m.personas) && i < visible; i++ {
		p := m.personas[i]
		displayName := persona.DisplayName(p)
		marker := " "
		if strings.TrimSpace(m.lastSpeakerName) != "" && displayName == m.lastSpeakerName {
			marker = ">"
		}
		turns := m.speakerTurns[p.ID]
		lines = append(lines, fmt.Sprintf("%s %2d) %s [%dT] %s", marker, i+1, truncateText(displayName, nameWidth), turns, miniMeter(turns, maxTurns, 3)))
	}
	if overflow {
		lines = appendOverflowLine(lines, fmt.Sprintf("... +%d more personas", len(m.personas)-visible), maxLines, width)
	}
	return strings.Join(lines, "\n")
}

func appendOverflowLine(lines []string, line string, maxLines int, width int) []string {
	line = truncateText(line, maxInt(12, width))
	if maxLines <= 0 {
		return lines
	}
	if len(lines) < maxLines {
		return append(lines, line)
	}
	if len(lines) == 0 {
		return lines
	}
	lines[maxLines-1] = line
	return lines
}

func maxSpeakerTurns(turns map[string]int) int {
	maxTurns := 0
	for _, t := range turns {
		if t > maxTurns {
			maxTurns = t
		}
	}
	if maxTurns <= 0 {
		return 1
	}
	return maxTurns
}
