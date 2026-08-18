package genvfile

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestRotateBackup_RenamesExistingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "genv.lock.json")
	if err := os.WriteFile(path, []byte("{}\n"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	if err := RotateBackup(path); err != nil {
		t.Fatalf("RotateBackup: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("original lock should be gone, stat err=%v", err)
	}
	matches, err := filepath.Glob(path + ".bak-*")
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("backups = %v, want 1", matches)
	}
}

func TestRotateBackup_MissingFileIsOK(t *testing.T) {
	path := filepath.Join(t.TempDir(), "genv.lock.json")
	if err := RotateBackup(path); err != nil {
		t.Fatalf("missing lock: %v", err)
	}
}

func TestRotateBackup_UnwritableParentIsFatal(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod is not POSIX on Windows")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "genv.lock.json")
	if err := os.WriteFile(path, []byte("{}\n"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })

	err := RotateBackup(path)
	if err == nil {
		t.Fatal("expected backup error on unwritable parent")
	}
	if !strings.Contains(err.Error(), "backing up") {
		t.Fatalf("error = %v, want backing up context", err)
	}
}
