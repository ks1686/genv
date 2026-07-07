package files

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/ks1686/genv/internal/schema"
)

func setupMergeDirSource(t *testing.T, home, name string, files map[string]string) string {
	t.Helper()
	dir := filepath.Join(home, "repo", name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir source dir: %v", err)
	}
	for rel, content := range files {
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", filepath.Dir(p), err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", p, err)
		}
	}
	return dir
}

// TestMergeDir_CreatesPerFileSymlinks verifies that a merge-dir record
// creates one symlink per file under target, not a single directory symlink.
func TestMergeDir_CreatesPerFileSymlinks(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	base := setupMergeDirSource(t, home, "zsh-common", map[string]string{
		"aliases.zsh": "alias ll='ls -la'",
		"path.zsh":    "export PATH=$PATH",
	})
	cfg := &schema.FilesConfig{
		Links: []schema.FileLink{{Source: base, Target: "~/.config/zsh", Mode: "merge-dir"}},
	}

	res, err := Apply(context.Background(), cfg, "any", ApplyOptions{})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if len(res.Created) != 2 {
		t.Fatalf("expected 2 created files, got %v", res.Created)
	}

	targetDir := filepath.Join(home, ".config", "zsh")
	for _, name := range []string{"aliases.zsh", "path.zsh"} {
		link := filepath.Join(targetDir, name)
		fi, err := os.Lstat(link)
		if err != nil {
			t.Fatalf("Lstat %s: %v", link, err)
		}
		if fi.Mode()&os.ModeSymlink == 0 {
			t.Errorf("%s: expected a symlink, got a real file", link)
		}
		got, err := os.Readlink(link)
		if err != nil {
			t.Fatalf("Readlink %s: %v", link, err)
		}
		want := filepath.Join(base, name)
		if got != want {
			t.Errorf("%s: symlink target = %q, want %q", link, got, want)
		}
	}
}

// TestMergeDir_LayersHostSpecificOverBase verifies that two merge-dir records
// targeting the same directory layer correctly: the base (unfiltered) record
// applies everywhere, and a later, host-filtered record's same-named file
// wins without requiring --force, while its host-unique file is added
// alongside the base's files.
func TestMergeDir_LayersHostSpecificOverBase(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	base := setupMergeDirSource(t, home, "zsh-common", map[string]string{
		"aliases.zsh": "common aliases",
		"path.zsh":    "common path",
	})
	archOverlay := setupMergeDirSource(t, home, "zsh-arch", map[string]string{
		"aliases.zsh":   "arch-specific aliases",
		"arch-only.zsh": "arch only",
	})
	cfg := &schema.FilesConfig{
		Links: []schema.FileLink{
			{Source: base, Target: "~/.config/zsh", Mode: "merge-dir"},
			{Source: archOverlay, Target: "~/.config/zsh", Mode: "merge-dir", Host: schema.HostPredicate{"arch"}},
		},
	}

	// On macOS, only the base layer applies.
	res, err := Apply(context.Background(), cfg, "macos", ApplyOptions{})
	if err != nil {
		t.Fatalf("Apply (macos): %v", err)
	}
	if len(res.Created) != 2 {
		t.Fatalf("macos: expected 2 created files (base only), got %v", res.Created)
	}
	targetDir := filepath.Join(home, ".config", "zsh")
	aliasesLink := filepath.Join(targetDir, "aliases.zsh")
	got, err := os.Readlink(aliasesLink)
	if err != nil {
		t.Fatalf("Readlink: %v", err)
	}
	if want := filepath.Join(base, "aliases.zsh"); got != want {
		t.Errorf("macos aliases.zsh: got %q, want %q (base)", got, want)
	}

	// Reset and apply on arch: base creates aliases.zsh+path.zsh, then the
	// arch layer must relink aliases.zsh to itself (no --force needed) and
	// add arch-only.zsh, without touching path.zsh.
	home2 := t.TempDir()
	t.Setenv("HOME", home2)
	base2 := setupMergeDirSource(t, home2, "zsh-common", map[string]string{
		"aliases.zsh": "common aliases",
		"path.zsh":    "common path",
	})
	archOverlay2 := setupMergeDirSource(t, home2, "zsh-arch", map[string]string{
		"aliases.zsh":   "arch-specific aliases",
		"arch-only.zsh": "arch only",
	})
	cfg2 := &schema.FilesConfig{
		Links: []schema.FileLink{
			{Source: base2, Target: "~/.config/zsh", Mode: "merge-dir"},
			{Source: archOverlay2, Target: "~/.config/zsh", Mode: "merge-dir", Host: schema.HostPredicate{"arch"}},
		},
	}
	res2, err := Apply(context.Background(), cfg2, "arch", ApplyOptions{})
	if err != nil {
		t.Fatalf("Apply (arch): %v", err)
	}
	if len(res2.Errors) > 0 {
		t.Fatalf("Apply (arch): unexpected errors: %v", res2.Errors)
	}

	targetDir2 := filepath.Join(home2, ".config", "zsh")
	aliasesLink2 := filepath.Join(targetDir2, "aliases.zsh")
	got2, err := os.Readlink(aliasesLink2)
	if err != nil {
		t.Fatalf("Readlink: %v", err)
	}
	if want := filepath.Join(archOverlay2, "aliases.zsh"); got2 != want {
		t.Errorf("arch aliases.zsh: got %q, want %q (arch overlay should win)", got2, want)
	}

	pathLink2 := filepath.Join(targetDir2, "path.zsh")
	gotPath, err := os.Readlink(pathLink2)
	if err != nil {
		t.Fatalf("Readlink path.zsh: %v", err)
	}
	if want := filepath.Join(base2, "path.zsh"); gotPath != want {
		t.Errorf("arch path.zsh: got %q, want %q (base, untouched by overlay)", gotPath, want)
	}

	archOnlyLink := filepath.Join(targetDir2, "arch-only.zsh")
	if _, err := os.Lstat(archOnlyLink); err != nil {
		t.Errorf("arch-only.zsh: expected to exist, Lstat error: %v", err)
	}
}

// TestMergeDir_RealFileRequiresForce verifies that a hand-authored real file
// (not a symlink) at a merge-dir target path is left untouched without
// --force, and reported as mismatched.
func TestMergeDir_RealFileRequiresForce(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	base := setupMergeDirSource(t, home, "zsh-common", map[string]string{
		"aliases.zsh": "common aliases",
	})
	targetDir := filepath.Join(home, ".config", "zsh")
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		t.Fatalf("mkdir target: %v", err)
	}
	handAuthored := filepath.Join(targetDir, "aliases.zsh")
	if err := os.WriteFile(handAuthored, []byte("hand written"), 0o644); err != nil {
		t.Fatalf("write hand-authored file: %v", err)
	}

	cfg := &schema.FilesConfig{
		Links: []schema.FileLink{{Source: base, Target: "~/.config/zsh", Mode: "merge-dir"}},
	}

	res, err := Apply(context.Background(), cfg, "any", ApplyOptions{})
	if err == nil {
		t.Fatal("Apply: expected an error (mismatch without --force), got nil")
	}
	if len(res.Mismatched) != 1 || res.Mismatched[0] != handAuthored {
		t.Errorf("expected %s in Mismatched, got %v", handAuthored, res.Mismatched)
	}
	content, err := os.ReadFile(handAuthored)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(content) != "hand written" {
		t.Error("hand-authored file was modified without --force")
	}
}

// TestMergeDir_DryRunWritesNothing verifies dry-run reports planned creates
// without touching the filesystem.
func TestMergeDir_DryRunWritesNothing(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	base := setupMergeDirSource(t, home, "zsh-common", map[string]string{
		"aliases.zsh": "common aliases",
	})
	cfg := &schema.FilesConfig{
		Links: []schema.FileLink{{Source: base, Target: "~/.config/zsh", Mode: "merge-dir"}},
	}

	res, err := Apply(context.Background(), cfg, "any", ApplyOptions{DryRun: true})
	if err != nil {
		t.Fatalf("Apply --dry-run: %v", err)
	}
	if len(res.Created) != 1 {
		t.Fatalf("expected 1 planned create, got %v", res.Created)
	}
	targetDir := filepath.Join(home, ".config", "zsh")
	if _, err := os.Stat(targetDir); !os.IsNotExist(err) {
		t.Errorf("dry-run must not create the target directory, got err=%v", err)
	}
}

// TestMergeDir_StatusReportsPerFileEntries verifies that genv status --files
// reports one entry per merged file, correctly distinguishing ok/missing/
// mismatch on a per-file basis rather than treating the whole directory as
// one unit.
func TestMergeDir_StatusReportsPerFileEntries(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	base := setupMergeDirSource(t, home, "zsh-common", map[string]string{
		"aliases.zsh": "common aliases",
		"path.zsh":    "common path",
	})
	cfg := &schema.FilesConfig{
		Links: []schema.FileLink{{Source: base, Target: "~/.config/zsh", Mode: "merge-dir"}},
	}

	// Before apply: both files should report missing.
	res, err := Status(cfg, "any")
	if err != nil {
		t.Fatalf("Status (pre-apply): %v", err)
	}
	if len(res.Entries) != 2 {
		t.Fatalf("expected 2 status entries, got %v", res.Entries)
	}
	for _, e := range res.Entries {
		if e.Kind != "missing" {
			t.Errorf("pre-apply entry %+v: kind = %q, want missing", e, e.Kind)
		}
	}
	if res.OK {
		t.Error("Status.OK = true before apply, want false")
	}

	// Apply, then status should be fully OK.
	if _, err := Apply(context.Background(), cfg, "any", ApplyOptions{}); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	res2, err := Status(cfg, "any")
	if err != nil {
		t.Fatalf("Status (post-apply): %v", err)
	}
	if !res2.OK {
		t.Errorf("Status.OK = false after apply, entries: %+v", res2.Entries)
	}

	// Break one file's symlink target manually; status should report exactly
	// that one file as mismatched, leaving the other ok.
	targetDir := filepath.Join(home, ".config", "zsh")
	if err := os.Remove(filepath.Join(targetDir, "path.zsh")); err != nil {
		t.Fatalf("remove path.zsh symlink: %v", err)
	}
	if err := os.Symlink(filepath.Join(base, "aliases.zsh"), filepath.Join(targetDir, "path.zsh")); err != nil {
		t.Fatalf("plant wrong symlink: %v", err)
	}
	res3, err := Status(cfg, "any")
	if err != nil {
		t.Fatalf("Status (drifted): %v", err)
	}
	if res3.OK {
		t.Error("Status.OK = true with a drifted file, want false")
	}
	var sawMismatch, sawOK bool
	for _, e := range res3.Entries {
		if filepath.Base(e.Target) == "path.zsh" && e.Kind == "mismatch" {
			sawMismatch = true
		}
		if filepath.Base(e.Target) == "aliases.zsh" && e.Kind == "ok" {
			sawOK = true
		}
	}
	if !sawMismatch {
		t.Errorf("expected path.zsh to report mismatch, entries: %+v", res3.Entries)
	}
	if !sawOK {
		t.Errorf("expected aliases.zsh to still report ok, entries: %+v", res3.Entries)
	}
}
