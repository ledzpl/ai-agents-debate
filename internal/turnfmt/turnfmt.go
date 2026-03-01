package turnfmt

import (
	"strings"

	"debate/internal/orchestrator"
)

type Options struct {
	Header         func(orchestrator.Turn) string
	Separator      func(orchestrator.Turn) string
	ContentPrefix  string
	KeepBlankLines bool
}

func FormatLines(turn orchestrator.Turn, opts Options) []string {
	header := defaultHeader(turn)
	if opts.Header != nil {
		header = opts.Header(turn)
	}

	separator := defaultSeparator(turn)
	if opts.Separator != nil {
		separator = opts.Separator(turn)
	}

	prefix := opts.ContentPrefix
	if prefix == "" {
		prefix = "  "
	}

	lines := []string{"", separator, header}
	contentLines := strings.Split(strings.TrimSpace(turn.Content), "\n")
	appended := false

	for _, line := range contentLines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			if opts.KeepBlankLines {
				lines = append(lines, "")
			}
			continue
		}
		lines = append(lines, prefix+trimmed)
		appended = true
	}
	if !appended {
		lines = append(lines, prefix+"(empty)")
	}
	lines = append(lines, separator, "")
	return lines
}

func defaultHeader(turn orchestrator.Turn) string {
	return "turn"
}

func defaultSeparator(turn orchestrator.Turn) string {
	return "---"
}
