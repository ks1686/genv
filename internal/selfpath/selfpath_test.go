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

func TestPreferStable_prefersBrewBinEvenWhenSymlinkDangling(t *testing.T) {
	// During cask upgrade, bin/genv can briefly dangle at the purged version while
	// post_install runs the new Caskroom binary — SameFile must not be required.
	dir := t.TempDir()
	prefix := filepath.Join(dir, "opt", "homebrew")
	caskroom := filepath.Join(prefix, "Caskroom", "genv", "4.0.4")
	binDir := filepath.Join(prefix, "bin")
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
	if err := os.Symlink(filepath.Join(prefix, "Caskroom", "genv", "4.0.3", "genv"), stable); err != nil {
		t.Fatal(err)
	}
	// Dangling: 4.0.3 target does not exist.
	lookPath := func(string) (string, error) { return "", os.ErrNotExist }

	got := PreferStable(realBin, realBin, lookPath)
	if got != stable {
		t.Fatalf("PreferStable = %q, want brew bin symlink %q despite dangling target", got, stable)
	}
}

func TestPreferStable_derivesBrewBinWhenSymlinkMissing(t *testing.T) {
	dir := t.TempDir()
	prefix := filepath.Join(dir, "opt", "homebrew")
	caskroom := filepath.Join(prefix, "Caskroom", "genv", "4.0.4")
	if err := os.MkdirAll(caskroom, 0o755); err != nil {
		t.Fatal(err)
	}
	realBin := filepath.Join(caskroom, "genv")
	if err := os.WriteFile(realBin, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(prefix, "bin", "genv")
	lookPath := func(string) (string, error) { return "", os.ErrNotExist }

	got := PreferStable(realBin, realBin, lookPath)
	if got != want {
		t.Fatalf("PreferStable = %q, want derived %q", got, want)
	}
}

func TestBrewStableBin_caskroomAndCellar(t *testing.T) {
	tests := []struct {
		exe  string
		want string
	}{
		{exe: "/opt/homebrew/Caskroom/genv/4.0.4/genv", want: "/opt/homebrew/bin/genv"},
		{exe: "/usr/local/Caskroom/genv/4.0.4/genv", want: "/usr/local/bin/genv"},
		{exe: "/home/linuxbrew/.linuxbrew/Caskroom/genv/1.0.0/genv", want: "/home/linuxbrew/.linuxbrew/bin/genv"},
		{exe: "/opt/homebrew/Cellar/genv/4.0.4/bin/genv", want: "/opt/homebrew/bin/genv"},
		{exe: `C:\homebrew\Caskroom\genv\4.0.4\genv`, want: filepath.FromSlash("C:/homebrew/bin/genv")},
		{exe: "/tmp/build/genv", want: ""},
		{exe: "/opt/homebrew/bin/genv", want: ""},
	}
	for _, tt := range tests {
		if got := brewStableBin(tt.exe); got != tt.want {
			t.Errorf("brewStableBin(%q) = %q, want %q", tt.exe, got, tt.want)
		}
	}
}
