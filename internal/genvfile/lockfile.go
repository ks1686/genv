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

// LockedEnvVar records how one environment variable was last applied by genv.
// Sensitive is preserved so status/output can redact the value as needed.
type LockedEnvVar struct {
	Name      string `json:"name"`
	Value     string `json:"value"`
	Sensitive bool   `json:"sensitive,omitempty"`
}

// LockedShellAlias records a single applied shell alias.
type LockedShellAlias struct {
	Name  string `json:"name"`
	Value string `json:"value"`
	Shell string `json:"shell,omitempty"`
}

// LockedShellFunction records a single applied shell function.
type LockedShellFunction struct {
	Name  string `json:"name"`
	Body  string `json:"body"`
	Shell string `json:"shell,omitempty"`
}

// LockedShellConfig records the applied shell configuration block.
type LockedShellConfig struct {
	Aliases   []LockedShellAlias    `json:"aliases,omitempty"`
	Functions []LockedShellFunction `json:"functions,omitempty"`
	Source    []string              `json:"source,omitempty"`
}

// LockFile is the on-disk representation of the applied state tracked by genv.
// The Env field is added in M8 (schemaVersion "2") and is absent in v1 lock files.
// The Shell field is added in M9 (schemaVersion "3") and is absent in v1/v2 lock files.
type LockFile struct {
	SchemaVersion string             `json:"schemaVersion"`
	Packages      []LockedPackage    `json:"packages"`
	Env           []LockedEnvVar     `json:"env,omitempty"`
	Shell         *LockedShellConfig `json:"shell,omitempty"`
}

// ReadLock reads the lock file at path. If the file does not exist (first run),
// it returns an empty LockFile with no error — the caller treats that as a
// clean slate and will install everything in genv.json.
func ReadLock(path string) (*LockFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &LockFile{SchemaVersion: schema.Version}, nil
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
		return fmt.Errorf("serializing lock file: %w", err)
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
