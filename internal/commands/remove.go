package commands

import (
	"errors"
	"fmt"

	"github.com/ks1686/genv/internal/schema"
)

// ErrNotTracked is returned by Remove when the package ID does not exist.
var ErrNotTracked = errors.New("package not tracked")

// Remove deletes the first package with the given id from f.
// Order of the remaining packages is preserved.
// Returns ErrNotTracked if the id is not found.
func Remove(f *schema.GenvFile, id, targetID string) error {
	if id == "" {
		return fmt.Errorf("package id must not be empty")
	}

	packages := &f.Packages
	if f.SchemaVersion == schema.Version8 {
		targetPackages, err := activePackageSlice(f, targetID)
		if err != nil {
			return err
		}
		packages = targetPackages
	}

	for i, p := range *packages {
		if p.ID == id {
			*packages = append((*packages)[:i], (*packages)[i+1:]...)
			return nil
		}
	}

	return fmt.Errorf("%w: %q", ErrNotTracked, id)
}
