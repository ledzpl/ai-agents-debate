package output

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"debate/internal/orchestrator"
)

func TestSaveResultWritesJSON(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "result.json")
	result := orchestrator.Result{
		Problem: "test problem",
		Status:  orchestrator.StatusMaxTurnsReached,
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

	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Fatalf("expected no temp file left, got err=%v", err)
	}
}

func TestNewTimestampPath(t *testing.T) {
	now := time.Date(2026, 2, 28, 10, 30, 20, 123456789, time.UTC)
	path := NewTimestampPath("./outputs", now)
	if filepath.Base(path) != "20260228-103020.123456789-debate.json" {
		t.Fatalf("unexpected path: %s", path)
	}
}
