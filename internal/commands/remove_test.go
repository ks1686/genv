package commands

import (
	"errors"
	"testing"

	"github.com/ks1686/gpm/internal/schema"
)

func TestRemove_Basic(t *testing.T) {
	f := newFile(
		schema.Package{ID: "git"},
		schema.Package{ID: "neovim"},
	)
	if err := Remove(f, "git"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(f.Packages) != 1 {
		t.Fatalf("expected 1 package, got %d", len(f.Packages))
	}
	if f.Packages[0].ID != "neovim" {
		t.Errorf("remaining package should be neovim, got %q", f.Packages[0].ID)
	}
}

func TestRemove_PreservesOrder(t *testing.T) {
	f := newFile(
		schema.Package{ID: "a"},
		schema.Package{ID: "b"},
		schema.Package{ID: "c"},
	)
	if err := Remove(f, "b"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"a", "c"}
	for i, p := range f.Packages {
		if p.ID != want[i] {
			t.Errorf("Packages[%d].ID = %q, want %q", i, p.ID, want[i])
		}
	}
}

func TestRemove_NotFound(t *testing.T) {
	f := newFile(schema.Package{ID: "git"})
	err := Remove(f, "neovim")
	if err == nil {
		t.Fatal("expected ErrNotTracked")
	}
	if !errors.Is(err, ErrNotTracked) {
		t.Errorf("expected ErrNotTracked, got: %v", err)
	}
}

func TestRemove_EmptyID(t *testing.T) {
	f := newFile()
	if err := Remove(f, ""); err == nil {
		t.Fatal("expected error for empty id")
	}
}

func TestRemove_LastPackage(t *testing.T) {
	f := newFile(schema.Package{ID: "git"})
	if err := Remove(f, "git"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(f.Packages) != 0 {
		t.Errorf("expected empty Packages slice, got %d items", len(f.Packages))
	}
}
