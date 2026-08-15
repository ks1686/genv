package files

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ks1686/genv/internal/schema"
)

func TestApply_createsMissingTemplate(t *testing.T) {
	home := t.TempDir()
	setTestHome(t, home)

	repo := filepath.Join(home, "repo")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatalf("mkdir repo: %v", err)
	}
	source := filepath.Join(repo, "config.toml")
	if err := os.WriteFile(source, []byte("home = __HOME__\n"), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}

	target := filepath.Join(home, ".config", "codex", "config.toml")
	cfg := &schema.FilesConfig{
		Templates: []schema.FileTemplate{{
			Source: "config.toml",
			Target: "~/.config/codex/config.toml",
		}},
	}

	res, err := Apply(context.Background(), cfg, "any", ApplyOptions{SourceRoot: repo})
	if err != nil {
		t.Fatalf("Apply error = %v", err)
	}
	if len(res.Created) != 1 || res.Created[0] != target {
		t.Fatalf("Created = %v, want [%s]", res.Created, target)
	}

	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read target: %v", err)
	}
	want := "home = " + home + "\n"
	if string(got) != want {
		t.Fatalf("target = %q, want %q", got, want)
	}
}

func TestApply_skipsMatchingTemplate(t *testing.T) {
	home := t.TempDir()
	setTestHome(t, home)

	repo := filepath.Join(home, "repo")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatalf("mkdir repo: %v", err)
	}
	source := filepath.Join(repo, "config.toml")
	if err := os.WriteFile(source, []byte("home = __HOME__\n"), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}

	target := filepath.Join(home, ".config", "codex", "config.toml")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("mkdir target parent: %v", err)
	}
	want := "home = " + home + "\n"
	if err := os.WriteFile(target, []byte(want), 0o644); err != nil {
		t.Fatalf("write target: %v", err)
	}

	cfg := &schema.FilesConfig{
		Templates: []schema.FileTemplate{{
			Source: "config.toml",
			Target: "~/.config/codex/config.toml",
		}},
	}
	res, err := Apply(context.Background(), cfg, "any", ApplyOptions{SourceRoot: repo})
	if err != nil {
		t.Fatalf("Apply error = %v", err)
	}
	if len(res.Skipped) != 1 || res.Skipped[0] != target {
		t.Fatalf("Skipped = %v, want [%s]", res.Skipped, target)
	}
}

func TestApply_templateMismatchNoForceLeavesTarget(t *testing.T) {
	home := t.TempDir()
	setTestHome(t, home)

	repo := filepath.Join(home, "repo")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatalf("mkdir repo: %v", err)
	}
	source := filepath.Join(repo, "config.toml")
	if err := os.WriteFile(source, []byte("home = __HOME__\n"), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}

	target := filepath.Join(home, ".config", "codex", "config.toml")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("mkdir target parent: %v", err)
	}
	original := []byte("home = /old/home\n")
	if err := os.WriteFile(target, original, 0o644); err != nil {
		t.Fatalf("write target: %v", err)
	}

	cfg := &schema.FilesConfig{
		Templates: []schema.FileTemplate{{
			Source: "config.toml",
			Target: "~/.config/codex/config.toml",
		}},
	}
	res, err := Apply(context.Background(), cfg, "any", ApplyOptions{SourceRoot: repo})
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
}

func TestApply_templateMismatchForceBackup(t *testing.T) {
	home := t.TempDir()
	setTestHome(t, home)

	repo := filepath.Join(home, "repo")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatalf("mkdir repo: %v", err)
	}
	source := filepath.Join(repo, "config.toml")
	if err := os.WriteFile(source, []byte("home = __HOME__\n"), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}

	target := filepath.Join(home, ".config", "codex", "config.toml")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("mkdir target parent: %v", err)
	}
	original := []byte("home = /old/home\n")
	if err := os.WriteFile(target, original, 0o644); err != nil {
		t.Fatalf("write target: %v", err)
	}

	cfg := &schema.FilesConfig{
		Templates: []schema.FileTemplate{{
			Source: "config.toml",
			Target: "~/.config/codex/config.toml",
			Backup: true,
		}},
	}
	res, err := Apply(context.Background(), cfg, "any", ApplyOptions{SourceRoot: repo, Force: true})
	if err != nil {
		t.Fatalf("Apply error = %v", err)
	}
	if len(res.Updated) != 1 || res.Updated[0] != target {
		t.Fatalf("Updated = %v, want [%s]", res.Updated, target)
	}

	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read target: %v", err)
	}
	want := "home = " + home + "\n"
	if string(got) != want {
		t.Fatalf("target = %q, want %q", got, want)
	}

	matches, err := filepath.Glob(target + ".backup.*")
	if err != nil {
		t.Fatalf("glob backups: %v", err)
	}
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

func TestApply_templateDryRunWritesNothing(t *testing.T) {
	home := t.TempDir()
	setTestHome(t, home)

	repo := filepath.Join(home, "repo")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatalf("mkdir repo: %v", err)
	}
	source := filepath.Join(repo, "config.toml")
	if err := os.WriteFile(source, []byte("home = __HOME__\n"), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}

	target := filepath.Join(home, ".config", "codex", "config.toml")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("mkdir target parent: %v", err)
	}
	original := []byte("home = /old/home\n")
	if err := os.WriteFile(target, original, 0o644); err != nil {
		t.Fatalf("write target: %v", err)
	}

	cfg := &schema.FilesConfig{
		Templates: []schema.FileTemplate{{
			Source: "config.toml",
			Target: "~/.config/codex/config.toml",
			Backup: true,
		}},
	}
	res, err := Apply(context.Background(), cfg, "any", ApplyOptions{SourceRoot: repo, Force: true, DryRun: true})
	if err != nil {
		t.Fatalf("Apply error = %v", err)
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
	matches, err := filepath.Glob(target + ".backup.*")
	if err != nil {
		t.Fatalf("glob backups: %v", err)
	}
	if len(matches) != 0 {
		t.Fatalf("expected no backups during dry-run, got %v", matches)
	}
}

func TestApply_filtersTemplatesByHost(t *testing.T) {
	home := t.TempDir()
	setTestHome(t, home)

	repo := filepath.Join(home, "repo")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatalf("mkdir repo: %v", err)
	}
	macSource := filepath.Join(repo, "mac.toml")
	archSource := filepath.Join(repo, "arch.toml")
	if err := os.WriteFile(macSource, []byte("host=mac\n"), 0o644); err != nil {
		t.Fatalf("write mac source: %v", err)
	}
	if err := os.WriteFile(archSource, []byte("host=arch\n"), 0o644); err != nil {
		t.Fatalf("write arch source: %v", err)
	}

	cfg := &schema.FilesConfig{
		Templates: []schema.FileTemplate{
			{Source: "mac.toml", Target: "~/.config/mac.toml", Host: schema.HostPredicate{"macos"}},
			{Source: "arch.toml", Target: "~/.config/arch.toml", Host: schema.HostPredicate{"arch"}},
		},
	}

	res, err := Apply(context.Background(), cfg, "arch", ApplyOptions{SourceRoot: repo})
	if err != nil {
		t.Fatalf("Apply error = %v", err)
	}
	if len(res.Created) != 1 {
		t.Fatalf("Created = %v, want 1 entry", res.Created)
	}

	if _, err := os.Stat(filepath.Join(home, ".config", "arch.toml")); err != nil {
		t.Fatalf("arch template should exist: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".config", "mac.toml")); !os.IsNotExist(err) {
		t.Fatalf("mac template should not exist")
	}
}

func TestApply_templateS5DriftScenario(t *testing.T) {
	home := t.TempDir()
	setTestHome(t, home)

	repo := filepath.Join(home, "repo")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatalf("mkdir repo: %v", err)
	}
	source := filepath.Join(repo, "codex-config.toml")
	srcData := "[core]\nhome = \"__HOME__\"\nuser = \"__USER__\"\n"
	if err := os.WriteFile(source, []byte(srcData), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}

	target := filepath.Join(home, ".config", "codex", "config.toml")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("mkdir target parent: %v", err)
	}
	oldRendered := []byte("[core]\nhome = \"/old/home\"\nuser = \"olduser\"\n")
	if err := os.WriteFile(target, oldRendered, 0o644); err != nil {
		t.Fatalf("write old rendered file: %v", err)
	}

	cfg := &schema.FilesConfig{
		Templates: []schema.FileTemplate{{
			Source: "codex-config.toml",
			Target: "~/.config/codex/config.toml",
			Backup: true,
		}},
	}
	res, err := Apply(context.Background(), cfg, "any", ApplyOptions{SourceRoot: repo, Force: true})
	if err != nil {
		t.Fatalf("Apply error = %v", err)
	}
	if len(res.Updated) != 1 || res.Updated[0] != target {
		t.Fatalf("Updated = %v, want [%s]", res.Updated, target)
	}

	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read rendered file: %v", err)
	}
	if !strings.Contains(string(got), home) {
		t.Fatalf("rendered file should contain HOME %q, got: %q", home, got)
	}
	if strings.Contains(string(got), "__HOME__") {
		t.Fatalf("rendered file should not contain literal __HOME__, got: %q", got)
	}

	matches, err := filepath.Glob(target + ".backup.*")
	if err != nil {
		t.Fatalf("glob backups: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected exactly one backup file, got %v", matches)
	}
}
