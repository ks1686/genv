package commands

import (
	"errors"
	"fmt"

	"github.com/ks1686/gpm/internal/schema"
)

// ErrAlreadyTracked is returned by Add when the package ID already exists.
var ErrAlreadyTracked = errors.New("package already tracked")

// Add appends a new package entry to f.
//
//   - id must be non-empty.
//   - version may be empty (omitted from the file, meaning "any").
//   - prefer may be empty or a known manager name.
//   - managers may be nil; each key must be a known manager name.
//
// Returns ErrAlreadyTracked if the ID is already present.
func Add(f *schema.GpmFile, id, version, prefer string, managers map[string]string) error {
	if id == "" {
		return fmt.Errorf("package id must not be empty")
	}

	for _, p := range f.Packages {
		if p.ID == id {
			return fmt.Errorf("%w: %q (use 'gpm remove %s' first to re-add it)", ErrAlreadyTracked, id, id)
		}
	}

	if prefer != "" && !schema.KnownManagers[prefer] {
		return fmt.Errorf("unknown manager %q for --prefer; valid managers: %s", prefer, knownManagerList())
	}

	for mgr := range managers {
		if !schema.KnownManagers[mgr] {
			return fmt.Errorf("unknown manager %q in --manager; valid managers: %s", mgr, knownManagerList())
		}
	}

	pkg := schema.Package{
		ID:      id,
		Version: version,
		Prefer:  prefer,
	}
	if len(managers) > 0 {
		pkg.Managers = managers
	}

	f.Packages = append(f.Packages, pkg)
	return nil
}
