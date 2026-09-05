package genvfile

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// LockMutation acquires an exclusive cross-process lock for mutating lockPath.
// The returned unlock function releases it and removes the sidecar mutex file.
// Safe to call with an empty path.
func LockMutation(lockPath string) (unlock func(), err error) {
	if lockPath == "" {
		return func() {}, nil
	}
	dir := filepath.Dir(lockPath)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("creating parent directory for lock file (%s): %w", dir, err)
	}
	mutexPath := lockPath + ".mutex"
	f, err := acquireMutex(mutexPath)
	if err != nil {
		return nil, fmt.Errorf("locking %s: %w", lockPath, err)
	}
	return func() {
		releaseMutex(f, mutexPath)
	}, nil
}

// acquireMutex opens mutexPath and takes an exclusive flock. After waiting it
// re-checks that the path still names the locked inode so an unlocker that
// unlinked the sidecar cannot race a new creator onto a different inode.
func acquireMutex(mutexPath string) (*os.File, error) {
	for {
		f, err := os.OpenFile(mutexPath, os.O_CREATE|os.O_RDWR, 0o600)
		if err != nil {
			return nil, err
		}
		if err := flockExclusive(f); err != nil {
			_ = f.Close()
			return nil, err
		}
		held, err := f.Stat()
		if err != nil {
			_ = flockUnlock(f)
			_ = f.Close()
			return nil, err
		}
		onDisk, err := os.Stat(mutexPath)
		if err == nil && os.SameFile(held, onDisk) {
			return f, nil
		}
		_ = flockUnlock(f)
		_ = f.Close()
	}
}

func releaseMutex(f *os.File, mutexPath string) {
	// Unlink while still holding the flock on Unix so a waiter that then
	// acquires the stale inode fails the SameFile check and retries. Windows
	// cannot remove an open file, so unlink after close instead.
	if runtime.GOOS != "windows" {
		_ = os.Remove(mutexPath)
	}
	_ = flockUnlock(f)
	_ = f.Close()
	if runtime.GOOS == "windows" {
		_ = os.Remove(mutexPath)
	}
}
