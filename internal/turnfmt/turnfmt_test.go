package turnfmt

import (
	"strings"
	"testing"

	"debate/internal/orchestrator"
)

func TestFormatLinesKeepsBlankLinesWhenEnabled(t *testing.T) {
	turn := orchestrator.Turn{
		Index:   1,
		Type:    orchestrator.TurnTypePersona,
		Content: "line1\n\nline2",
	}

	lines := FormatLines(turn, Options{
		Header:         func(orchestrator.Turn) string { return "header" },
		Separator:      func(orchestrator.Turn) string { return "---" },
		KeepBlankLines: true,
	})
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "line1\n\n  line2") {
		t.Fatalf("expected preserved blank line, got %q", joined)
	}
}

func TestFormatLinesSkipsBlankLinesWhenDisabled(t *testing.T) {
	turn := orchestrator.Turn{
		Index:   1,
		Type:    orchestrator.TurnTypePersona,
		Content: "line1\n\nline2",
	}

	lines := FormatLines(turn, Options{
		Header:         func(orchestrator.Turn) string { return "header" },
		Separator:      func(orchestrator.Turn) string { return "---" },
		KeepBlankLines: false,
	})
	joined := strings.Join(lines, "\n")
	if strings.Contains(joined, "line1\n\n  line2") {
		t.Fatalf("expected blank line to be removed, got %q", joined)
	}
}
