package complete

import (
	"os"
	"path/filepath"
	"runtime"
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

func TestReadDump_emptyFileIsMiss(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	dir, err := CacheDir()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, DumpFilename("brew"))
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	if names, hit := ReadDump("brew"); hit || names != nil {
		t.Fatalf("ReadDump() = %v, %v; want nil, false", names, hit)
	}
}

func TestWriteDump_emptyDoesNotCreateDump(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	for _, names := range [][]string{nil, {}} {
		if err := WriteDump("brew", names); err != nil {
			t.Fatal(err)
		}
	}

	dir, err := CacheDir()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, DumpFilename("brew"))
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("Stat() error = %v, want not exist", err)
	}
}

func TestWriteDump_atomicallyReplacesExistingDump(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if err := WriteDump("brew", []string{"old"}); err != nil {
		t.Fatal(err)
	}
	if err := WriteDump("brew", []string{"new", "names"}); err != nil {
		t.Fatal(err)
	}

	got, hit := ReadDump("brew")
	if !hit || !slices.Equal(got, []string{"new", "names"}) {
		t.Fatalf("ReadDump() = %v, %v; want [new names], true", got, hit)
	}
	dir, err := CacheDir()
	if err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != DumpFilename("brew") {
		t.Fatalf("cache entries = %v, want only brew.txt", entries)
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
	if runtime.GOOS == "windows" {
		t.Skip("chmod 0 does not deny the file owner on Windows")
	}
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
