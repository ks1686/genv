package profile

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ks1686/genv/internal/genvfile"
	"github.com/ks1686/genv/internal/schema"
)

// ErrProfileNotFound is returned when a requested profile does not exist.
var ErrProfileNotFound = errors.New("profile not found")

// Dir returns the directory where profiles are stored, given the base spec path.
func Dir(specPath string) string {
	return filepath.Join(filepath.Dir(specPath), "profiles")
}

// Path returns the file path for a named profile.
func Path(specPath, name string) string {
	return filepath.Join(Dir(specPath), name+".json")
}

// List returns the names of all available profiles (excluding base).
func List(specPath string) ([]string, error) {
	dir := Dir(specPath)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading profiles directory: %w", err)
	}

	var profiles []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") {
			profiles = append(profiles, strings.TrimSuffix(e.Name(), ".json"))
		}
	}
	return profiles, nil
}

// Load reads a named profile.
func Load(specPath, name string) (*schema.GenvFile, error) {
	path := Path(specPath, name)
	f, err := genvfile.Read(path)
	if err != nil {
		if errors.Is(err, genvfile.ErrNotFound) {
			return nil, fmt.Errorf("%w: %s", ErrProfileNotFound, name)
		}
		return nil, err
	}
	return f, nil
}

// LoadMerged reads the base spec and the named profile, and returns the merged result.
// If name is empty or "base", it just returns the base spec.
func LoadMerged(specPath, name string) (*schema.GenvFile, error) {
	base, err := genvfile.Read(specPath)
	if err != nil {
		return nil, err
	}
	if name == "" || name == "base" {
		return base, nil
	}
	ext, err := Load(specPath, name)
	if err != nil {
		return nil, err
	}
	return Merge(base, ext), nil
}

// Create scaffolds a new empty profile.
func Create(specPath, name string) error {
	path := Path(specPath, name)
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("profile %q already exists", name)
	}

	f := &schema.GenvFile{
		SchemaVersion: schema.Version6,
		Packages:      []schema.Package{},
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("creating profiles directory: %w", err)
	}

	return genvfile.Write(path, f)
}

// BaseProfileName is the reserved name for the root genv.json profile.
const BaseProfileName = "base"
