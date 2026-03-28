package genvfile

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ks1686/genv/internal/schema"
)

func TestNew(t *testing.T) {
	f := New()
	if f.SchemaVersion != schema.Version {
		t.Errorf("SchemaVersion = %q, want %q", f.SchemaVersion, schema.Version)
	}
	if f.Packages == nil {
		t.Error("Packages must be non-nil to marshal as [] not null")
	}
}

func TestWriteAndRead_Roundtrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "genv.json")

	original := &schema.GenvFile{
		SchemaVersion: schema.Version,
		Packages: []schema.Package{
			{ID: "git", Version: "*"},
			{ID: "neovim", Version: "0.10.*", Prefer: "brew"},
			{
				ID: "firefox",
				Managers: map[string]string{
					"snap": "firefox",
					"brew": "firefox",
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
	if got == nil {
		t.Fatal("Read returned nil")
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
	if f.SchemaVersion != schema.Version {
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

	// "99" is not a valid schema version (both "1" and "2" are accepted).
	bad := []byte(`{"schemaVersion":"99","packages":[]}`)
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
	t.Cleanup(func() { _ = os.Chmod(path, 0o644) })

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
	// so that first-run behavior is self-bootstrapping.
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
		SchemaVersion: schema.Version,
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
	if got == nil {
		t.Fatal("Read returned nil")
	}
	if len(got.Packages) != 1 || got.Packages[0].ID != "git" {
		t.Errorf("expected 1 package 'git' after overwrite, got: %+v", got.Packages)
	}
}

// ---------------------------------------------------------------------------
// LockPathFrom — pure function
// ---------------------------------------------------------------------------

func TestLockPathFrom(t *testing.T) {
	tests := []struct {
		specPath string
		want     string
	}{
		{"genv.json", "genv.lock.json"},
		{"/home/user/.config/genv/genv.json", "/home/user/.config/genv/genv.lock.json"},
		{"custom.json", "custom.lock.json"},
		{"/tmp/env.json", "/tmp/env.lock.json"},
	}
	for _, tc := range tests {
		got := LockPathFrom(tc.specPath)
		if got != tc.want {
			t.Errorf("LockPathFrom(%q) = %q, want %q", tc.specPath, got, tc.want)
		}
	}
}

// ---------------------------------------------------------------------------
// DefaultDir / DefaultSpecPath — XDG_CONFIG_HOME support
// ---------------------------------------------------------------------------

func TestDefaultDir_UsesXDG(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/custom/config")
	dir, err := DefaultDir()
	if err != nil {
		t.Fatalf("DefaultDir: %v", err)
	}
	if !strings.HasPrefix(dir, "/custom/config") {
		t.Errorf("DefaultDir with XDG_CONFIG_HOME: got %q, expected prefix /custom/config", dir)
	}
}

func TestDefaultDir_FallsBackToHome(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "")
	dir, err := DefaultDir()
	if err != nil {
		t.Fatalf("DefaultDir: %v", err)
	}
	if dir == "" {
		t.Error("DefaultDir: returned empty string")
	}
	if !strings.Contains(dir, "genv") {
		t.Errorf("DefaultDir: expected 'genv' in path, got %q", dir)
	}
}

func TestDefaultSpecPath_ContainsGenvJSON(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/xdg")
	path, err := DefaultSpecPath()
	if err != nil {
		t.Fatalf("DefaultSpecPath: %v", err)
	}
	if !strings.HasSuffix(path, "genv.json") {
		t.Errorf("DefaultSpecPath: expected path ending in genv.json, got %q", path)
	}
}

// ---------------------------------------------------------------------------
// ReadLock — missing file, existing file, malformed JSON
// ---------------------------------------------------------------------------

func TestReadLock_MissingFile_ReturnsEmpty(t *testing.T) {
	lf, err := ReadLock("/nonexistent/path/genv.lock.json")
	if err != nil {
		t.Fatalf("ReadLock on missing file: expected nil error, got %v", err)
	}
	if lf == nil {
		t.Fatal("ReadLock on missing file: expected non-nil LockFile")
	}
	if lf.SchemaVersion != schema.Version {
		t.Errorf("SchemaVersion: got %q, want %q", lf.SchemaVersion, schema.Version)
	}
	if len(lf.Packages) != 0 {
		t.Errorf("Packages: got %d entries, want 0", len(lf.Packages))
	}
}

func TestReadLock_ValidFile_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "genv.lock.json")

	original := &LockFile{
		SchemaVersion: schema.Version,
		Packages: []LockedPackage{
			{ID: "git", Manager: "brew", PkgName: "git", InstalledVersion: "2.43.0"},
			{ID: "neovim", Manager: "paru", PkgName: "neovim"},
		},
	}
	if err := WriteLock(path, original); err != nil {
		t.Fatalf("WriteLock: %v", err)
	}

	got, err := ReadLock(path)
	if err != nil {
		t.Fatalf("ReadLock: %v", err)
	}
	if len(got.Packages) != 2 {
		t.Fatalf("len(Packages): got %d, want 2", len(got.Packages))
	}
	if got.Packages[0].ID != "git" {
		t.Errorf("Packages[0].ID: got %q, want \"git\"", got.Packages[0].ID)
	}
	if got.Packages[0].InstalledVersion != "2.43.0" {
		t.Errorf("InstalledVersion: got %q, want \"2.43.0\"", got.Packages[0].InstalledVersion)
	}
	if got.Packages[1].InstalledVersion != "" {
		t.Errorf("InstalledVersion omitempty: got %q, want empty", got.Packages[1].InstalledVersion)
	}
}

func TestReadLock_MalformedJSON_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "genv.lock.json")
	if err := os.WriteFile(path, []byte(`{broken json`), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	_, err := ReadLock(path)
	if err == nil {
		t.Fatal("ReadLock on malformed JSON: expected error, got nil")
	}
}

func TestReadLock_PermissionError(t *testing.T) {
	dir := t.TempDir()

	_, err := ReadLock(dir)
	if err == nil {
		t.Fatal("expected error for unreadable lock file")
	}
	if errors.Is(err, os.ErrNotExist) {
		t.Error("expected a non-ErrNotExist error for permission-denied read")
	}
}

// ---------------------------------------------------------------------------
// WriteLock — atomicity, parent dir creation, InstalledVersion omitempty
// ---------------------------------------------------------------------------

func TestWriteLock_IsAtomic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "genv.lock.json")
	lf := &LockFile{SchemaVersion: schema.Version, Packages: []LockedPackage{}}
	if err := WriteLock(path, lf); err != nil {
		t.Fatalf("WriteLock: %v", err)
	}
	if _, err := os.Stat(path + ".tmp"); !errors.Is(err, os.ErrNotExist) {
		t.Error("WriteLock left .tmp file behind")
	}
}

func TestWriteLock_CreatesParentDirs(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "dir", "genv.lock.json")
	lf := &LockFile{SchemaVersion: schema.Version, Packages: []LockedPackage{}}
	if err := WriteLock(path, lf); err != nil {
		t.Fatalf("WriteLock with nested dirs: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("lock file not created: %v", err)
	}
}

func TestWriteLock_ProducesValidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "genv.lock.json")
	lf := &LockFile{
		SchemaVersion: schema.Version,
		Packages: []LockedPackage{
			{ID: "git", Manager: "brew", PkgName: "git", InstalledVersion: "2.43.0"},
		},
	}
	if err := WriteLock(path, lf); err != nil {
		t.Fatalf("WriteLock: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("lock file is not valid JSON: %v\n%s", err, data)
	}
}

func TestWriteLock_InstalledVersion_OmitEmpty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "genv.lock.json")
	lf := &LockFile{
		SchemaVersion: schema.Version,
		Packages: []LockedPackage{
			{ID: "git", Manager: "brew", PkgName: "git"}, // no InstalledVersion
		},
	}
	if err := WriteLock(path, lf); err != nil {
		t.Fatalf("WriteLock: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if strings.Contains(string(data), "installedVersion") {
		t.Errorf("installedVersion should be omitted when empty, got: %s", data)
	}
}

// TestNew_PackagesNonNil verifies that New() initialises Packages as a non-nil
// empty slice so that it marshals as "[]" rather than "null".
func TestNew_PackagesNonNil(t *testing.T) {
	f := New()
	if f.Packages == nil {
		t.Error("New().Packages must be non-nil to serialize as []")
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
		SchemaVersion: schema.Version,
		Packages: []schema.Package{
			{
				ID:      "firefox",
				Version: "1.0",
				Prefer:  "snap",
				Managers: map[string]string{
					"snap": "firefox",
					"brew": "firefox",
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
	if got.Packages[0].Managers["snap"] != "firefox" {
		t.Errorf("managers roundtrip: got %v", got.Packages[0].Managers)
	}
}
