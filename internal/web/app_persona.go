package web

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"debate/internal/persona"
)

func (a *App) resolvePersonas(personaPath string, inline []persona.Persona) ([]persona.Persona, string, error) {
	if len(inline) > 0 && strings.TrimSpace(personaPath) != "" {
		return nil, "", errors.New("persona_path and personas cannot be used together")
	}
	if len(inline) > 0 {
		normalized, err := persona.NormalizeAndValidate(inline)
		if err != nil {
			return nil, "", err
		}
		return normalized, "", nil
	}

	loaderPath, displayPath, err := a.resolvePersonaPath(personaPath)
	if err != nil {
		return nil, "", err
	}
	personas, err := a.loader(loaderPath)
	if err != nil {
		return nil, displayPath, err
	}
	normalized, err := persona.NormalizeAndValidate(personas)
	if err != nil {
		return nil, displayPath, err
	}
	return normalized, displayPath, nil
}

func (a *App) resolvePersonaPath(rawPath string) (loaderPath string, displayPath string, err error) {
	path := strings.TrimSpace(rawPath)
	if path == "" {
		path = strings.TrimSpace(a.personaPath)
	}
	if path == "" {
		return "", "", errors.New("persona path is required")
	}
	if !strings.EqualFold(filepath.Ext(path), ".json") {
		return "", "", errors.New("persona path must be a .json file")
	}

	cleanPath := filepath.Clean(path)
	candidateAbs := cleanPath
	if !filepath.IsAbs(candidateAbs) {
		candidateAbs = filepath.Join(a.baseDir, cleanPath)
	}
	candidateAbs, err = filepath.Abs(candidateAbs)
	if err != nil {
		return "", "", fmt.Errorf("abs path: %w", err)
	}

	baseForCheck, err := resolvePathForContainment(a.baseDir)
	if err != nil {
		return "", "", fmt.Errorf("resolve base path: %w", err)
	}
	candidateForCheck, err := resolvePathForContainment(candidateAbs)
	if err != nil {
		return "", "", fmt.Errorf("resolve persona path: %w", err)
	}
	isWithinBase, err := pathWithinBase(baseForCheck, candidateForCheck)
	if err != nil {
		return "", "", fmt.Errorf("relative path: %w", err)
	}
	if !isWithinBase {
		return "", "", errors.New("persona path must stay within the project directory")
	}

	relToBase, err := filepath.Rel(a.baseDir, candidateAbs)
	if err != nil {
		return "", "", fmt.Errorf("loader relative path: %w", err)
	}
	if relToBase == ".." || strings.HasPrefix(relToBase, ".."+string(filepath.Separator)) {
		return "", "", errors.New("persona path must stay within the project directory")
	}
	relToBase = filepath.Clean(relToBase)
	displayPath = filepath.ToSlash(relToBase)
	if !strings.HasPrefix(displayPath, ".") {
		displayPath = "." + string(filepath.Separator) + displayPath
		displayPath = filepath.ToSlash(displayPath)
	}
	loaderPath = candidateAbs
	return loaderPath, displayPath, nil
}

func resolvePathForContainment(path string) (string, error) {
	resolved, err := filepath.EvalSymlinks(path)
	switch {
	case err == nil:
		path = resolved
	case os.IsNotExist(err):
		// Keep original path for non-existent targets.
	default:
		return "", fmt.Errorf("evaluate symlink: %w", err)
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return filepath.Clean(absPath), nil
}

func pathWithinBase(baseAbs, candidateAbs string) (bool, error) {
	relToBase, err := filepath.Rel(baseAbs, candidateAbs)
	if err != nil {
		return false, err
	}
	if relToBase == ".." || strings.HasPrefix(relToBase, ".."+string(filepath.Separator)) {
		return false, nil
	}
	return true, nil
}
