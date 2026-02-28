package tui

import (
	"fmt"
	"strings"

	"debate/internal/persona"
)

func (m model) progressLine(width int) string {
	if m.maxTurns <= 0 {
		if m.running {
			return fmt.Sprintf("round progress  %s  %d/INF", renderProgressBar(minInt(30, maxInt(12, width-30)), m.personaTurnCount, 0), m.personaTurnCount)
		}
		return "round progress  unbounded"
	}

	barWidth := minInt(30, maxInt(12, width-34))
	bar := renderProgressBar(barWidth, m.personaTurnCount, m.maxTurns)
	pct := 0
	if m.maxTurns > 0 {
		pct = int((float64(m.personaTurnCount) / float64(m.maxTurns)) * 100)
		if pct > 100 {
			pct = 100
		}
	}
	return fmt.Sprintf("round progress  %s  %d/%d (%d%%)", bar, m.personaTurnCount, m.maxTurns, pct)
}

func renderProgressBar(width int, current int, total int) string {
	if width <= 0 {
		return "[]"
	}
	if total <= 0 {
		if current <= 0 {
			return "[" + strings.Repeat("░", width) + "]"
		}
		return "[" + strings.Repeat("█", width) + "]"
	}

	ratio := float64(current) / float64(total)
	if ratio < 0 {
		ratio = 0
	}
	if ratio > 1 {
		ratio = 1
	}
	filled := int(ratio * float64(width))
	if current > 0 && filled == 0 {
		filled = 1
	}
	if filled > width {
		filled = width
	}
	return "[" + strings.Repeat("█", filled) + strings.Repeat("░", width-filled) + "]"
}

func (m model) personaActivityLine(width int) string {
	if len(m.personas) == 0 {
		return "-"
	}

	maxTurnsBySpeaker := 0
	for _, p := range m.personas {
		if t := m.speakerTurns[p.ID]; t > maxTurnsBySpeaker {
			maxTurnsBySpeaker = t
		}
	}
	if maxTurnsBySpeaker == 0 {
		return "no-turns"
	}

	parts := make([]string, 0, len(m.personas))
	for _, p := range m.personas {
		label := personaInitial(p)
		meter := miniMeter(m.speakerTurns[p.ID], maxTurnsBySpeaker, 4)
		parts = append(parts, fmt.Sprintf("%s%s", label, meter))
	}
	return truncateText(strings.Join(parts, " "), width)
}

func miniMeter(value int, maxValue int, width int) string {
	if width <= 0 {
		return ""
	}
	if maxValue <= 0 {
		return strings.Repeat("·", width)
	}
	filled := int((float64(value) / float64(maxValue)) * float64(width))
	if value > 0 && filled == 0 {
		filled = 1
	}
	if filled > width {
		filled = width
	}
	return strings.Repeat("▮", filled) + strings.Repeat("▯", width-filled)
}

func personaInitial(p persona.Persona) string {
	name := strings.TrimSpace(persona.DisplayName(p))
	if name == "" {
		return "?"
	}
	for _, r := range name {
		return strings.ToUpper(string(r))
	}
	return "?"
}
