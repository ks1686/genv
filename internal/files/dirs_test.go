package files

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/ks1686/genv/internal/schema"
)

func TestApply_createsMissingDirectory(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	target := filepath.Join(home, ".config", "foo")
	cfg := &schema.FilesConfig{
		Dirs: []schema.FileDir{{Target: "~/.config/foo"}},
	}

	res, err := Apply(context.Background(), cfg, "any", ApplyOptions{})
	if err != nil {
		t.Fatalf("Apply error = %v, want nil", err)
	}
	if len(res.Created) != 1 || res.Created[0] != target {
		t.Fatalf("Created = %v, want [%s]", res.Created, target)
	}
	if fi, err := os.Stat(target); err != nil || !fi.IsDir() {
		t.Fatalf("target %s should be a directory, err=%v", target, err)
	}
}

func TestApply_skipsExistingDirectory(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	target := filepath.Join(home, ".config", "foo")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	cfg := &schema.FilesConfig{
		Dirs: []schema.FileDir{{Target: "~/.config/foo"}},
	}
	res, err := Apply(context.Background(), cfg, "any", ApplyOptions{})
	if err != nil {
		t.Fatalf("Apply error = %v, want nil", err)
	}
	if len(res.Skipped) != 1 || res.Skipped[0] != target {
		t.Fatalf("Skipped = %v, want [%s]", res.Skipped, target)
	}
	if len(res.Created) != 0 || len(res.Updated) != 0 {
		t.Fatalf("unexpected created/updated: %+v", res)
	}
}

func TestApply_dirMismatchReplacesFileWithForceAndBackup(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	target := filepath.Join(home, ".config", "foo")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("mkdir parent: %v", err)
	}
	original := []byte("i am a file")
	if err := os.WriteFile(target, original, 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	cfg := &schema.FilesConfig{
		Dirs: []schema.FileDir{{Target: "~/.config/foo"}},
	}
	res, err := Apply(context.Background(), cfg, "any", ApplyOptions{Force: true, Backup: true})
	if err != nil {
		t.Fatalf("Apply error = %v, want nil", err)
	}
	if len(res.Updated) != 1 || res.Updated[0] != target {
		t.Fatalf("Updated = %v, want [%s]", res.Updated, target)
	}
	if fi, err := os.Stat(target); err != nil || !fi.IsDir() {
		t.Fatalf("target should be a directory, err=%v", err)
	}
	matches, _ := filepath.Glob(target + ".backup.*")
	if len(matches) != 1 {
		t.Fatalf("expected one backup, got %v", matches)
	}
}
