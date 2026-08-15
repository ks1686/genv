package files

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ks1686/genv/internal/schema"
)

func TestApply_nilConfig(t *testing.T) {
	res, err := Apply(context.Background(), nil, "any", ApplyOptions{})
	if err != nil {
		t.Fatalf("Apply(nil) error = %v, want nil", err)
	}
	if len(res.Created) != 0 || len(res.Updated) != 0 || len(res.Skipped) != 0 || len(res.Mismatched) != 0 || len(res.Errors) != 0 {
		t.Fatalf("Apply(nil) result = %+v, want empty", res)
	}
}

func TestApply_contextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	cfg := &schema.FilesConfig{
		Dirs: []schema.FileDir{{Target: "~/.config/foo"}},
	}
	_, err := Apply(ctx, cfg, "any", ApplyOptions{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
}

func TestApply_filtersLinksAndDirsByHost(t *testing.T) {
	home := t.TempDir()
	setTestHome(t, home)

	source := filepath.Join(home, "repo", "simple.txt")
	if err := os.MkdirAll(filepath.Dir(source), 0o755); err != nil {
		t.Fatalf("mkdir source parent: %v", err)
	}
	if err := os.WriteFile(source, []byte("hello"), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}

	cfg := &schema.FilesConfig{
		Dirs: []schema.FileDir{
			{Target: "~/.config/mac", Host: schema.HostPredicate{"macos"}},
			{Target: "~/.config/arch", Host: schema.HostPredicate{"arch"}},
		},
		Links: []schema.FileLink{
			{Source: source, Target: "~/.mac-link", Mode: "link", Host: schema.HostPredicate{"macos"}},
			{Source: source, Target: "~/.arch-link", Mode: "link", Host: schema.HostPredicate{"arch"}},
		},
	}

	res, err := Apply(context.Background(), cfg, "arch", ApplyOptions{})
	if err != nil {
		t.Fatalf("Apply error = %v, want nil", err)
	}
	if len(res.Created) != 2 {
		t.Fatalf("Created = %v, want 2 entries", res.Created)
	}
	if _, err := os.Stat(filepath.Join(home, ".config", "arch")); err != nil {
		t.Fatalf("arch dir should exist: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".config", "mac")); !os.IsNotExist(err) {
		t.Fatalf("mac dir should not exist")
	}
	if _, err := os.Lstat(filepath.Join(home, ".arch-link")); err != nil {
		t.Fatalf("arch link should exist: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(home, ".mac-link")); !os.IsNotExist(err) {
		t.Fatalf("mac link should not exist")
	}
}

func TestApply_errorUnwrapsToUnderlyingFailure(t *testing.T) {
	home := t.TempDir()
	setTestHome(t, home)

	// A merge-dir whose source directory does not exist makes applyMergeDir
	// return an error wrapping os.ErrNotExist, collected into res.Errors.
	missing := filepath.Join(home, "does-not-exist")
	cfg := &schema.FilesConfig{
		Links: []schema.FileLink{{
			Source: missing,
			Target: "~/.genv-test/merged",
			Mode:   "merge-dir",
		}},
	}

	res, err := Apply(context.Background(), cfg, "any", ApplyOptions{})
	if err == nil {
		t.Fatal("Apply error = nil, want summary error")
	}
	if len(res.Errors) != 1 {
		t.Fatalf("res.Errors = %v, want exactly one underlying error", res.Errors)
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("errors.Is(err, os.ErrNotExist) = false, want true; summary error must unwrap to the underlying failure: %v", err)
	}
	if !strings.Contains(err.Error(), "error(s)") {
		t.Fatalf("summary text = %q, want it to still contain the human-readable %q count", err.Error(), "error(s)")
	}
}
