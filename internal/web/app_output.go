package web

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"

	"debate/internal/orchestrator"
	"debate/internal/output"
	"debate/internal/persona"
)

func (a *App) runAndSaveDebate(ctx context.Context, problem string, personas []persona.Persona, runCfg *orchestrator.Config, onTurn func(orchestrator.Turn)) (debateResponse, error) {
	var (
		result orchestrator.Result
		err    error
	)
	if runCfg != nil {
		configurableRunner, ok := a.runner.(ConfigurableRunner)
		if !ok {
			return debateResponse{}, fmt.Errorf("runtime tuning is not supported by the current runner")
		}
		result, err = configurableRunner.RunWithConfig(ctx, problem, personas, *runCfg, onTurn)
	} else {
		result, err = a.runner.Run(ctx, problem, personas, onTurn)
	}
	if err != nil {
		return debateResponse{}, fmt.Errorf("run debate: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return debateResponse{}, fmt.Errorf("debate canceled before save: %w", err)
	}

	savePath, err := a.nextOutputPath()
	if err != nil {
		return debateResponse{}, fmt.Errorf("prepare output path: %w", err)
	}
	if err := output.SaveResult(savePath, result); err != nil {
		return debateResponse{}, fmt.Errorf("save result: %w", err)
	}

	return debateResponse{
		Result:            result,
		SavedJSONPath:     savePath,
		SavedMarkdownPath: output.MarkdownPath(savePath),
	}, nil
}

func (a *App) nextOutputPath() (string, error) {
	basePath := output.NewTimestampPath(a.outputDir, a.now())
	ext := filepath.Ext(basePath)
	stem := strings.TrimSuffix(basePath, ext)

	for {
		seq := atomic.AddUint64(&a.outputSeq, 1)
		candidate := basePath
		if seq > 1 {
			candidate = fmt.Sprintf("%s-%06d%s", stem, seq-1, ext)
		}
		available, err := pathAvailable(candidate)
		if err != nil {
			return "", err
		}
		if available {
			return candidate, nil
		}
	}
}

func pathAvailable(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return false, nil
	}
	if os.IsNotExist(err) {
		return true, nil
	}
	return false, fmt.Errorf("stat output path %q: %w", path, err)
}
