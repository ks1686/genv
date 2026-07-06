package files

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/ks1686/genv/internal/schema"
)

func TestApply_sourceRelativeToSourceRoot(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	repo := filepath.Join(home, "repo")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatalf("mkdir repo: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, "simple.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}

	target := filepath.Join(home, ".genv-test", "simple.txt")
	cfg := &schema.FilesConfig{
		Links: []schema.FileLink{{
			Source: "simple.txt",
			Target: "~/.genv-test/simple.txt",
			Mode:   "link",
		}},
	}

	res, err := Apply(context.Background(), cfg, "any", ApplyOptions{SourceRoot: repo})
	if err != nil {
		t.Fatalf("Apply error = %v, want nil", err)
	}
	if len(res.Created) != 1 {
		t.Fatalf("Created = %v, want 1", res.Created)
	}
	got, err := os.Readlink(target)
	if err != nil {
		t.Fatalf("Readlink: %v", err)
	}
	want := filepath.Join(repo, "simple.txt")
	if got != want {
		t.Fatalf("symlink points to %q, want %q", got, want)
	}
}

func TestApply_expandsHomeEnvVar(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	source := filepath.Join(home, "repo", "simple.txt")
	if err := os.MkdirAll(filepath.Dir(source), 0o755); err != nil {
		t.Fatalf("mkdir source parent: %v", err)
	}
	if err := os.WriteFile(source, []byte("hello"), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}

	target := filepath.Join(home, ".genv-test", "simple.txt")
	cfg := &schema.FilesConfig{
		Links: []schema.FileLink{{
			Source: source,
			Target: "$HOME/.genv-test/simple.txt",
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
}
