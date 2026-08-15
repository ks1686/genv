package files

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/ks1686/genv/internal/schema"
)

func TestApply_dryRunWritesNothing(t *testing.T) {
	home := t.TempDir()
	setTestHome(t, home)

	source := filepath.Join(home, "repo", "simple.txt")
	if err := os.MkdirAll(filepath.Dir(source), 0o755); err != nil {
		t.Fatalf("mkdir source parent: %v", err)
	}
	if err := os.WriteFile(source, []byte("hello"), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}

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
	res, err := Apply(context.Background(), cfg, "any", ApplyOptions{Force: true, DryRun: true})
	if err != nil {
		t.Fatalf("Apply error = %v, want nil", err)
	}
	if len(res.Updated) != 1 || res.Updated[0] != target {
		t.Fatalf("Updated = %v, want [%s]", res.Updated, target)
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read target: %v", err)
	}
	if !bytes.Equal(got, original) {
		t.Fatalf("target was modified during dry-run: got %q, want %q", got, original)
	}
	matches, _ := filepath.Glob(target + ".backup.*")
	if len(matches) != 0 {
		t.Fatalf("expected no backups during dry-run, got %v", matches)
	}
}
