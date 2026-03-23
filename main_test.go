package main

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ks1686/genv/internal/genvfile"
)

// withWorkDir changes the working directory for the duration of the test.
func withWorkDir(t *testing.T, dir string) {
	t.Helper()
	orig, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(orig); err != nil {
			t.Errorf("cleanup: failed to restore working directory: %v", err)
		}
	})
}

// writeLock writes a minimal lock file with the given packages so tests can
// exercise commands that depend on prior installed state.
func writeLock(t *testing.T, lockPath string, pkgs []genvfile.LockedPackage) {
	t.Helper()
	lf := &genvfile.LockFile{SchemaVersion: "1", Packages: pkgs}
	if err := genvfile.WriteLock(lockPath, lf); err != nil {
		t.Fatalf("writeLock: %v", err)
	}
}

// ---- basic routing ----------------------------------------------------------

func TestRun_NoArgs(t *testing.T) {
	code := run(nil)
	if code != exitUsage {
		t.Errorf("expected exitUsage (%d), got %d", exitUsage, code)
	}
}

func TestRun_UnknownCommand(t *testing.T) {
	code := run([]string{"frobnicate"})
	if code != exitUsage {
		t.Errorf("expected exitUsage (%d), got %d", exitUsage, code)
	}
}

func TestRun_Help(t *testing.T) {
	for _, cmd := range []string{"help", "--help", "-h"} {
		code := run([]string{cmd})
		if code != exitOK {
			t.Errorf("run(%q): expected exitOK, got %d", cmd, code)
		}
	}
}

func TestRun_Version(t *testing.T) {
	for _, cmd := range []string{"version", "--version"} {
		code := run([]string{cmd})
		if code != exitOK {
			t.Errorf("run(%q): expected exitOK, got %d", cmd, code)
		}
	}
}

// ---- genv add ----------------------------------------------------------------
// add writes to genv.json and attempts a best-effort install.
// Install failure is non-fatal (no package manager in CI), so all spec-update
// tests expect exitOK regardless of whether the install succeeds.

func TestAddCmd_CreatesFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "genv.json")

	code := run([]string{"add", "--file", path, "git"})
	if code != exitOK {
		t.Fatalf("expected exitOK, got %d", code)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected genv.json to exist: %v", err)
	}
}

func TestAddCmd_DuplicateFails(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "genv.json")

	if code := run([]string{"add", "--file", path, "git"}); code != exitOK {
		t.Fatalf("first add: expected exitOK, got %d", code)
	}
	code := run([]string{"add", "--file", path, "git"})
	if code != exitLogic {
		t.Errorf("duplicate add: expected exitLogic (%d), got %d", exitLogic, code)
	}
}

func TestAddCmd_WithVersionAndPrefer(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "genv.json")

	code := run([]string{"add", "--file", path, "--version", "0.10.*", "--prefer", "brew", "neovim"})
	if code != exitOK {
		t.Fatalf("expected exitOK, got %d", code)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	s := string(content)
	if !strings.Contains(s, `"0.10.*"`) {
		t.Errorf("version not in file: %s", s)
	}
	if !strings.Contains(s, `"brew"`) {
		t.Errorf("prefer not in file: %s", s)
	}
}

func TestAddCmd_WithManagerFlag(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "genv.json")

	code := run([]string{"add", "--file", path, "--manager", "flatpak:org.mozilla.firefox,brew:firefox", "firefox"})
	if code != exitOK {
		t.Fatalf("expected exitOK, got %d", code)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	s := string(content)
	if !strings.Contains(s, "org.mozilla.firefox") {
		t.Errorf("flatpak name not in file: %s", s)
	}
}

func TestAddCmd_FlagsAfterID(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "genv.json")

	code := run([]string{"add", "--file", path, "neovim", "--version", "0.10.*", "--prefer", "brew"})
	if code != exitOK {
		t.Fatalf("expected exitOK, got %d", code)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	s := string(content)
	if !strings.Contains(s, `"0.10.*"`) {
		t.Errorf("version not written to file (flag after id was ignored): %s", s)
	}
	if !strings.Contains(s, `"brew"`) {
		t.Errorf("prefer not written to file (flag after id was ignored): %s", s)
	}
}

func TestAddCmd_FlagsBeforeID(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "genv.json")

	code := run([]string{"add", "--file", path, "--version", "1.0.*", "--prefer", "brew", "neovim"})
	if code != exitOK {
		t.Fatalf("expected exitOK, got %d", code)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	s := string(content)
	if !strings.Contains(s, `"1.0.*"`) {
		t.Errorf("version not in file: %s", s)
	}
	if !strings.Contains(s, `"brew"`) {
		t.Errorf("prefer not in file: %s", s)
	}
}

func TestAddCmd_UnknownPreferFails(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "genv.json")

	code := run([]string{"add", "--file", path, "--prefer", "yum", "git"})
	if code != exitUsage {
		t.Errorf("expected exitUsage (%d), got %d", exitUsage, code)
	}
}

func TestAddCmd_MissingIDFails(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "genv.json")

	code := run([]string{"add", "--file", path})
	if code != exitUsage {
		t.Errorf("expected exitUsage (%d), got %d", exitUsage, code)
	}
}

func TestAddCmd_InvalidFileFails(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "genv.json")

	if err := os.WriteFile(path, []byte(`{"schemaVersion":"99","packages":[]}`), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	code := run([]string{"add", "--file", path, "git"})
	if code != exitValidation {
		t.Errorf("expected exitValidation (%d), got %d", exitValidation, code)
	}
}

func TestAddCmd_IOError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "genv.json")

	if err := os.WriteFile(path, []byte(`{"schemaVersion":"1","packages":[]}`), 0o200); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	t.Cleanup(func() { os.Chmod(path, 0o644) })
	code := run([]string{"add", "--file", path, "git"})
	if code != exitIO {
		t.Errorf("io error on add: expected exitIO (%d), got %d", exitIO, code)
	}
}

func TestAddCmd_BadManagerFormatFails(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "genv.json")

	code := run([]string{"add", "--file", path, "--manager", "notaformat", "git"})
	if code != exitUsage {
		t.Errorf("bad manager format: expected exitUsage (%d), got %d", exitUsage, code)
	}
}

func TestAddCmd_UnknownManagerKeyInFlagFails(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "genv.json")

	code := run([]string{"add", "--file", path, "--manager", "yum:git", "git"})
	if code != exitUsage {
		t.Errorf("unknown manager key: expected exitUsage (%d), got %d", exitUsage, code)
	}
}

// ---- genv remove -------------------------------------------------------------

func TestRemoveCmd_Basic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "genv.json")

	run([]string{"add", "--file", path, "git"})
	run([]string{"add", "--file", path, "neovim"})

	code := run([]string{"remove", "--file", path, "git"})
	if code != exitOK {
		t.Fatalf("remove: expected exitOK, got %d", code)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	s := string(content)
	if strings.Contains(s, `"git"`) {
		t.Error("git should have been removed from spec")
	}
	if !strings.Contains(s, `"neovim"`) {
		t.Error("neovim should still be present in spec")
	}
}

func TestRemoveCmd_NotFound(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "genv.json")

	run([]string{"add", "--file", path, "git"})

	code := run([]string{"remove", "--file", path, "neovim"})
	if code != exitLogic {
		t.Errorf("expected exitLogic (%d), got %d", exitLogic, code)
	}
}

func TestRemoveCmd_FileNotFound(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "genv.json")

	code := run([]string{"remove", "--file", path, "git"})
	if code != exitLogic {
		t.Errorf("expected exitLogic (%d), got %d", exitLogic, code)
	}
}

func TestRemoveCmd_AliasRm(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "genv.json")

	run([]string{"add", "--file", path, "git"})
	code := run([]string{"rm", "--file", path, "git"})
	if code != exitOK {
		t.Errorf("alias rm: expected exitOK, got %d", code)
	}
}

func TestRemoveCmd_InvalidFileFails(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "genv.json")

	if err := os.WriteFile(path, []byte(`{"schemaVersion":"99","packages":[]}`), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	code := run([]string{"remove", "--file", path, "git"})
	if code != exitValidation {
		t.Errorf("expected exitValidation (%d), got %d", exitValidation, code)
	}
}

func TestRemoveCmd_MissingID(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "genv.json")

	run([]string{"add", "--file", path, "git"})
	code := run([]string{"remove", "--file", path})
	if code != exitUsage {
		t.Errorf("missing id: expected exitUsage (%d), got %d", exitUsage, code)
	}
}

func TestRemoveCmd_IOError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "genv.json")

	if err := os.WriteFile(path, []byte(`{"schemaVersion":"1","packages":[]}`), 0o200); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	t.Cleanup(func() { os.Chmod(path, 0o644) })
	code := run([]string{"remove", "--file", path, "git"})
	if code != exitIO {
		t.Errorf("io error: expected exitIO (%d), got %d", exitIO, code)
	}
}

func TestRemoveCmd_FlagParseError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "genv.json")
	code := run([]string{"remove", "--file", path, "--no-such-flag", "git"})
	if code != exitUsage {
		t.Errorf("unknown flag: expected exitUsage (%d), got %d", exitUsage, code)
	}
}

func TestRemoveCmd_MultiplePackages(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "genv.json")

	run([]string{"add", "--file", path, "git"})
	run([]string{"add", "--file", path, "neovim"})
	run([]string{"add", "--file", path, "firefox"})

	code := run([]string{"remove", "--file", path, "neovim"})
	if code != exitOK {
		t.Fatalf("remove neovim: expected exitOK, got %d", code)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	s := string(content)
	if strings.Contains(s, `"neovim"`) {
		t.Error("neovim should have been removed")
	}
	if !strings.Contains(s, `"git"`) {
		t.Error("git should still be present")
	}
	if !strings.Contains(s, `"firefox"`) {
		t.Error("firefox should still be present")
	}
}

// ---- genv adopt --------------------------------------------------------------
// adopt requires the package to already be installed on the system.
// In CI no package manager is guaranteed to be present, so tests that reach
// the query step will get either "no manager available" or "not installed" —
// both return exitLogic. Tests that fail before the query are deterministic.

func TestAdoptCmd_MissingIDFails(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "genv.json")

	code := run([]string{"adopt", "--file", path})
	if code != exitUsage {
		t.Errorf("expected exitUsage (%d), got %d", exitUsage, code)
	}
}

func TestAdoptCmd_InvalidFileFails(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "genv.json")

	if err := os.WriteFile(path, []byte(`{"schemaVersion":"99","packages":[]}`), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	// adopt checks manager/install before reading the file, so an invalid file
	// is only reached after the query step; in CI this returns exitLogic first.
	code := run([]string{"adopt", "--file", path, "git"})
	if code != exitValidation && code != exitLogic {
		t.Errorf("expected exitValidation or exitLogic, got %d", code)
	}
}

func TestAdoptCmd_AlreadyTrackedFails(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "genv.json")

	run([]string{"add", "--file", path, "git"})
	// adopt on an already-tracked package should return exitLogic.
	// In CI the query step may fail first (also exitLogic), so both are valid.
	code := run([]string{"adopt", "--file", path, "git"})
	if code != exitLogic {
		t.Errorf("expected exitLogic (%d), got %d", exitLogic, code)
	}
}

func TestAdoptCmd_BadManagerFormatFails(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "genv.json")

	code := run([]string{"adopt", "--file", path, "--manager", "notaformat", "git"})
	if code != exitUsage {
		t.Errorf("bad manager format: expected exitUsage (%d), got %d", exitUsage, code)
	}
}

func TestAdoptCmd_UnknownPreferFails(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "genv.json")

	// --prefer validation happens inside commands.Add, which is called after the
	// query; in CI the query fails first. Both lead to exitLogic or exitUsage.
	code := run([]string{"adopt", "--file", path, "--prefer", "yum", "git"})
	if code != exitUsage && code != exitLogic {
		t.Errorf("unknown prefer: expected exitUsage or exitLogic, got %d", code)
	}
}

func TestAdoptCmd_NoManagerOrNotInstalled(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "genv.json")

	// In any environment: either no manager resolves (exitLogic) or the package
	// is not installed (exitLogic). Both are acceptable outcomes for this test.
	code := run([]string{"adopt", "--file", path, "this-package-definitely-does-not-exist-xyzzy"})
	if code != exitLogic {
		t.Errorf("expected exitLogic (%d), got %d", exitLogic, code)
	}
}

// ---- genv disown -------------------------------------------------------------
// disown removes the package from genv.json and the lock file without uninstalling.

func TestDisownCmd_Basic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "genv.json")
	lockPath := filepath.Join(dir, "genv.lock.json")

	// Set up: git and neovim in spec and lock.
	run([]string{"add", "--file", path, "git"})
	run([]string{"add", "--file", path, "neovim"})
	writeLock(t, lockPath, []genvfile.LockedPackage{
		{ID: "git", Manager: "apt", PkgName: "git"},
		{ID: "neovim", Manager: "apt", PkgName: "neovim"},
	})

	code := run([]string{"disown", "--file", path, "git"})
	if code != exitOK {
		t.Fatalf("disown: expected exitOK, got %d", code)
	}

	// git must be gone from spec.
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile spec: %v", err)
	}
	if strings.Contains(string(content), `"git"`) {
		t.Error("git should have been removed from spec")
	}
	if !strings.Contains(string(content), `"neovim"`) {
		t.Error("neovim should still be present in spec")
	}

	// git must be gone from lock.
	lf, err := genvfile.ReadLock(lockPath)
	if err != nil {
		t.Fatalf("ReadLock: %v", err)
	}
	for _, p := range lf.Packages {
		if p.ID == "git" {
			t.Error("git should have been removed from lock")
		}
	}
}

func TestDisownCmd_NotInSpec(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "genv.json")

	run([]string{"add", "--file", path, "git"})
	code := run([]string{"disown", "--file", path, "neovim"})
	if code != exitLogic {
		t.Errorf("expected exitLogic (%d), got %d", exitLogic, code)
	}
}

func TestDisownCmd_FileNotFound(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "genv.json")

	code := run([]string{"disown", "--file", path, "git"})
	if code != exitLogic {
		t.Errorf("expected exitLogic (%d), got %d", exitLogic, code)
	}
}

func TestDisownCmd_MissingID(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "genv.json")

	run([]string{"add", "--file", path, "git"})
	code := run([]string{"disown", "--file", path})
	if code != exitUsage {
		t.Errorf("missing id: expected exitUsage (%d), got %d", exitUsage, code)
	}
}

func TestDisownCmd_InvalidFileFails(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "genv.json")

	if err := os.WriteFile(path, []byte(`{"schemaVersion":"99","packages":[]}`), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	code := run([]string{"disown", "--file", path, "git"})
	if code != exitValidation {
		t.Errorf("expected exitValidation (%d), got %d", exitValidation, code)
	}
}

func TestDisownCmd_NotInLock(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "genv.json")

	// Package in spec but never in lock (never installed by genv).
	run([]string{"add", "--file", path, "git"})
	code := run([]string{"disown", "--file", path, "git"})
	if code != exitOK {
		t.Errorf("not-in-lock disown: expected exitOK, got %d", code)
	}

	// Verify git is gone from spec.
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if strings.Contains(string(content), `"git"`) {
		t.Error("git should have been removed from spec")
	}
}

func TestDisownCmd_FlagParseError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "genv.json")
	code := run([]string{"disown", "--file", path, "--no-such-flag", "git"})
	if code != exitUsage {
		t.Errorf("unknown flag: expected exitUsage (%d), got %d", exitUsage, code)
	}
}

// ---- genv list ---------------------------------------------------------------
// list reads from the lock file, not genv.json.

func TestListCmd_Empty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "genv.json")

	// No lock file exists — should succeed with "no packages installed".
	code := run([]string{"list", "--file", path})
	if code != exitOK {
		t.Errorf("expected exitOK, got %d", code)
	}
}

func TestListCmd_AliasLs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "genv.json")

	code := run([]string{"ls", "--file", path})
	if code != exitOK {
		t.Errorf("alias ls: expected exitOK, got %d", code)
	}
}

func TestListCmd_ShowsLockedPackages(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "genv.json")
	lockPath := filepath.Join(dir, "genv.lock.json")

	writeLock(t, lockPath, []genvfile.LockedPackage{
		{ID: "git", Manager: "apt", PkgName: "git"},
		{ID: "neovim", Manager: "brew", PkgName: "neovim"},
	})

	code := run([]string{"list", "--file", path})
	if code != exitOK {
		t.Errorf("expected exitOK, got %d", code)
	}
}

func TestListCmd_IOError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "genv.json")
	lockPath := filepath.Join(dir, "genv.lock.json")

	// Make the lock file unreadable.
	if err := os.WriteFile(lockPath, []byte(`{"schemaVersion":"1","packages":[]}`), 0o200); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	t.Cleanup(func() { os.Chmod(lockPath, 0o644) })
	code := run([]string{"list", "--file", path})
	if code != exitIO {
		t.Errorf("io error: expected exitIO (%d), got %d", exitIO, code)
	}
}

func TestListCmd_FlagParseError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "genv.json")
	code := run([]string{"list", "--file", path, "--no-such-flag"})
	if code != exitUsage {
		t.Errorf("unknown flag: expected exitUsage (%d), got %d", exitUsage, code)
	}
}

// ---- genv apply --------------------------------------------------------------

func TestApplyCmd_DryRun_NoCrash(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "genv.json")

	run([]string{"add", "--file", path, "git"})
	run([]string{"add", "--file", path, "--prefer", "brew", "neovim"})

	code := run([]string{"apply", "--file", path, "--dry-run"})
	if code != exitOK {
		t.Errorf("dry-run: expected exitOK (%d), got %d", exitOK, code)
	}
}

func TestApplyCmd_DryRun_FileNotFound(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "genv.json")

	code := run([]string{"apply", "--file", path, "--dry-run"})
	if code != exitIO {
		t.Errorf("missing file: expected exitIO (%d), got %d", exitIO, code)
	}
}

func TestApplyCmd_DryRun_InvalidFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "genv.json")

	if err := os.WriteFile(path, []byte(`{"schemaVersion":"99","packages":[]}`), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	code := run([]string{"apply", "--file", path, "--dry-run"})
	if code != exitValidation {
		t.Errorf("invalid file: expected exitValidation (%d), got %d", exitValidation, code)
	}
}

func TestApplyCmd_DryRun_EmptyPackages(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "genv.json")

	if err := os.WriteFile(path, []byte(`{"schemaVersion":"1","packages":[]}`), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	// Empty spec with empty lock → nothing to do, exits OK without prompting.
	code := run([]string{"apply", "--file", path, "--dry-run"})
	if code != exitOK {
		t.Errorf("empty packages: expected exitOK (%d), got %d", exitOK, code)
	}
}

func TestApplyCmd_Strict_DryRun_DoesNotPanic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "genv.json")

	run([]string{"add", "--file", path, "git"})

	// --strict --dry-run must not panic; exit code is environment-dependent.
	code := run([]string{"apply", "--file", path, "--dry-run", "--strict"})
	if code != exitOK && code != exitLogic {
		t.Errorf("strict dry-run: expected exitOK or exitLogic, got %d", code)
	}
}

func TestApplyCmd_DryRun_ShowsReconcilePlan(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "genv.json")
	lockPath := filepath.Join(dir, "genv.lock.json")

	// Desired: git, neovim. Previously applied: git, htop.
	// Expected: install neovim, remove htop, git unchanged.
	run([]string{"add", "--file", path, "git"})
	run([]string{"add", "--file", path, "neovim"})
	writeLock(t, lockPath, []genvfile.LockedPackage{
		{ID: "git", Manager: "apt", PkgName: "git"},
		{ID: "htop", Manager: "apt", PkgName: "htop"},
	})

	code := run([]string{"apply", "--file", path, "--dry-run"})
	if code != exitOK {
		t.Errorf("dry-run with delta: expected exitOK, got %d", code)
	}
}

func TestApplyCmd_AlreadyUpToDate(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "genv.json")
	lockPath := filepath.Join(dir, "genv.lock.json")

	run([]string{"add", "--file", path, "git"})
	writeLock(t, lockPath, []genvfile.LockedPackage{
		{ID: "git", Manager: "apt", PkgName: "git"},
	})

	// Desired == applied → "already up to date", no prompt, exitOK.
	code := run([]string{"apply", "--file", path})
	if code != exitOK {
		t.Errorf("up to date: expected exitOK, got %d", code)
	}
}

func TestApplyCmd_FlagParseError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "genv.json")
	code := run([]string{"apply", "--file", path, "--no-such-flag"})
	if code != exitUsage {
		t.Errorf("unknown flag: expected exitUsage (%d), got %d", exitUsage, code)
	}
}

// ---- helpers ----------------------------------------------------------------

func TestParseManagerFlag(t *testing.T) {
	tests := []struct {
		input   string
		wantLen int
		wantErr bool
	}{
		{"", 0, false},
		{"apt:git", 1, false},
		{"flatpak:org.mozilla.firefox,brew:firefox", 2, false},
		{"badformat", 0, true},
		{"mgr:", 0, true},
		{":name", 0, true},
		{",  ,  ,", 0, false},
	}
	for _, tc := range tests {
		got, err := parseManagerFlag(tc.input)
		if tc.wantErr {
			if err == nil {
				t.Errorf("parseManagerFlag(%q): expected error", tc.input)
			}
		} else {
			if err != nil {
				t.Errorf("parseManagerFlag(%q): unexpected error: %v", tc.input, err)
			}
			if len(got) != tc.wantLen {
				t.Errorf("parseManagerFlag(%q): got %d entries, want %d", tc.input, len(got), tc.wantLen)
			}
		}
	}
}

// captureStdout redirects os.Stdout to a pipe for the duration of fn and
// returns everything written to stdout. Not goroutine-safe; do not call
// t.Parallel() in tests that use this helper.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	rp, wp, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stdout = wp
	fn()
	wp.Close()
	os.Stdout = old
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, rp); err != nil {
		t.Fatalf("io.Copy: %v", err)
	}
	rp.Close()
	return buf.String()
}

// ---- genv scan ---------------------------------------------------------------

func TestScanCmd_NoCrash(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "genv.json")
	// scan must not crash regardless of what managers are available in CI.
	code := run([]string{"scan", "--file", path})
	if code != exitOK {
		t.Errorf("scan: expected exitOK (%d), got %d", exitOK, code)
	}
}

func TestScanCmd_FlagParseError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "genv.json")
	code := run([]string{"scan", "--file", path, "--no-such-flag"})
	if code != exitUsage {
		t.Errorf("unknown flag: expected exitUsage (%d), got %d", exitUsage, code)
	}
}

func TestScanCmd_InvalidFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "genv.json")
	if err := os.WriteFile(path, []byte(`{"schemaVersion":"99","packages":[]}`), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	code := run([]string{"scan", "--file", path})
	if code != exitValidation {
		t.Errorf("invalid file: expected exitValidation (%d), got %d", exitValidation, code)
	}
}

func TestScanCmd_JsonOutput(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "genv.json")
	var code int
	out := captureStdout(t, func() {
		code = run([]string{"scan", "--file", path, "--json"})
	})
	if code != exitOK {
		t.Fatalf("scan --json: expected exitOK (%d), got %d", exitOK, code)
	}
	var env map[string]interface{}
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("scan --json output is not valid JSON: %v\noutput: %q", err, out)
	}
	if env["command"] != "scan" {
		t.Errorf("JSON command: got %v, want %q", env["command"], "scan")
	}
	if _, ok := env["ok"]; !ok {
		t.Error("JSON envelope missing 'ok' field")
	}
}

func TestScanCmd_Debug_NoCrash(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "genv.json")
	code := run([]string{"scan", "--file", path, "--debug"})
	if code != exitOK {
		t.Errorf("scan --debug: expected exitOK (%d), got %d", exitOK, code)
	}
}

// ---- genv status -------------------------------------------------------------

func TestStatusCmd_FileNotFound(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "genv.json")
	code := run([]string{"status", "--file", path})
	if code != exitIO {
		t.Errorf("missing spec: expected exitIO (%d), got %d", exitIO, code)
	}
}

func TestStatusCmd_NothingTracked(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "genv.json")
	if err := os.WriteFile(path, []byte(`{"schemaVersion":"1","packages":[]}`), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	code := run([]string{"status", "--file", path})
	if code != exitOK {
		t.Errorf("empty: expected exitOK (%d), got %d", exitOK, code)
	}
}

func TestStatusCmd_AllOK(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "genv.json")
	lockPath := filepath.Join(dir, "genv.lock.json")

	run([]string{"add", "--file", path, "git"})
	writeLock(t, lockPath, []genvfile.LockedPackage{
		{ID: "git", Manager: "apt", PkgName: "git"},
	})

	code := run([]string{"status", "--file", path})
	if code != exitOK {
		t.Errorf("all ok: expected exitOK (%d), got %d", exitOK, code)
	}
}

func TestStatusCmd_MissingEntry(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "genv.json")

	// Package in spec but not in lock — "missing" exits OK (not drift/extra).
	run([]string{"add", "--file", path, "git"})
	code := run([]string{"status", "--file", path})
	if code != exitOK {
		t.Errorf("missing: expected exitOK (%d), got %d", exitOK, code)
	}
}

func TestStatusCmd_ExtraEntry(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "genv.json")
	lockPath := filepath.Join(dir, "genv.lock.json")

	// Empty spec but lock has git → "extra" exits with exitLogic.
	if err := os.WriteFile(path, []byte(`{"schemaVersion":"1","packages":[]}`), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	writeLock(t, lockPath, []genvfile.LockedPackage{
		{ID: "git", Manager: "apt", PkgName: "git"},
	})
	code := run([]string{"status", "--file", path})
	if code != exitLogic {
		t.Errorf("extra: expected exitLogic (%d), got %d", exitLogic, code)
	}
}

func TestStatusCmd_DriftEntry(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "genv.json")
	lockPath := filepath.Join(dir, "genv.lock.json")

	run([]string{"add", "--file", path, "--version", "2.0.*", "git"})
	// Lock records version 1.x — does not satisfy "2.0.*" → drift.
	writeLock(t, lockPath, []genvfile.LockedPackage{
		{ID: "git", Manager: "apt", PkgName: "git", InstalledVersion: "1.9.0"},
	})
	code := run([]string{"status", "--file", path})
	if code != exitLogic {
		t.Errorf("drift: expected exitLogic (%d), got %d", exitLogic, code)
	}
}

func TestStatusCmd_InvalidFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "genv.json")
	if err := os.WriteFile(path, []byte(`{"schemaVersion":"99","packages":[]}`), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	code := run([]string{"status", "--file", path})
	if code != exitValidation {
		t.Errorf("invalid file: expected exitValidation (%d), got %d", exitValidation, code)
	}
}

func TestStatusCmd_FlagParseError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "genv.json")
	code := run([]string{"status", "--file", path, "--no-such-flag"})
	if code != exitUsage {
		t.Errorf("unknown flag: expected exitUsage (%d), got %d", exitUsage, code)
	}
}

func TestStatusCmd_JsonOutput(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "genv.json")
	lockPath := filepath.Join(dir, "genv.lock.json")

	run([]string{"add", "--file", path, "git"})
	writeLock(t, lockPath, []genvfile.LockedPackage{
		{ID: "git", Manager: "apt", PkgName: "git"},
	})

	var code int
	out := captureStdout(t, func() {
		code = run([]string{"status", "--file", path, "--json"})
	})
	if code != exitOK {
		t.Fatalf("status --json: expected exitOK (%d), got %d", exitOK, code)
	}
	var env map[string]interface{}
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("status --json output is not valid JSON: %v\noutput: %q", err, out)
	}
	if env["command"] != "status" {
		t.Errorf("JSON command: got %v, want %q", env["command"], "status")
	}
	if _, ok := env["ok"]; !ok {
		t.Error("JSON envelope missing 'ok' field")
	}
	data, ok := env["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("JSON data field missing or wrong type: %v", env["data"])
	}
	if _, ok := data["entries"]; !ok {
		t.Error("JSON data missing 'entries' field")
	}
}

func TestStatusCmd_Debug_NoCrash(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "genv.json")
	if err := os.WriteFile(path, []byte(`{"schemaVersion":"1","packages":[]}`), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	code := run([]string{"status", "--file", path, "--debug"})
	if code != exitOK {
		t.Errorf("status --debug: expected exitOK (%d), got %d", exitOK, code)
	}
}

// ---- genv apply new flags ----------------------------------------------------

func TestApplyCmd_Yes_AlreadyUpToDate(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "genv.json")
	lockPath := filepath.Join(dir, "genv.lock.json")

	run([]string{"add", "--file", path, "git"})
	writeLock(t, lockPath, []genvfile.LockedPackage{
		{ID: "git", Manager: "apt", PkgName: "git"},
	})

	// --yes with an up-to-date state exits OK immediately (no prompt, no work).
	code := run([]string{"apply", "--file", path, "--yes"})
	if code != exitOK {
		t.Errorf("--yes up to date: expected exitOK (%d), got %d", exitOK, code)
	}
}

func TestApplyCmd_Debug_DryRun_NoCrash(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "genv.json")
	run([]string{"add", "--file", path, "git"})
	code := run([]string{"apply", "--file", path, "--dry-run", "--debug"})
	if code != exitOK {
		t.Errorf("--debug dry-run: expected exitOK (%d), got %d", exitOK, code)
	}
}

func TestApplyCmd_Timeout_DryRun_NoCrash(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "genv.json")
	run([]string{"add", "--file", path, "git"})
	code := run([]string{"apply", "--file", path, "--dry-run", "--timeout", "5m"})
	if code != exitOK {
		t.Errorf("--timeout dry-run: expected exitOK (%d), got %d", exitOK, code)
	}
}

func TestApplyCmd_DryRun_JsonOutput(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "genv.json")
	run([]string{"add", "--file", path, "git"})

	var code int
	out := captureStdout(t, func() {
		code = run([]string{"apply", "--file", path, "--dry-run", "--json"})
	})
	if code != exitOK {
		t.Fatalf("apply --dry-run --json: expected exitOK (%d), got %d", exitOK, code)
	}
	var env map[string]interface{}
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("apply --json output is not valid JSON: %v\noutput: %q", err, out)
	}
	if env["command"] != "apply" {
		t.Errorf("JSON command: got %v, want %q", env["command"], "apply")
	}
	if _, ok := env["ok"]; !ok {
		t.Error("JSON envelope missing 'ok' field")
	}
	data, ok := env["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("JSON data field missing or wrong type: %v", env["data"])
	}
	if _, ok := data["toInstall"]; !ok {
		t.Error("JSON plan data missing 'toInstall' field")
	}
}

func TestApplyCmd_AlreadyUpToDate_JsonOutput(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "genv.json")
	lockPath := filepath.Join(dir, "genv.lock.json")

	run([]string{"add", "--file", path, "git"})
	writeLock(t, lockPath, []genvfile.LockedPackage{
		{ID: "git", Manager: "apt", PkgName: "git"},
	})

	var code int
	out := captureStdout(t, func() {
		code = run([]string{"apply", "--file", path, "--json"})
	})
	// Up-to-date with --json: the "already up to date" path still exits OK.
	// In JSON mode there's no work to do, so the apply skips to the plan check.
	// Note: apply --json without --dry-run and with toInstall==0 exits OK.
	if code != exitOK {
		t.Errorf("apply --json up-to-date: expected exitOK (%d), got %d\noutput: %s", exitOK, code, out)
	}
}

func TestExtractPositional(t *testing.T) {
	tests := []struct {
		name          string
		args          []string
		wantPos       string
		wantFlagCount int
	}{
		{"empty args", nil, "", 0},
		{"positional only", []string{"git"}, "git", 0},
		{"flag before positional", []string{"--prefer", "brew", "neovim"}, "neovim", 2},
		{"flag after positional", []string{"neovim", "--prefer", "brew"}, "neovim", 2},
		{"flag=value form", []string{"--prefer=brew", "neovim"}, "neovim", 1},
		{"multiple flags before and after", []string{"--version", "0.10.*", "neovim", "--prefer", "brew"}, "neovim", 4},
		{"only flags no positional", []string{"--prefer", "brew"}, "", 2},
		{"first non-flag is positional second is ignored", []string{"first", "second"}, "first", 0},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			pos, flagArgs := extractPositional(tc.args)
			if pos != tc.wantPos {
				t.Errorf("positional: got %q, want %q", pos, tc.wantPos)
			}
			if len(flagArgs) != tc.wantFlagCount {
				t.Errorf("flagArgs length: got %d, want %d (args: %v)", len(flagArgs), tc.wantFlagCount, flagArgs)
			}
		})
	}
}
