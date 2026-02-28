package output

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"debate/internal/orchestrator"
)

func SaveResult(path string, result orchestrator.Result) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create output dir: %w", err)
	}

	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal result: %w", err)
	}

	tempPath := path + ".tmp"
	if err := os.WriteFile(tempPath, data, 0o644); err != nil {
		return fmt.Errorf("write temp result file: %w", err)
	}
	if err := os.Rename(tempPath, path); err != nil {
		_ = os.Remove(tempPath)
		return fmt.Errorf("move temp result file: %w", err)
	}
	return nil
}

func NewTimestampPath(dir string, now time.Time) string {
	name := now.UTC().Format("20060102-150405.000000000") + "-debate.json"
	return filepath.Join(dir, name)
}
