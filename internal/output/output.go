package output

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"debate/internal/orchestrator"
)

func SaveResult(path string, result orchestrator.Result) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create output dir: %w", err)
	}
	jsonPathExisted := false
	if _, err := os.Stat(path); err == nil {
		jsonPathExisted = true
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat json result file: %w", err)
	}

	jsonData, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal result: %w", err)
	}
	if err := writeAtomic(path, jsonData, 0o644); err != nil {
		return fmt.Errorf("write json result file: %w", err)
	}

	mdPath := MarkdownPath(path)
	mdData := []byte(formatResultMarkdown(result))
	if err := writeAtomic(mdPath, mdData, 0o644); err != nil {
		// Avoid leaving half-written artifacts when markdown write fails.
		if !jsonPathExisted {
			_ = os.Remove(path)
		}
		return fmt.Errorf("write markdown result file: %w", err)
	}
	return nil
}

func MarkdownPath(path string) string {
	ext := filepath.Ext(path)
	if ext == "" {
		return path + ".md"
	}
	return strings.TrimSuffix(path, ext) + ".md"
}

func writeAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	base := filepath.Base(path)
	tempFile, err := os.CreateTemp(dir, base+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tempPath := tempFile.Name()
	cleanup := func() {
		_ = tempFile.Close()
		_ = os.Remove(tempPath)
	}

	if err := tempFile.Chmod(perm); err != nil {
		cleanup()
		return fmt.Errorf("chmod temp file: %w", err)
	}
	if _, err := tempFile.Write(data); err != nil {
		cleanup()
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := tempFile.Sync(); err != nil {
		cleanup()
		return fmt.Errorf("sync temp file: %w", err)
	}
	if err := tempFile.Close(); err != nil {
		_ = os.Remove(tempPath)
		return fmt.Errorf("close temp file: %w", err)
	}
	if err := os.Rename(tempPath, path); err != nil {
		_ = os.Remove(tempPath)
		return fmt.Errorf("move temp file: %w", err)
	}
	return nil
}

func NewTimestampPath(dir string, now time.Time) string {
	name := now.UTC().Format("20060102-150405.000000000") + "-debate.json"
	return filepath.Join(dir, name)
}
