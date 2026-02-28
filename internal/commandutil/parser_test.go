package commandutil

import "testing"

func TestParse(t *testing.T) {
	aliases := map[string]string{
		"ask":   "/ask",
		"/ask":  "/ask",
		"show":  "/show",
		"/show": "/show",
	}

	cmd, arg := Parse("ask\tgrowth loop", aliases)
	if cmd != "/ask" || arg != "growth loop" {
		t.Fatalf("unexpected parse result: cmd=%q arg=%q", cmd, arg)
	}

	cmd, arg = Parse("/show", aliases)
	if cmd != "/show" || arg != "" {
		t.Fatalf("unexpected parse result: cmd=%q arg=%q", cmd, arg)
	}

	cmd, arg = Parse("   ", aliases)
	if cmd != "" || arg != "" {
		t.Fatalf("expected empty parse for blank input, got cmd=%q arg=%q", cmd, arg)
	}
}
