// Package gpmfile handles reading and writing gpm.json files.
// It is the only package that performs filesystem I/O on the manifest.
package gpmfile

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/ks1686/gpm/internal/schema"
)

// DefaultPath is the conventional name for a gpm manifest.
const DefaultPath = "gpm.json"

// ErrNotFound is returned by Read when the file does not exist.
var ErrNotFound = errors.New("gpm.json not found")

// ErrInvalidFile is returned by Read when the file exists but fails schema
// validation or JSON parsing. Wrap-chain: callers can use errors.Is to
// distinguish this from I/O errors and route to the correct exit code.
var ErrInvalidFile = errors.New("invalid gpm.json")

// Read loads, parses, and validates the gpm.json at path.
// Returns ErrNotFound if the file is absent.
// Returns a descriptive error (with line info) for parse or validation failures.
func Read(path string) (*schema.GpmFile, error) {
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

// ReadOrNew reads the gpm.json at path. If the file does not exist, it returns
// a new empty GpmFile and reports isNew=true. Other errors are returned as-is.
func ReadOrNew(path string) (f *schema.GpmFile, isNew bool, err error) {
	f, err = Read(path)
	if errors.Is(err, ErrNotFound) {
		return New(), true, nil
	}
	return f, false, err
}

// New returns a minimal, valid GpmFile ready to be populated.
func New() *schema.GpmFile {
	return &schema.GpmFile{
		SchemaVersion: schema.SchemaVersion,
		Packages:      []schema.Package{},
	}
}

// Write serialises f to path with 2-space indentation.
// The write is atomic: it writes to a temp file then renames, so a crash
// mid-write cannot leave a half-written gpm.json.
func Write(path string, f *schema.GpmFile) error {
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return fmt.Errorf("serialising gpm.json: %w", err)
	}
	data = append(data, '\n')

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("saving %s: %w", path, err)
	}
	return nil
}
