package commands

import (
	"errors"
	"fmt"

	"github.com/ks1686/gpm/internal/schema"
)

// ErrNotTracked is returned by Remove when the package ID does not exist.
var ErrNotTracked = errors.New("package not tracked")

// Remove deletes the first package with the given id from f.
// Order of the remaining packages is preserved.
// Returns ErrNotTracked if the id is not found.
func Remove(f *schema.GpmFile, id string) error {
	if id == "" {
		return fmt.Errorf("package id must not be empty")
	}

	for i, p := range f.Packages {
		if p.ID == id {
			f.Packages = append(f.Packages[:i], f.Packages[i+1:]...)
			return nil
		}
	}

	return fmt.Errorf("%w: %q", ErrNotTracked, id)
}
