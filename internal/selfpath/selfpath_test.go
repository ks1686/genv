package selfpath

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPreferStable_prefersPATHLookupOverVersionedCaskroomPath(t *testing.T) {
	dir := t.TempDir()
	caskroom := filepath.Join(dir, "Caskroom", "genv", "4.0.3")
	binDir := filepath.Join(dir, "bin")
	if err := os.MkdirAll(caskroom, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	realBin := filepath.Join(caskroom, "genv")
	if err := os.WriteFile(realBin, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	stable := filepath.Join(binDir, "genv")
	if err := os.Symlink(realBin, stable); err != nil {
		t.Fatal(err)
	}

	lookPath := func(file string) (string, error) {
		if file == "genv" {
			return stable, nil
		}
		return "", os.ErrNotExist
	}

	// Simulate Homebrew cask post_install: process started via staged_path.
	got := PreferStable(realBin, realBin, lookPath)
	if got != stable {
		t.Fatalf("PreferStable = %q, want stable symlink %q", got, stable)
	}
}

func TestPreferStable_doesNotPreferDifferentBinaryOnPATH(t *testing.T) {
	dir := t.TempDir()
	running := filepath.Join(dir, "build", "genv")
	other := filepath.Join(dir, "bin", "genv")
	if err := os.MkdirAll(filepath.Dir(running), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(other), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(running, []byte("a"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(other, []byte("b"), 0o755); err != nil {
		t.Fatal(err)
	}

	lookPath := func(file string) (string, error) {
		if file == "genv" {
			return other, nil
		}
		return "", os.ErrNotExist
	}

	got := PreferStable(running, running, lookPath)
	if got != running {
		t.Fatalf("PreferStable = %q, want running binary %q (PATH hit is a different inode)", got, running)
	}
}

func TestPreferStable_keepsUnresolvedStablePathWhenAlreadyStable(t *testing.T) {
	dir := t.TempDir()
	caskroom := filepath.Join(dir, "Caskroom", "genv", "4.0.3")
	binDir := filepath.Join(dir, "bin")
	if err := os.MkdirAll(caskroom, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	realBin := filepath.Join(caskroom, "genv")
	if err := os.WriteFile(realBin, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	stable := filepath.Join(binDir, "genv")
	if err := os.Symlink(realBin, stable); err != nil {
		t.Fatal(err)
	}

	lookPath := func(file string) (string, error) {
		if file == "genv" {
			return stable, nil
		}
		return "", os.ErrNotExist
	}

	got := PreferStable(stable, stable, lookPath)
	if got != stable {
		t.Fatalf("PreferStable = %q, want %q", got, stable)
	}
}

func TestPreferStable_fallsBackWhenNotOnPATH(t *testing.T) {
	dir := t.TempDir()
	running := filepath.Join(dir, "genv")
	if err := os.WriteFile(running, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	lookPath := func(string) (string, error) { return "", os.ErrNotExist }
	got := PreferStable(running, running, lookPath)
	if got != running {
		t.Fatalf("PreferStable = %q, want %q", got, running)
	}
}

func TestPreferStable_emptyExe(t *testing.T) {
	if got := PreferStable("", "/bin/genv", nil); got != "" {
		t.Fatalf("got %q, want empty", got)
	}
}

func TestPreferStable_nilLookPathUsesExecLookPath(t *testing.T) {
	// Cover the nil lookPath default without asserting a specific PATH hit:
	// when LookPath fails or points elsewhere, PreferStable returns exe.
	dir := t.TempDir()
	running := filepath.Join(dir, "unique-genv-not-on-path-"+filepath.Base(t.TempDir()))
	if err := os.WriteFile(running, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	got := PreferStable(running, running, nil)
	if got != running {
		t.Fatalf("PreferStable = %q, want %q", got, running)
	}
}

func TestPreferStable_prefersAbsoluteArg0WhenSameFile(t *testing.T) {
	dir := t.TempDir()
	realBin := filepath.Join(dir, "real", "genv")
	link := filepath.Join(dir, "link", "genv")
	if err := os.MkdirAll(filepath.Dir(realBin), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(realBin, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(realBin, link); err != nil {
		t.Fatal(err)
	}
	lookPath := func(string) (string, error) { return "", os.ErrNotExist }
	got := PreferStable(realBin, link, lookPath)
	if got != link {
		t.Fatalf("PreferStable = %q, want arg0 symlink %q", got, link)
	}
}
