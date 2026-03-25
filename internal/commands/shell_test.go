package commands

import (
	"errors"
	"testing"

	"github.com/ks1686/genv/internal/schema"
)

// ─── ShellAliasSet ───────────────────────────────────────────────────────────

func TestShellAliasSet_New(t *testing.T) {
	f := &schema.GenvFile{SchemaVersion: schema.Version, Packages: []schema.Package{}}
	if err := ShellAliasSet(f, "ll", "ls -la", ""); err != nil {
		t.Fatalf("ShellAliasSet: %v", err)
	}
	if f.SchemaVersion != schema.Version3 {
		t.Errorf("expected schemaVersion %q, got %q", schema.Version3, f.SchemaVersion)
	}
	a, ok := f.Shell.Aliases["ll"]
	if !ok {
		t.Fatal("expected alias 'll' in shell map")
	}
	if a.Value != "ls -la" {
		t.Errorf("expected value %q, got %q", "ls -la", a.Value)
	}
	if a.Shell != "" {
		t.Errorf("expected empty shell target, got %q", a.Shell)
	}
}

func TestShellAliasSet_WithShellTarget(t *testing.T) {
	f := &schema.GenvFile{SchemaVersion: schema.Version, Packages: []schema.Package{}}
	if err := ShellAliasSet(f, "gs", "git status", "zsh"); err != nil {
		t.Fatalf("ShellAliasSet: %v", err)
	}
	a := f.Shell.Aliases["gs"]
	if a.Shell != "zsh" {
		t.Errorf("expected shell %q, got %q", "zsh", a.Shell)
	}
}

func TestShellAliasSet_Update(t *testing.T) {
	f := &schema.GenvFile{
		SchemaVersion: schema.Version3,
		Packages:      []schema.Package{},
		Shell: &schema.ShellConfig{
			Aliases: map[string]schema.ShellAlias{"ll": {Value: "ls -l"}},
		},
	}
	if err := ShellAliasSet(f, "ll", "ls -la", ""); err != nil {
		t.Fatalf("ShellAliasSet update: %v", err)
	}
	if f.Shell.Aliases["ll"].Value != "ls -la" {
		t.Errorf("expected updated value, got %q", f.Shell.Aliases["ll"].Value)
	}
}

func TestShellAliasSet_EmptyName(t *testing.T) {
	f := &schema.GenvFile{SchemaVersion: schema.Version, Packages: []schema.Package{}}
	if err := ShellAliasSet(f, "", "ls -la", ""); err == nil {
		t.Error("expected error for empty name")
	}
}

func TestShellAliasSet_InvalidShellTarget(t *testing.T) {
	f := &schema.GenvFile{SchemaVersion: schema.Version, Packages: []schema.Package{}}
	if err := ShellAliasSet(f, "ll", "ls -la", "powershell"); err == nil {
		t.Error("expected error for unknown shell target")
	}
}

func TestShellAliasSet_AllShellTargets(t *testing.T) {
	for _, shell := range []string{"bash", "zsh", "fish", ""} {
		f := &schema.GenvFile{SchemaVersion: schema.Version, Packages: []schema.Package{}}
		if err := ShellAliasSet(f, "foo", "bar", shell); err != nil {
			t.Errorf("ShellAliasSet with shell=%q: unexpected error: %v", shell, err)
		}
	}
}

// ─── ShellAliasUnset ─────────────────────────────────────────────────────────

func TestShellAliasUnset_OK(t *testing.T) {
	f := &schema.GenvFile{
		SchemaVersion: schema.Version3,
		Packages:      []schema.Package{},
		Shell: &schema.ShellConfig{
			Aliases: map[string]schema.ShellAlias{"ll": {Value: "ls -la"}},
		},
	}
	if err := ShellAliasUnset(f, "ll"); err != nil {
		t.Fatalf("ShellAliasUnset: %v", err)
	}
	if _, ok := f.Shell.Aliases["ll"]; ok {
		t.Error("expected alias 'll' to be removed")
	}
}

func TestShellAliasUnset_NotFound(t *testing.T) {
	f := &schema.GenvFile{
		SchemaVersion: schema.Version3,
		Packages:      []schema.Package{},
		Shell:         &schema.ShellConfig{},
	}
	err := ShellAliasUnset(f, "missing")
	if err == nil {
		t.Fatal("expected error for missing alias")
	}
	if !errors.Is(err, ErrShellAliasNotFound) {
		t.Errorf("expected ErrShellAliasNotFound, got %v", err)
	}
}

func TestShellAliasUnset_NoShellBlock(t *testing.T) {
	f := &schema.GenvFile{SchemaVersion: schema.Version, Packages: []schema.Package{}}
	err := ShellAliasUnset(f, "ll")
	if err == nil {
		t.Fatal("expected error when shell block is nil")
	}
	if !errors.Is(err, ErrShellAliasNotFound) {
		t.Errorf("expected ErrShellAliasNotFound, got %v", err)
	}
}
