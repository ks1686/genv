package files

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/ks1686/genv/internal/schema"
)

func setupSource(t *testing.T, home, name string) string {
	t.Helper()
	source := filepath.Join(home, "repo", name)
	if err := os.MkdirAll(filepath.Dir(source), 0o755); err != nil {
		t.Fatalf("mkdir source parent: %v", err)
	}
	if err := os.WriteFile(source, []byte("hello"), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}
	return source
}

func TestApply_createsMissingLink(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	source := setupSource(t, home, "simple.txt")
	target := filepath.Join(home, ".genv-test", "simple.txt")
	cfg := &schema.FilesConfig{
		Links: []schema.FileLink{{
			Source: source,
			Target: "~/.genv-test/simple.txt",
			Mode:   "link",
		}},
	}

	res, err := Apply(context.Background(), cfg, "any", ApplyOptions{})
	if err != nil {
		t.Fatalf("Apply error = %v, want nil", err)
	}
	if len(res.Created) != 1 || res.Created[0] != target {
		t.Fatalf("Created = %v, want [%s]", res.Created, target)
	}
	got, err := os.Readlink(target)
	if err != nil {
		t.Fatalf("Readlink: %v", err)
	}
	if got != source {
		t.Fatalf("symlink points to %q, want %q", got, source)
	}
}

func TestApply_skipsCorrectLink(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	source := setupSource(t, home, "simple.txt")
	target := filepath.Join(home, ".genv-test", "simple.txt")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("mkdir target parent: %v", err)
	}
	if err := os.Symlink(source, target); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	cfg := &schema.FilesConfig{
		Links: []schema.FileLink{{
			Source: source,
			Target: "~/.genv-test/simple.txt",
			Mode:   "link",
		}},
	}
	res, err := Apply(context.Background(), cfg, "any", ApplyOptions{})
	if err != nil {
		t.Fatalf("Apply error = %v, want nil", err)
	}
	if len(res.Skipped) != 1 || res.Skipped[0] != target {
		t.Fatalf("Skipped = %v, want [%s]", res.Skipped, target)
	}
}

func TestApply_linkMismatchNoForceLeavesFileUntouched(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	source := setupSource(t, home, "simple.txt")
	target := filepath.Join(home, ".genv-test", "simple.txt")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("mkdir target parent: %v", err)
	}
	original := []byte("i am a hand-written file\n")
	if err := os.WriteFile(target, original, 0o644); err != nil {
		t.Fatalf("write planted file: %v", err)
	}

	cfg := &schema.FilesConfig{
		Links: []schema.FileLink{{
			Source: source,
			Target: "~/.genv-test/simple.txt",
			Mode:   "link",
		}},
	}
	res, err := Apply(context.Background(), cfg, "any", ApplyOptions{})
	if err == nil {
		t.Fatal("Apply error = nil, want mismatch error")
	}
	if len(res.Mismatched) != 1 || res.Mismatched[0] != target {
		t.Fatalf("Mismatched = %v, want [%s]", res.Mismatched, target)
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read target: %v", err)
	}
	if !bytes.Equal(got, original) {
		t.Fatalf("target was modified: got %q, want %q", got, original)
	}
	if fi, err := os.Lstat(target); err != nil || fi.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("target should remain a regular file")
	}
}

func TestApply_linkMismatchForceBackup(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	source := setupSource(t, home, "simple.txt")
	target := filepath.Join(home, ".genv-test", "simple.txt")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("mkdir target parent: %v", err)
	}
	original := []byte("i am a hand-written file\n")
	if err := os.WriteFile(target, original, 0o644); err != nil {
		t.Fatalf("write planted file: %v", err)
	}

	cfg := &schema.FilesConfig{
		Links: []schema.FileLink{{
			Source: source,
			Target: "~/.genv-test/simple.txt",
			Mode:   "link",
			Backup: true,
		}},
	}
	res, err := Apply(context.Background(), cfg, "any", ApplyOptions{Force: true})
	if err != nil {
		t.Fatalf("Apply error = %v, want nil", err)
	}
	if len(res.Updated) != 1 || res.Updated[0] != target {
		t.Fatalf("Updated = %v, want [%s]", res.Updated, target)
	}
	got, err := os.Readlink(target)
	if err != nil {
		t.Fatalf("Readlink: %v", err)
	}
	if got != source {
		t.Fatalf("symlink points to %q, want %q", got, source)
	}
	matches, _ := filepath.Glob(target + ".backup.*")
	if len(matches) != 1 {
		t.Fatalf("expected one backup, got %v", matches)
	}
	backupData, err := os.ReadFile(matches[0])
	if err != nil {
		t.Fatalf("read backup: %v", err)
	}
	if !bytes.Equal(backupData, original) {
		t.Fatalf("backup data mismatch: got %q, want %q", backupData, original)
	}
}

func TestApply_managedLinkSelfHealsWrongSymlink(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	source := setupSource(t, home, "simple.txt")
	wrongSource := setupSource(t, home, "other.txt")

	target := filepath.Join(home, ".genv-test", "simple.txt")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("mkdir target parent: %v", err)
	}
	if err := os.Symlink(wrongSource, target); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	cfg := &schema.FilesConfig{
		Links: []schema.FileLink{{
			Source: source,
			Target: "~/.genv-test/simple.txt",
			Mode:   "managed-link",
		}},
	}
	res, err := Apply(context.Background(), cfg, "any", ApplyOptions{})
	if err != nil {
		t.Fatalf("Apply error = %v, want nil", err)
	}
	if len(res.Updated) != 1 || res.Updated[0] != target {
		t.Fatalf("Updated = %v, want [%s]", res.Updated, target)
	}
	got, err := os.Readlink(target)
	if err != nil {
		t.Fatalf("Readlink: %v", err)
	}
	if got != source {
		t.Fatalf("symlink points to %q, want %q", got, source)
	}
}

func TestApply_managedLinkRequiresForceForRealFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	source := setupSource(t, home, "simple.txt")
	target := filepath.Join(home, ".genv-test", "simple.txt")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("mkdir target parent: %v", err)
	}
	if err := os.WriteFile(target, []byte("i am a file"), 0o644); err != nil {
		t.Fatalf("write planted file: %v", err)
	}

	cfg := &schema.FilesConfig{
		Links: []schema.FileLink{{
			Source: source,
			Target: "~/.genv-test/simple.txt",
			Mode:   "managed-link",
		}},
	}
	res, err := Apply(context.Background(), cfg, "any", ApplyOptions{})
	if err == nil {
		t.Fatal("Apply error = nil, want mismatch error")
	}
	if len(res.Mismatched) != 1 || res.Mismatched[0] != target {
		t.Fatalf("Mismatched = %v, want [%s]", res.Mismatched, target)
	}
}
