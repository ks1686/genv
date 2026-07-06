package files

import (
	"context"
	"errors"
	"os"
	"path/filepath"
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
	t.Setenv("HOME", home)

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
