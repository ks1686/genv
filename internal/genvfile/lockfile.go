package genvfile

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ks1686/genv/internal/schema"
)

// LockedPackage records how one package was last applied by genv: which manager
// was chosen, what concrete package name was passed to it, and the version that
// was installed. InstalledVersion is empty for entries written before M3.
type LockedPackage struct {
	ID               string `json:"id"`
	Manager          string `json:"manager"`
	PkgName          string `json:"pkgName"`
	InstalledVersion string `json:"installedVersion,omitempty"`
}

// LockFile is the on-disk representation of genv's applied state.
type LockFile struct {
	SchemaVersion string          `json:"schemaVersion"`
	Packages      []LockedPackage `json:"packages"`
}

// ReadLock reads the lock file at path. If the file does not exist (first run),
// it returns an empty LockFile with no error — the caller treats that as a
// clean slate and will install everything in genv.json.
func ReadLock(path string) (*LockFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &LockFile{SchemaVersion: schema.SchemaVersion}, nil
		}
		return nil, err
	}
	var lf LockFile
	if err := json.Unmarshal(data, &lf); err != nil {
		return nil, err
	}
	return &lf, nil
}

// WriteLock atomically writes lf to path using a temp-file + rename, matching
// the same safety pattern as Write for genv.json.
func WriteLock(path string, lf *LockFile) error {
	data, err := json.MarshalIndent(lf, "", "  ")
	if err != nil {
		return fmt.Errorf("serialising lock file: %w", err)
	}
	data = append(data, '\n')
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("creating parent directory for lock file (%s): %w", dir, err)
	}

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
