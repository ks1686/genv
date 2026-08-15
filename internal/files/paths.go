package files

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func resolveSource(sourceRoot, source string) (string, error) {
	if source == "" {
		return "", errors.New("source must not be empty")
	}
	expanded, err := expandPath(source)
	if err != nil {
		return "", err
	}
	if filepath.IsAbs(expanded) {
		return expanded, nil
	}
	if sourceRoot == "" {
		return "", errors.New("source is relative but SourceRoot is empty")
	}
	rootExpanded, err := expandPath(sourceRoot)
	if err != nil {
		return "", err
	}
	rootClean := filepath.Clean(rootExpanded)
	joined := filepath.Join(rootClean, expanded)
	rel, err := filepath.Rel(rootClean, joined)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("source %q escapes SourceRoot %s", source, rootClean)
	}
	return joined, nil
}

func expandPath(s string) (string, error) {
	if strings.HasPrefix(s, "~") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		s = home + s[1:]
	}
	return os.Expand(s, os.Getenv), nil
}

func ensureParentDir(target string) error {
	parent := filepath.Dir(target)
	if parent == "" || parent == "." || parent == target {
		return nil
	}
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", parent, err)
	}
	return nil
}
