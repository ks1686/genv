package genvfile

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLockMutation_CreatesMutexAndUnlocks(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "genv.lock.json")

	unlock, err := LockMutation(path)
	if err != nil {
		t.Fatalf("LockMutation: %v", err)
	}
	if _, err := os.Stat(path + ".mutex"); err != nil {
		t.Fatalf("mutex file missing: %v", err)
	}
	unlock()
}

func TestLockMutation_EmptyPathIsNoop(t *testing.T) {
	unlock, err := LockMutation("")
	if err != nil {
		t.Fatalf("empty path: %v", err)
	}
	unlock()
}
