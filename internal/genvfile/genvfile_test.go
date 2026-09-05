package genvfile

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/ks1686/genv/internal/schema"
	"github.com/ks1686/genv/internal/testutil"
)

func TestNew(t *testing.T) {
	f := New()
	if f.SchemaVersion != schema.Version8 {
		t.Errorf("SchemaVersion = %q, want %q", f.SchemaVersion, schema.Version8)
	}
	if f.Defaults == nil {
		t.Error("Defaults must be non-nil")
	}
	if len(f.Targets) != len(schema.KnownTargets) {
		t.Errorf("Targets = %d, want %d known targets", len(f.Targets), len(schema.KnownTargets))
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
		return
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

func TestWrite_V8OmitsEmptyLegacyTopLevelFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "genv.json")

	original := &schema.GenvFile{
		SchemaVersion: schema.Version8,
		Targets: map[string]*schema.TargetBundle{
			"arch": {},
		},
	}
	if err := Write(path, original); err != nil {
		t.Fatalf("Write: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	for _, forbidden := range []string{
		`"packages": null`,
		`"env": null`,
		`"shell": null`,
		`"files": null`,
		`"services": null`,
		`"hooks": null`,
	} {
		if strings.Contains(string(data), forbidden) {
			t.Fatalf("v8 write emitted %s:\n%s", forbidden, data)
		}
	}
	if strings.Contains(string(data), `"packages"`) {
		t.Fatalf("v8 write emitted empty top-level packages:\n%s", data)
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
		return
	}
	if f.SchemaVersion != schema.Version8 {
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
	if runtime.GOOS == "windows" {
		t.Skip("chmod 0200 does not deny the file owner on Windows")
	}
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
		return
	}
	if len(got.Packages) != 1 || got.Packages[0].ID != "git" {
		t.Errorf("expected 1 package 'git' after overwrite, got: %+v", got.Packages)
	}
}

// ---------------------------------------------------------------------------
// LockPathFrom — lock lives in the genv config directory, ignoring spec path.
// ---------------------------------------------------------------------------

func TestLockPathFrom(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", filepath.FromSlash("/custom/config"))
	got := LockPathFrom("/tmp/repo/genv.json")
	want := filepath.Join(filepath.FromSlash("/tmp/repo"), "genv.lock.json")
	if got != want {
		t.Errorf("LockPathFrom(spec) = %q, want lock next to spec %q", got, want)
	}
}

func TestLockPathFrom_EmptySpecUsesDefaultDir(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", filepath.FromSlash("/custom/config"))
	got := LockPathFrom("")
	want := filepath.Join(filepath.FromSlash("/custom/config"), "genv", "genv.lock.json")
	if got != want {
		t.Errorf("LockPathFrom(\"\") = %q, want %q", got, want)
	}
}

func TestLockPathFrom_FallsBackToHomeConfig(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "")
	home := filepath.FromSlash("/home/testuser")
	testutil.SetHome(t, home)
	got := LockPathFrom("")
	want := filepath.Join(home, ".config", "genv", "genv.lock.json")
	if got != want {
		t.Errorf("LockPathFrom fallback = %q, want %q", got, want)
	}
}

func TestResolveStateDir(t *testing.T) {
	spec := filepath.Join(t.TempDir(), "nested", "genv.json")
	got, err := ResolveStateDir(spec, "")
	if err != nil {
		t.Fatal(err)
	}
	if got != filepath.Dir(spec) {
		t.Fatalf("ResolveStateDir(spec) = %q, want %q", got, filepath.Dir(spec))
	}
	override := t.TempDir()
	got, err = ResolveStateDir(spec, override)
	if err != nil {
		t.Fatal(err)
	}
	absOverride, err := filepath.Abs(override)
	if err != nil {
		t.Fatal(err)
	}
	if got != absOverride {
		t.Fatalf("ResolveStateDir(spec, override) = %q, want %q", got, absOverride)
	}
}

func TestWithinDir(t *testing.T) {
	dir := t.TempDir()
	if !WithinDir(dir, filepath.Join(dir, "env.sh")) {
		t.Fatal("expected env.sh inside dir")
	}
	if !WithinDir(dir, dir) {
		t.Fatal("expected dir to contain itself")
	}
	if WithinDir(dir, filepath.Join(dir, "..", "outside")) {
		t.Fatal("parent path must not be inside dir")
	}
}

// ---------------------------------------------------------------------------
// DefaultDir / DefaultSpecPath — XDG_CONFIG_HOME support
// ---------------------------------------------------------------------------

func TestDefaultDir_UsesXDG(t *testing.T) {
	xdg := filepath.FromSlash("/custom/config")
	t.Setenv("XDG_CONFIG_HOME", xdg)
	dir, err := DefaultDir()
	if err != nil {
		t.Fatalf("DefaultDir: %v", err)
	}
	if !strings.HasPrefix(dir, xdg) {
		t.Errorf("DefaultDir with XDG_CONFIG_HOME: got %q, expected prefix %s", dir, xdg)
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
		return
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
		Target:        "arch",
		GOOS:          "linux",
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
	if got.Target != "arch" {
		t.Errorf("Target: got %q, want \"arch\"", got.Target)
	}
	if got.GOOS != "linux" {
		t.Errorf("GOOS: got %q, want \"linux\"", got.GOOS)
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

func TestWriteLock_Mode0600(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file mode bits are not POSIX on Windows")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "genv.lock.json")
	lf := &LockFile{SchemaVersion: schema.Version, Packages: []LockedPackage{}}
	if err := WriteLock(path, lf); err != nil {
		t.Fatalf("WriteLock: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("lock mode = %o, want 0600", got)
	}
}

func TestWritePrivate_Mode0600(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file mode bits are not POSIX on Windows")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "secret")
	if err := WritePrivate(path, []byte("ok\n")); err != nil {
		t.Fatalf("WritePrivate: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got) != "ok\n" {
		t.Fatalf("content = %q, want %q", got, "ok\n")
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("mode = %o, want 0600", perm)
	}
}

func TestNew_WritesValidV8(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "genv.json")
	if err := Write(path, New()); err != nil {
		t.Fatalf("Write New(): %v", err)
	}
	got, err := Read(path)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if got.SchemaVersion != schema.Version8 {
		t.Fatalf("schemaVersion = %q", got.SchemaVersion)
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

func TestWrite_RejectsInvalidSchemaContent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "genv.json")

	f := &schema.GenvFile{
		SchemaVersion: schema.Version4,
		Packages:      []schema.Package{},
		Services: map[string]schema.Service{
			"bad\nname": {Start: []string{"echo", "ok"}},
		},
	}

	err := Write(path, f)
	if err == nil {
		t.Fatal("expected Write to reject invalid content")
	}
	if !errors.Is(err, ErrInvalidFile) {
		t.Fatalf("expected ErrInvalidFile, got: %v", err)
	}
}

func TestDefaultDir_HomeDirError(t *testing.T) {
	// First ensure XDG_CONFIG_HOME is unset
	t.Setenv("XDG_CONFIG_HOME", "")

	t.Setenv("HOME", "")
	t.Setenv("USERPROFILE", "")
	t.Setenv("HOMEDRIVE", "")
	t.Setenv("HOMEPATH", "")

	_, err := DefaultDir()
	if err == nil {
		t.Error("DefaultDir: expected error when home directory cannot be determined, got nil")
	} else if !strings.Contains(err.Error(), "cannot determine home directory") {
		t.Errorf("DefaultDir: expected error to mention 'cannot determine home directory', got %v", err)
	}
}
