package commands

import (
	"errors"
	"testing"

	"github.com/ks1686/gpm/internal/schema"
)

func newFile(pkgs ...schema.Package) *schema.GpmFile {
	return &schema.GpmFile{
		SchemaVersion: schema.SchemaVersion,
		Packages:      pkgs,
	}
}

func TestAdd_Basic(t *testing.T) {
	f := newFile()
	if err := Add(f, "git", "*", "", nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(f.Packages) != 1 {
		t.Fatalf("expected 1 package, got %d", len(f.Packages))
	}
	p := f.Packages[0]
	if p.ID != "git" || p.Version != "*" {
		t.Errorf("unexpected package: %+v", p)
	}
}

func TestAdd_WithPreferAndManagers(t *testing.T) {
	f := newFile()
	managers := map[string]string{"flatpak": "org.mozilla.firefox", "brew": "firefox"}
	if err := Add(f, "firefox", "*", "flatpak", managers); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	p := f.Packages[0]
	if p.Prefer != "flatpak" {
		t.Errorf("Prefer = %q, want flatpak", p.Prefer)
	}
	if p.Managers["flatpak"] != "org.mozilla.firefox" {
		t.Errorf("managers[flatpak] = %q", p.Managers["flatpak"])
	}
}

func TestAdd_EmptyID(t *testing.T) {
	f := newFile()
	if err := Add(f, "", "*", "", nil); err == nil {
		t.Fatal("expected error for empty id")
	}
}

func TestAdd_Duplicate(t *testing.T) {
	f := newFile(schema.Package{ID: "git"})
	err := Add(f, "git", "*", "", nil)
	if err == nil {
		t.Fatal("expected ErrAlreadyTracked")
	}
	if !errors.Is(err, ErrAlreadyTracked) {
		t.Errorf("expected ErrAlreadyTracked, got: %v", err)
	}
}

func TestAdd_UnknownPrefer(t *testing.T) {
	f := newFile()
	if err := Add(f, "git", "*", "yum", nil); err == nil {
		t.Fatal("expected error for unknown prefer")
	}
}

func TestAdd_UnknownManagerKey(t *testing.T) {
	f := newFile()
	if err := Add(f, "git", "*", "", map[string]string{"yum": "git"}); err == nil {
		t.Fatal("expected error for unknown manager key")
	}
}

func TestAdd_NoVersionOmitted(t *testing.T) {
	f := newFile()
	if err := Add(f, "git", "", "", nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if f.Packages[0].Version != "" {
		t.Errorf("Version should be empty (omitted), got %q", f.Packages[0].Version)
	}
}

func TestAdd_PreservesOrder(t *testing.T) {
	f := newFile()
	for _, id := range []string{"git", "neovim", "firefox"} {
		if err := Add(f, id, "*", "", nil); err != nil {
			t.Fatalf("Add(%s): %v", id, err)
		}
	}
	want := []string{"git", "neovim", "firefox"}
	for i, p := range f.Packages {
		if p.ID != want[i] {
			t.Errorf("Packages[%d].ID = %q, want %q", i, p.ID, want[i])
		}
	}
}
