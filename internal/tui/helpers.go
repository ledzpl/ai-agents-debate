package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"

	"debate/internal/commandutil"
	"debate/internal/orchestrator"
	"debate/internal/turnfmt"
)

var tuiCommandAliases = map[string]string{
	"ask":     "/ask",
	"/ask":    "/ask",
	"stop":    "/stop",
	"/stop":   "/stop",
	"follow":  "/follow",
	"/follow": "/follow",
	"show":    "/show",
	"/show":   "/show",
	"load":    "/load",
	"/load":   "/load",
	"help":    "/help",
	"/help":   "/help",
	"exit":    "/exit",
	"/exit":   "/exit",
}

func parseCommand(line string) (command string, arg string) {
	return commandutil.Parse(line, tuiCommandAliases)
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

// styleBoxWidth returns the width to pass into Style.Width so that the
// rendered block fits the requested outer width.
func styleBoxWidth(style lipgloss.Style, outerWidth int) int {
	return maxInt(1, outerWidth-style.GetHorizontalMargins()-style.GetHorizontalBorderSize())
}

// styleTextWidth returns the visible text area width inside a styled block.
func styleTextWidth(style lipgloss.Style, outerWidth int) int {
	return maxInt(1, outerWidth-style.GetHorizontalFrameSize())
}

// styleBoxHeight returns the height to pass into Style.Height so that the
// rendered block fits the requested outer height.
func styleBoxHeight(style lipgloss.Style, outerHeight int) int {
	return maxInt(1, outerHeight-style.GetVerticalMargins()-style.GetVerticalBorderSize())
}

// styleTextHeight returns the visible text area height inside a styled block.
func styleTextHeight(style lipgloss.Style, outerHeight int) int {
	return maxInt(1, outerHeight-style.GetVerticalFrameSize())
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
	return turnfmt.FormatLines(turn, turnfmt.Options{
		Header:         renderTurnHeader,
		Separator:      renderTurnSeparator,
		ContentPrefix:  "  ",
		KeepBlankLines: true,
	})
}

func renderTurnSeparator(turn orchestrator.Turn) string {
	line := strings.Repeat("-", 58)
	if turn.Type == orchestrator.TurnTypeModerator {
		line = strings.Repeat("=", 58)
	}
	return line
}

func renderTurnHeader(turn orchestrator.Turn) string {
	badge := "[P]"
	if turn.Type == orchestrator.TurnTypeModerator {
		badge = "[M]"
	}

	badgeStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("231")).Background(lipgloss.Color("31")).Padding(0, 1)
	nameStyle := lipgloss.NewStyle().Bold(true).Foreground(speakerColor(turn))
	metaStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("151"))
	if turn.Type == orchestrator.TurnTypeModerator {
		badgeStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("230")).Background(lipgloss.Color("166")).Padding(0, 1)
		nameStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("222"))
	}

	label := turn.SpeakerName
	if turn.Type == orchestrator.TurnTypeModerator {
		label = "사회자"
	}

	header := lipgloss.JoinHorizontal(
		lipgloss.Left,
		badgeStyle.Render(badge),
		" ",
		metaStyle.Render(fmt.Sprintf("turn %d", turn.Index)),
		" | ",
		nameStyle.Render(label),
	)
	if turn.Timestamp.IsZero() {
		return header
	}

	timeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("151"))
	stamp := turn.Timestamp.Local().Format(time.TimeOnly)
	return lipgloss.JoinHorizontal(lipgloss.Left, header, " | ", timeStyle.Render(stamp))
}

func speakerColor(turn orchestrator.Turn) lipgloss.Color {
	palette := []string{"45", "51", "80", "86", "111", "117", "123", "159", "194"}
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
