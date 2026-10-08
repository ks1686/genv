package genvfile

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeOversizeFile(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	// A syntactically valid prefix proves the limit is enforced before JSON
	// parsing, not accidentally by an unrelated decoder failure.
	data := `{"schemaVersion":"1","packages":[]}` + strings.Repeat(" ", int(MaxFileBytes))
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestReadRejectsOversizeSpec(t *testing.T) {
	path := writeOversizeFile(t, "genv.json")
	_, err := Read(path)
	if !errors.Is(err, ErrTooLarge) {
		t.Fatalf("Read() error = %v, want ErrTooLarge", err)
	}
	if !strings.Contains(err.Error(), path) || !strings.Contains(err.Error(), "limit") {
		t.Errorf("error = %q, want path and limit", err)
	}
}

func TestReadLockRejectsOversizeLock(t *testing.T) {
	path := writeOversizeFile(t, "genv.lock.json")
	_, err := ReadLock(path)
	if !errors.Is(err, ErrTooLarge) {
		t.Fatalf("ReadLock() error = %v, want ErrTooLarge", err)
	}
	if !strings.Contains(err.Error(), path) || !strings.Contains(err.Error(), "limit") {
		t.Errorf("error = %q, want path and limit", err)
	}
}

func TestReadFileLimitedAcceptsExactLimit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "exact.json")
	data := []byte(`{}` + strings.Repeat(" ", int(MaxFileBytes)-2))
	if int64(len(data)) != MaxFileBytes {
		t.Fatalf("fixture length = %d, want %d", len(data), MaxFileBytes)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := readFileLimited(path)
	if err != nil {
		t.Fatalf("readFileLimited exact limit: %v", err)
	}
	if len(got) != len(data) {
		t.Fatalf("read length = %d, want %d", len(got), len(data))
	}
}
