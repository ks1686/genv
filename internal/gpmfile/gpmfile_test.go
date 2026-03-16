package gpmfile

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/ks1686/gpm/internal/schema"
)

func TestNew(t *testing.T) {
	f := New()
	if f.SchemaVersion != schema.SchemaVersion {
		t.Errorf("SchemaVersion = %q, want %q", f.SchemaVersion, schema.SchemaVersion)
	}
	if f.Packages == nil {
		t.Error("Packages must be non-nil to marshal as [] not null")
	}
}

func TestWriteAndRead_Roundtrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gpm.json")

	original := &schema.GpmFile{
		SchemaVersion: schema.SchemaVersion,
		Packages: []schema.Package{
			{ID: "git", Version: "*"},
			{ID: "neovim", Version: "0.10.*", Prefer: "brew"},
			{
				ID: "firefox",
				Managers: map[string]string{
					"flatpak": "org.mozilla.firefox",
					"brew":    "firefox",
				},
			},
		},
	}

	if err := Write(path, original); err != nil {
		t.Fatalf("Write: %v", err)
	}

	got, err := Read(path)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}

	if got.SchemaVersion != original.SchemaVersion {
		t.Errorf("SchemaVersion: got %q, want %q", got.SchemaVersion, original.SchemaVersion)
	}
	if len(got.Packages) != len(original.Packages) {
		t.Fatalf("len(Packages): got %d, want %d", len(got.Packages), len(original.Packages))
	}
	for i, p := range got.Packages {
		want := original.Packages[i]
		if p.ID != want.ID {
			t.Errorf("Packages[%d].ID: got %q, want %q", i, p.ID, want.ID)
		}
		if p.Version != want.Version {
			t.Errorf("Packages[%d].Version: got %q, want %q", i, p.Version, want.Version)
		}
		if p.Prefer != want.Prefer {
			t.Errorf("Packages[%d].Prefer: got %q, want %q", i, p.Prefer, want.Prefer)
		}
	}
}

func TestWrite_IsAtomic(t *testing.T) {
	// After a successful Write, there should be no leftover .tmp file.
	dir := t.TempDir()
	path := filepath.Join(dir, "gpm.json")

	if err := Write(path, New()); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if _, err := os.Stat(path + ".tmp"); !errors.Is(err, os.ErrNotExist) {
		t.Error("expected .tmp file to be cleaned up after Write")
	}
}

func TestRead_NotFound(t *testing.T) {
	_, err := Read("/nonexistent/path/gpm.json")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got: %v", err)
	}
}

func TestReadOrNew_CreatesNew(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gpm.json")

	f, isNew, err := ReadOrNew(path)
	if err != nil {
		t.Fatalf("ReadOrNew: %v", err)
	}
	if !isNew {
		t.Error("isNew should be true for a missing file")
	}
	if f == nil {
		t.Fatal("expected non-nil GpmFile")
	}
	if f.SchemaVersion != schema.SchemaVersion {
		t.Errorf("SchemaVersion = %q", f.SchemaVersion)
	}
}

func TestReadOrNew_ReadsExisting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gpm.json")

	if err := Write(path, New()); err != nil {
		t.Fatalf("Write: %v", err)
	}

	f, isNew, err := ReadOrNew(path)
	if err != nil {
		t.Fatalf("ReadOrNew: %v", err)
	}
	if isNew {
		t.Error("isNew should be false for an existing file")
	}
	if f == nil {
		t.Fatal("expected non-nil GpmFile")
	}
}

func TestRead_ValidationError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gpm.json")

	bad := []byte(`{"schemaVersion":"2","packages":[]}`)
	if err := os.WriteFile(path, bad, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, err := Read(path)
	if err == nil {
		t.Fatal("expected error for invalid schemaVersion")
	}
}
