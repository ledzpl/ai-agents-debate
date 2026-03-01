package output

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"debate/internal/orchestrator"
)

func TestSaveResultWritesJSONAndMarkdown(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "result.json")
	result := orchestrator.Result{
		Problem:   "test problem\nsecond line",
		Status:    orchestrator.StatusMaxTurnsReached,
		StartedAt: time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC),
		EndedAt:   time.Date(2026, 3, 1, 10, 0, 5, 0, time.UTC),
		Turns: []orchestrator.Turn{
			{Index: 1, SpeakerName: "A", Type: orchestrator.TurnTypePersona, Content: "first point\nsecond point"},
			{Index: 2, SpeakerName: "사회자", Type: orchestrator.TurnTypeModerator, Content: "next question"},
		},
		Consensus: orchestrator.Consensus{
			Reached:   true,
			Score:     0.91,
			Summary:   "aligned\nwith constraints",
			Rationale: "enough evidence",
		},
	}

	if err := SaveResult(path, result); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("expected non-empty result file")
	}

	mdPath := MarkdownPath(path)
	md, err := os.ReadFile(mdPath)
	if err != nil {
		t.Fatalf("read markdown failed: %v", err)
	}
	mdText := string(md)
	if !strings.Contains(mdText, "# Debate Result") {
		t.Fatalf("expected markdown title, got %q", mdText)
	}
	if !strings.Contains(mdText, "## Turns") {
		t.Fatalf("expected turns section, got %q", mdText)
	}
	if !strings.Contains(mdText, "### TOC (turn order)") {
		t.Fatalf("expected turns toc section, got %q", mdText)
	}
	if !strings.Contains(mdText, "[Turn 1 · A (persona)](#turn-1)") || !strings.Contains(mdText, "[Turn 2 · 사회자 (moderator)](#turn-2)") {
		t.Fatalf("expected turn-order toc anchors, got %q", mdText)
	}
	if !strings.Contains(mdText, "<a id=\"turns-speaker-1\"></a>") || !strings.Contains(mdText, "<a id=\"turns-speaker-2\"></a>") {
		t.Fatalf("expected explicit speaker anchors, got %q", mdText)
	}
	if !strings.Contains(mdText, "<details open>") || !strings.Contains(mdText, "</details>") {
		t.Fatalf("expected collapsible details blocks, got %q", mdText)
	}
	if !strings.Contains(mdText, "- test problem") || !strings.Contains(mdText, "- second line") {
		t.Fatalf("expected bulleted problem lines, got %q", mdText)
	}
	if !strings.Contains(mdText, "#### Turn 1 · A (persona)") || !strings.Contains(mdText, "#### Turn 2 · 사회자 (moderator)") {
		t.Fatalf("expected per-turn headers in speaker groups, got %q", mdText)
	}
	if !strings.Contains(mdText, "- content:\n  - first point\n  - second point") {
		t.Fatalf("expected bulleted turn content, got %q", mdText)
	}
	if !strings.Contains(mdText, "- aligned") || !strings.Contains(mdText, "- with constraints") {
		t.Fatalf("expected bulleted consensus summary, got %q", mdText)
	}
	if !strings.Contains(mdText, "---") {
		t.Fatalf("expected turn separator, got %q", mdText)
	}
	if !strings.Contains(mdText, "first point") {
		t.Fatalf("expected turn content in markdown, got %q", mdText)
	}

	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Fatalf("expected no temp file left, got err=%v", err)
	}
	if _, err := os.Stat(mdPath + ".tmp"); !os.IsNotExist(err) {
		t.Fatalf("expected no markdown temp file left, got err=%v", err)
	}
	if leftovers, err := filepath.Glob(path + ".tmp-*"); err != nil {
		t.Fatalf("glob json leftovers: %v", err)
	} else if len(leftovers) > 0 {
		t.Fatalf("expected no random json temp leftovers, got %v", leftovers)
	}
	if leftovers, err := filepath.Glob(mdPath + ".tmp-*"); err != nil {
		t.Fatalf("glob markdown leftovers: %v", err)
	} else if len(leftovers) > 0 {
		t.Fatalf("expected no random markdown temp leftovers, got %v", leftovers)
	}
}

func TestNewTimestampPath(t *testing.T) {
	now := time.Date(2026, 2, 28, 10, 30, 20, 123456789, time.UTC)
	path := NewTimestampPath("./outputs", now)
	if filepath.Base(path) != "20260228-103020.123456789-debate.json" {
		t.Fatalf("unexpected path: %s", path)
	}
}

func TestMarkdownPath(t *testing.T) {
	if got := MarkdownPath("./outputs/a-debate.json"); got != "./outputs/a-debate.md" {
		t.Fatalf("unexpected markdown path: %s", got)
	}
	if got := MarkdownPath("./outputs/result"); got != "./outputs/result.md" {
		t.Fatalf("unexpected markdown path without extension: %s", got)
	}
}

func TestGroupTurnsBySpeakerUsesSpeakerIDKey(t *testing.T) {
	turns := []orchestrator.Turn{
		{Index: 1, SpeakerID: "p1", SpeakerName: "Alex", Type: orchestrator.TurnTypePersona, Content: "a"},
		{Index: 2, SpeakerID: "p2", SpeakerName: "Alex", Type: orchestrator.TurnTypePersona, Content: "b"},
		{Index: 3, SpeakerID: "p1", SpeakerName: "Alex", Type: orchestrator.TurnTypePersona, Content: "c"},
	}

	groups := groupTurnsBySpeaker(turns)
	if len(groups) != 2 {
		t.Fatalf("expected 2 groups for same-name different-id speakers, got %d", len(groups))
	}
	if len(groups[0].Turns) != 2 {
		t.Fatalf("expected first speaker turns grouped by id, got %d", len(groups[0].Turns))
	}
	if len(groups[1].Turns) != 1 {
		t.Fatalf("expected second speaker turns grouped by id, got %d", len(groups[1].Turns))
	}
}

func TestGroupTurnsBySpeakerKeepsCaseSensitiveSpeakerID(t *testing.T) {
	turns := []orchestrator.Turn{
		{Index: 1, SpeakerID: "p1", SpeakerName: "Alex", Type: orchestrator.TurnTypePersona, Content: "a"},
		{Index: 2, SpeakerID: "P1", SpeakerName: "Alex", Type: orchestrator.TurnTypePersona, Content: "b"},
	}

	groups := groupTurnsBySpeaker(turns)
	if len(groups) != 2 {
		t.Fatalf("expected 2 groups for case-sensitive speaker IDs, got %d", len(groups))
	}
}

func TestFormatResultMarkdownEscapesHTMLSensitiveChars(t *testing.T) {
	result := orchestrator.Result{
		Problem: "<script>alert(1)</script>",
		Status:  orchestrator.StatusError,
		Turns: []orchestrator.Turn{
			{
				Index:       1,
				SpeakerName: "A&B",
				Type:        orchestrator.TurnTypePersona,
				Content:     "<b>unsafe</b>",
			},
		},
		Consensus: orchestrator.Consensus{
			Summary: "<div>summary</div>",
			Score:   0.3,
		},
	}

	md := formatResultMarkdown(result)
	if strings.Contains(md, "<script>") || strings.Contains(md, "<b>unsafe</b>") || strings.Contains(md, "<div>summary</div>") {
		t.Fatalf("expected html-sensitive chars to be escaped, got %q", md)
	}
	if !strings.Contains(md, "&lt;script&gt;alert(1)&lt;/script&gt;") {
		t.Fatalf("expected escaped problem text, got %q", md)
	}
	if !strings.Contains(md, "&lt;b&gt;unsafe&lt;/b&gt;") {
		t.Fatalf("expected escaped turn content, got %q", md)
	}
}

func TestMarkdownBulletedTextPreservesBlockquotePrefix(t *testing.T) {
	got := markdownBulletedText("> quote line\nnext line", "")
	if !strings.Contains(got, "> quote line") {
		t.Fatalf("expected blockquote to be preserved, got %q", got)
	}
	if !strings.Contains(got, "- next line") {
		t.Fatalf("expected regular line to become bullet, got %q", got)
	}
}
