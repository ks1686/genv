package genvfile

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/ks1686/genv/internal/schema"
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
	path := filepath.Join(dir, "genv.json")

	original := &schema.GenvFile{
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
		if len(p.Managers) != len(want.Managers) {
			t.Errorf("Packages[%d].Managers: got %v, want %v", i, p.Managers, want.Managers)
		}
		for k, wantV := range want.Managers {
			if p.Managers[k] != wantV {
				t.Errorf("Packages[%d].Managers[%q]: got %q, want %q", i, k, p.Managers[k], wantV)
			}
		}
	}
}

func TestWrite_IsAtomic(t *testing.T) {
	// After a successful Write, there should be no leftover .tmp file.
	dir := t.TempDir()
	path := filepath.Join(dir, "genv.json")

	if err := Write(path, New()); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if _, err := os.Stat(path + ".tmp"); !errors.Is(err, os.ErrNotExist) {
		t.Error("expected .tmp file to be cleaned up after Write")
	}
}

func TestRead_NotFound(t *testing.T) {
	_, err := Read("/nonexistent/path/genv.json")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got: %v", err)
	}
}

func TestReadOrNew_CreatesNew(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "genv.json")

	f, isNew, err := ReadOrNew(path)
	if err != nil {
		t.Fatalf("ReadOrNew: %v", err)
	}
	if !isNew {
		t.Error("isNew should be true for a missing file")
	}
	if f == nil {
		t.Fatal("expected non-nil GenvFile")
	}
	if f.SchemaVersion != schema.SchemaVersion {
		t.Errorf("SchemaVersion = %q", f.SchemaVersion)
	}
}

func TestReadOrNew_ReadsExisting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "genv.json")

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
		t.Fatal("expected non-nil GenvFile")
	}
}

func TestRead_ValidationError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "genv.json")

	bad := []byte(`{"schemaVersion":"2","packages":[]}`)
	if err := os.WriteFile(path, bad, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, err := Read(path)
	if err == nil {
		t.Fatal("expected error for invalid schemaVersion")
	}
	if !errors.Is(err, ErrInvalidFile) {
		t.Errorf("expected ErrInvalidFile, got: %v", err)
	}
}

func TestRead_SyntaxError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "genv.json")

	bad := []byte(`{"schemaVersion": "1", "packages": [`)
	if err := os.WriteFile(path, bad, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, err := Read(path)
	if err == nil {
		t.Fatal("expected error for malformed JSON")
	}
	if !errors.Is(err, ErrInvalidFile) {
		t.Errorf("expected ErrInvalidFile, got: %v", err)
	}
}

func TestRead_PermissionError(t *testing.T) {
	// Write a valid file then remove all permissions so os.ReadFile returns a
	// permission-denied error, which is neither ErrNotFound nor ErrInvalidFile.
	dir := t.TempDir()
	path := filepath.Join(dir, "genv.json")

	if err := os.WriteFile(path, []byte(`{"schemaVersion":"1","packages":[]}`), 0o200); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	t.Cleanup(func() { os.Chmod(path, 0o644) })

	_, err := Read(path)
	if err == nil {
		t.Fatal("expected error for unreadable file")
	}
	if errors.Is(err, ErrNotFound) {
		t.Error("expected a non-ErrNotFound error for permission-denied read")
	}
	if errors.Is(err, ErrInvalidFile) {
		t.Error("expected a non-ErrInvalidFile error for permission-denied read")
	}
}

func TestWrite_CreatesParentDirs(t *testing.T) {
	// Write must create any missing parent directories (e.g. ~/.config/genv/)
	// so that first-run behaviour is self-bootstrapping.
	path := filepath.Join(t.TempDir(), "nonexistent", "subdir", "genv.json")
	if err := Write(path, New()); err != nil {
		t.Fatalf("expected Write to create parent dirs, got error: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected genv.json to exist after Write: %v", err)
	}
}

// TestReadOrNew_InvalidFile verifies that ReadOrNew propagates ErrInvalidFile
// when the existing file fails schema validation (it must NOT return isNew=true).
func TestReadOrNew_InvalidFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "genv.json")

	if err := os.WriteFile(path, []byte(`{"schemaVersion":"99","packages":[]}`), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, isNew, err := ReadOrNew(path)
	if err == nil {
		t.Fatal("expected error for invalid file, got nil")
	}
	if isNew {
		t.Error("isNew should be false for an invalid existing file")
	}
	if !errors.Is(err, ErrInvalidFile) {
		t.Errorf("expected ErrInvalidFile, got: %v", err)
	}
}

// TestWrite_OverwritesExistingFile verifies that calling Write on an existing
// file replaces its content correctly.
func TestWrite_OverwritesExistingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "genv.json")

	first := New()
	if err := Write(path, first); err != nil {
		t.Fatalf("first Write: %v", err)
	}

	second := &schema.GenvFile{
		SchemaVersion: schema.SchemaVersion,
		Packages: []schema.Package{
			{ID: "git", Version: "1.0"},
		},
	}
	if err := Write(path, second); err != nil {
		t.Fatalf("second Write: %v", err)
	}

	got, err := Read(path)
	if err != nil {
		t.Fatalf("Read after overwrite: %v", err)
	}
	if len(got.Packages) != 1 || got.Packages[0].ID != "git" {
		t.Errorf("expected 1 package 'git' after overwrite, got: %+v", got.Packages)
	}
}

// TestNew_PackagesNonNil verifies that New() initialises Packages as a non-nil
// empty slice so that it marshals as "[]" rather than "null".
func TestNew_PackagesNonNil(t *testing.T) {
	f := New()
	if f.Packages == nil {
		t.Error("New().Packages must be non-nil to serialise as []")
	}
	if len(f.Packages) != 0 {
		t.Errorf("New().Packages should be empty, got %d entries", len(f.Packages))
	}
}

// TestWrite_ProducesValidJSON verifies that the output of Write is valid JSON
// that can be re-read by Read.
func TestWrite_ProducesValidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "genv.json")

	f := &schema.GenvFile{
		SchemaVersion: schema.SchemaVersion,
		Packages: []schema.Package{
			{
				ID:      "firefox",
				Version: "1.0",
				Prefer:  "flatpak",
				Managers: map[string]string{
					"flatpak": "org.mozilla.firefox",
					"brew":    "firefox",
				},
			},
		},
	}

	if err := Write(path, f); err != nil {
		t.Fatalf("Write: %v", err)
	}

	got, err := Read(path)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if got.Packages[0].Managers["flatpak"] != "org.mozilla.firefox" {
		t.Errorf("managers roundtrip: got %v", got.Packages[0].Managers)
	}
}
