package complete

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

func TestReadWriteDump_roundTrip(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	names := []string{"git", "wget"}
	if err := WriteDump("brew", names); err != nil {
		t.Fatal(err)
	}
	got, hit := ReadDump("brew")
	if !hit {
		t.Fatal("expected hit")
	}
	if !slices.Equal(got, names) {
		t.Fatalf("got %v want %v", got, names)
	}
}

func TestReadDump_expired(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if err := WriteDump("brew", []string{"git"}); err != nil {
		t.Fatal(err)
	}
	dir, err := CacheDir()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, DumpFilename("brew"))
	past := time.Now().Add(-CacheTTL - time.Hour)
	if err := os.Chtimes(path, past, past); err != nil {
		t.Fatal(err)
	}
	_, hit := ReadDump("brew")
	if hit {
		t.Fatal("expected miss after TTL")
	}
}

func TestReadDump_corrupt(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	dir, err := CacheDir()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, DumpFilename("brew"))
	if err := os.WriteFile(path, []byte("git\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0); err != nil {
		t.Fatal(err)
	}
	_, hit := ReadDump("brew")
	if hit {
		t.Fatal("expected miss for unreadable dump")
	}
	_, hit = ReadDump("missing-manager")
	if hit {
		t.Fatal("expected miss for missing manager")
	}
}

func TestReadDump_invalidManager(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	_, hit := ReadDump("../evil")
	if hit {
		t.Fatal("expected miss for invalid manager")
	}
	_, hit = ReadDump("foo/bar")
	if hit {
		t.Fatal("expected miss for invalid manager")
	}
}

func TestWriteDump_invalidManager(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if err := WriteDump("../evil", []string{"git"}); err == nil {
		t.Fatal("expected error for invalid manager")
	}
	if err := WriteDump("foo/bar", []string{"git"}); err == nil {
		t.Fatal("expected error for invalid manager")
	}
}
