// Package genvfile handles reading and writing genv.json files.
// It is the only package that performs filesystem I/O on the manifest.
package genvfile

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ks1686/genv/internal/schema"
)

// DefaultDir returns the platform-appropriate configuration directory for genv.
// It respects $XDG_CONFIG_HOME; otherwise falls back to ~/.config.
func DefaultDir() (string, error) {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "genv"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine home directory: %w", err)
	}
	return filepath.Join(home, ".config", "genv"), nil
}

// DefaultSpecPath returns the default path for genv.json inside the genv config
// directory (~/.config/genv/genv.json, or $XDG_CONFIG_HOME/genv/genv.json).
func DefaultSpecPath() (string, error) {
	dir, err := DefaultDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "genv.json"), nil
}

// LockPathFrom returns the per-host lock file path inside the genv config
// directory (~/.config/genv/genv.lock.json, or $XDG_CONFIG_HOME/genv/genv.lock.json
// when set). The specPath argument is ignored so the lock lives outside the spec
// repo by default.
func LockPathFrom(specPath string) string {
	dir, err := DefaultDir()
	if err != nil {
		return "genv.lock.json"
	}
	return filepath.Join(dir, "genv.lock.json")
}

// ErrNotFound is returned by Read when the file does not exist.
var ErrNotFound = errors.New("genv.json not found")

// ErrInvalidFile is returned by Read when the file exists but fails schema
// validation or JSON parsing. Wrap-chain: callers can use errors.Is to
// distinguish this from I/O errors and route to the correct exit code.
var ErrInvalidFile = errors.New("invalid genv.json")

// Read loads, parses, and validates the genv.json at path.
// Returns ErrNotFound if the file is absent.
// Returns a descriptive error (with line info) for parse or validation failures.
func Read(path string) (*schema.GenvFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}

	f, valErrs, parseErr := schema.ParseAndValidate(data)
	if parseErr != nil {
		return nil, fmt.Errorf("%w: %s: %w", ErrInvalidFile, path, parseErr)
	}
	if len(valErrs) > 0 {
		msgs := make([]string, len(valErrs))
		for i, e := range valErrs {
			msgs[i] = e.Error()
		}
		return nil, fmt.Errorf("%w: %s: validation errors:\n  %s", ErrInvalidFile, path, strings.Join(msgs, "\n  "))
	}
	return f, nil
}

// ReadOrNew reads the genv.json at path. If the file does not exist, it returns
// a new empty GenvFile and reports isNew=true. Other errors are returned as-is.
func ReadOrNew(path string) (f *schema.GenvFile, isNew bool, err error) {
	f, err = Read(path)
	if errors.Is(err, ErrNotFound) {
		return New(), true, nil
	}
	return f, false, err
}

// New returns a minimal, valid schemaVersion 8 GenvFile with empty defaults
// and a bucket for every known target so first-run add/init can persist
// without guessing a single host.
func New() *schema.GenvFile {
	targets := make(map[string]*schema.TargetBundle, len(schema.KnownTargets))
	for id := range schema.KnownTargets {
		targets[id] = &schema.TargetBundle{}
	}
	return &schema.GenvFile{
		SchemaVersion: schema.Version8,
		Defaults:      &schema.TargetBundle{},
		Targets:       targets,
	}
}

// Write serializes f to path with 2-space indentation.
// Writing is atomic: it writes to a temp file then renames, so a crash
// mid-write cannot leave a half-written genv.json.
func Write(path string, f *schema.GenvFile) error {
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return fmt.Errorf("serialising genv.json: %w", err)
	}
	data = append(data, '\n')

	_, valErrs, parseErr := schema.ParseAndValidate(data)
	if parseErr != nil {
		return fmt.Errorf("%w: %s: %w", ErrInvalidFile, path, parseErr)
	}
	if len(valErrs) > 0 {
		msgs := make([]string, len(valErrs))
		for i, e := range valErrs {
			msgs[i] = e.Error()
		}
		return fmt.Errorf("%w: %s: validation errors:\n  %s", ErrInvalidFile, path, strings.Join(msgs, "\n  "))
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("creating directory %s: %w", dir, err)
	}

	tmp := path + ".tmp"
	tmpF, err := os.Create(tmp)
	if err != nil {
		return fmt.Errorf("writing %s: %w", tmp, err)
	}
	if _, err := tmpF.Write(data); err != nil {
		_ = tmpF.Close()
		_ = os.Remove(tmp)
		return fmt.Errorf("writing %s: %w", tmp, err)
	}
	// Sync while the write handle is still open — on Windows,
	// FlushFileBuffers denies read-only handles, so this cannot be done via
	// a reopen.
	if err := tmpF.Sync(); err != nil {
		_ = tmpF.Close()
		_ = os.Remove(tmp)
		return fmt.Errorf("syncing %s: %w", tmp, err)
	}
	if err := tmpF.Close(); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("writing %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("saving %s: %w", path, err)
	}
	syncDir(dir)
	return nil
}
