// Package target resolves the portable target ID for v8 specs.
package target

import (
	"fmt"
	"os"
	"strings"

	"github.com/ks1686/genv/internal/host"
)

// Resolve returns the target ID selected by an explicit flag, GENV_TARGET, or
// host classification, in that order.
func Resolve(flag string) (string, error) {
	return resolve(flag, os.Getenv("GENV_TARGET"), host.Classify)
}

func resolve(flag, envTarget string, classify func() (string, error)) (string, error) {
	if target := strings.TrimSpace(flag); target != "" {
		return target, nil
	}
	if target := strings.TrimSpace(envTarget); target != "" {
		return target, nil
	}
	target, err := classify()
	if err != nil {
		return "", fmt.Errorf("resolve target: pass --target, set GENV_TARGET, or use a supported host target: %w", err)
	}
	return target, nil
}
