package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"

	"debate/internal/orchestrator"
)

func parseCommand(line string) (command string, arg string) {
	line = strings.TrimSpace(line)
	if line == "" {
		return "", ""
	}

	cmd, rest, found := strings.Cut(line, " ")
	cmd = normalizeCommand(cmd)
	if !found {
		return cmd, ""
	}
	return cmd, strings.TrimSpace(rest)
}

func normalizeCommand(cmd string) string {
	switch cmd {
	case "ask", "/ask":
		return "/ask"
	case "stop", "/stop":
		return "/stop"
	case "follow", "/follow":
		return "/follow"
	case "show", "/show":
		return "/show"
	case "load", "/load":
		return "/load"
	case "help", "/help":
		return "/help"
	case "exit", "/exit":
		return "/exit"
	default:
		return cmd
	}
}

func onOff(v bool) string {
	if v {
		return "ON"
	}
	return "OFF"
}

func normalizeMaxTurns(v int) int {
	if v < 0 {
		return 0
	}
	return v
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func wrapLogLines(lines []string, width int) []string {
	if len(lines) == 0 {
		return nil
	}
	if width <= 0 {
		out := make([]string, 0, len(lines))
		out = append(out, lines...)
		return out
	}

	wrapped := make([]string, 0, len(lines)*2)
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			wrapped = append(wrapped, "")
			continue
		}
		if strings.Contains(line, "\x1b[") {
			// Keep ANSI-styled lines intact; content lines are wrapped below.
			wrapped = append(wrapped, line)
			continue
		}
		if runewidth.StringWidth(line) <= width {
			wrapped = append(wrapped, line)
			continue
		}
		wrappedText := runewidth.Wrap(line, width)
		wrapped = append(wrapped, strings.Split(wrappedText, "\n")...)
	}
	return wrapped
}

func wrapLogLinesToWidth(lines []string, width int) string {
	return strings.Join(wrapLogLines(lines, width), "\n")
}

func truncateText(text string, width int) string {
	text = strings.TrimSpace(text)
	if width <= 0 || runewidth.StringWidth(text) <= width {
		return text
	}
	if width == 1 {
		return "…"
	}
	return runewidth.Truncate(text, width, "…")
}

func formatTurnLines(turn orchestrator.Turn) []string {
	header := renderTurnHeader(turn)
	lines := []string{
		"",
		renderTurnSeparator(turn),
		header,
	}

	contentLines := strings.Split(strings.TrimSpace(turn.Content), "\n")
	appended := false
	for _, line := range contentLines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			lines = append(lines, "")
			continue
		}
		lines = append(lines, "  "+trimmed)
		appended = true
	}
	if !appended {
		lines = append(lines, "  (empty)")
	}
	lines = append(lines, renderTurnSeparator(turn), "")
	return lines
}

func renderTurnSeparator(turn orchestrator.Turn) string {
	line := strings.Repeat("-", 52)
	if turn.Type == orchestrator.TurnTypeModerator {
		line = strings.Repeat("=", 52)
	}
	return line
}

func renderTurnHeader(turn orchestrator.Turn) string {
	badge := "[P]"
	if turn.Type == orchestrator.TurnTypeModerator {
		badge = "[M]"
	}

	badgeStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("230")).Background(lipgloss.Color("62")).Padding(0, 1)
	nameStyle := lipgloss.NewStyle().Bold(true).Foreground(speakerColor(turn))
	if turn.Type == orchestrator.TurnTypeModerator {
		badgeStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("230")).Background(lipgloss.Color("166")).Padding(0, 1)
		nameStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("214"))
	}

	label := turn.SpeakerName
	if turn.Type == orchestrator.TurnTypeModerator {
		label = "사회자"
	}

	header := lipgloss.JoinHorizontal(
		lipgloss.Left,
		badgeStyle.Render(badge),
		" ",
		fmt.Sprintf("turn %d", turn.Index),
		" | ",
		nameStyle.Render(label),
	)
	if turn.Timestamp.IsZero() {
		return header
	}

	timeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	stamp := turn.Timestamp.Local().Format(time.TimeOnly)
	return lipgloss.JoinHorizontal(lipgloss.Left, header, " | ", timeStyle.Render(stamp))
}

func speakerColor(turn orchestrator.Turn) lipgloss.Color {
	palette := []string{"39", "69", "75", "81", "111", "141", "147", "177", "207"}
	key := turn.SpeakerID
	if key == "" {
		key = turn.SpeakerName
	}
	sum := 0
	for _, r := range key {
		sum += int(r)
	}
	return lipgloss.Color(palette[sum%len(palette)])
}
