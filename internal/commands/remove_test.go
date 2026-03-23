package commands

import (
	"errors"
	"testing"

	"github.com/ks1686/genv/internal/schema"
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

// TestRemove_FromFront verifies that removing the first element preserves all
// subsequent elements in order.
func TestRemove_FromFront(t *testing.T) {
	f := newFile(
		schema.Package{ID: "a"},
		schema.Package{ID: "b"},
		schema.Package{ID: "c"},
	)
	if err := Remove(f, "a"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"b", "c"}
	if len(f.Packages) != len(want) {
		t.Fatalf("expected %d packages, got %d", len(want), len(f.Packages))
	}
	for i, p := range f.Packages {
		if p.ID != want[i] {
			t.Errorf("Packages[%d].ID = %q, want %q", i, p.ID, want[i])
		}
	}
}

// TestRemove_EmptyList verifies that Remove returns ErrNotTracked when the
// package list is empty.
func TestRemove_EmptyList(t *testing.T) {
	f := newFile() // no packages
	err := Remove(f, "git")
	if err == nil {
		t.Fatal("expected ErrNotTracked for empty list")
	}
	if !errors.Is(err, ErrNotTracked) {
		t.Errorf("expected ErrNotTracked, got: %v", err)
	}
}

// TestRemove_LeavesOtherFieldsIntact verifies that removing a package does not
// alter the remaining packages' fields.
func TestRemove_LeavesOtherFieldsIntact(t *testing.T) {
	f := newFile(
		schema.Package{ID: "keep", Version: "1.0", Prefer: "brew", Managers: map[string]string{"brew": "keep-pkg"}},
		schema.Package{ID: "remove-me"},
	)
	if err := Remove(f, "remove-me"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(f.Packages) != 1 {
		t.Fatalf("expected 1 package, got %d", len(f.Packages))
	}
	p := f.Packages[0]
	if p.ID != "keep" || p.Version != "1.0" || p.Prefer != "brew" || p.Managers["brew"] != "keep-pkg" {
		t.Errorf("remaining package fields changed: %+v", p)
	}
}
