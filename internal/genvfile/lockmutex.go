package genvfile

import (
	"fmt"
	"os"
	"path/filepath"
)

// LockMutation acquires an exclusive cross-process lock for mutating lockPath.
// The returned unlock function releases it. Safe to call with an empty path.
func LockMutation(lockPath string) (unlock func(), err error) {
	if lockPath == "" {
		return func() {}, nil
	}
	dir := filepath.Dir(lockPath)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("creating parent directory for lock file (%s): %w", dir, err)
	}
	f, err := os.OpenFile(lockPath+".mutex", os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("locking %s: %w", lockPath, err)
	}
	if err := flockExclusive(f); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("locking %s: %w", lockPath, err)
	}
	return func() {
		_ = flockUnlock(f)
		_ = f.Close()
	}, nil
}
