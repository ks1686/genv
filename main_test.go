package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ks1686/genv/internal/adapter"
	"github.com/ks1686/genv/internal/genvfile"
	"github.com/ks1686/genv/internal/resolver"
	"github.com/ks1686/genv/internal/schema"
	"github.com/ks1686/genv/internal/service"
	"github.com/ks1686/genv/internal/upgrade"
)

// TestMain disables interactive prompts (package search picker, remove fuzzy
// match) for all unit tests so they run non-interactively. It also isolates
// the genv config directory so tests do not read or write the developer's
// real lock file now that LockPathFrom uses the genv config directory.
func TestMain(m *testing.M) {
	_ = os.Setenv("GENV_NO_INTERACTIVE", "1")
	if os.Getenv("XDG_CONFIG_HOME") == "" {
		dir, err := os.MkdirTemp("", "genv-test-xdg-*")
		if err == nil {
			_ = os.Setenv("XDG_CONFIG_HOME", dir)
		}
	}
	// Shadow real package managers with fakes so CLI-level tests below never
	// install/uninstall/upgrade anything on the machine actually running
	// `go test`. See main_fakebin_test.go.
	if err := installFakeManagers(); err != nil {
		_, _ = os.Stderr.WriteString("genv test: " + err.Error() + "\n")
		os.Exit(1)
	}
	os.Exit(m.Run())
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

func TestMutationCommands_V8WriteActiveTarget(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	path := filepath.Join(dir, "genv.json")
	lockPath := filepath.Join(dir, "genv.lock.json")
	writeTestFile(t, path, `{"schemaVersion":"8","targets":{"arch":{},"macos":{}}}`)
	writeLock(t, lockPath, nil)

	cases := []struct {
		name string
		args []string
	}{
		{"add", []string{"add", "--file", path, "--lock-file", lockPath, "--target", "arch", "--no-hooks", "--no-search", "git"}},
		{"env set", []string{"env", "set", "--file", path, "--target", "arch", "EDITOR", "nvim"}},
		{"shell alias set", []string{"shell", "alias", "set", "--file", path, "--target", "arch", "ll", "ls -la"}},
		{"service add", []string{"service", "add", "--file", path, "--target", "arch", "worker", "--start", "worker"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if code := run(tc.args); code != exitOK {
				t.Fatalf("run(%v) = %d, want %d", tc.args, code, exitOK)
			}
		})
	}

	f, err := genvfile.Read(path)
	if err != nil {
		t.Fatalf("read spec: %v", err)
	}
	if len(f.Packages) != 0 || f.Env != nil || f.Shell != nil || f.Services != nil {
		t.Fatalf("v8 mutation wrote top-level fields: %+v", f)
	}
	arch := f.Targets["arch"]
	if arch == nil {
		t.Fatal("targets.arch missing")
	}
	if len(arch.Packages) != 1 || arch.Packages[0].ID != "git" {
		t.Fatalf("targets.arch packages = %+v, want git", arch.Packages)
	}
	if arch.Env["EDITOR"] == nil || arch.Env["EDITOR"].Value != "nvim" {
		t.Fatalf("targets.arch env = %+v, want EDITOR=nvim", arch.Env)
	}
	if arch.Shell == nil || arch.Shell.Aliases["ll"] == nil || arch.Shell.Aliases["ll"].Value != "ls -la" {
		t.Fatalf("targets.arch shell = %+v, want ll alias", arch.Shell)
	}
	if arch.Services["worker"] == nil || len(arch.Services["worker"].Start) != 1 || arch.Services["worker"].Start[0] != "worker" {
		t.Fatalf("targets.arch services = %+v, want worker", arch.Services)
	}
	if macos := f.Targets["macos"]; macos == nil || len(macos.Packages) != 0 || macos.Env != nil || macos.Shell != nil || macos.Services != nil {
		t.Fatalf("targets.macos mutated unexpectedly: %+v", macos)
	}
}

func TestMutationCommands_V8MissingTargetFails(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	path := filepath.Join(dir, "genv.json")
	writeTestFile(t, path, `{"schemaVersion":"8","targets":{"arch":{}}}`)

	code := run([]string{"env", "set", "--file", path, "--target", "ubuntu", "EDITOR", "nvim"})
	if code != exitValidation {
		t.Fatalf("missing v8 target exit = %d, want %d", code, exitValidation)
	}
}

func TestMutationRemoveCommands_V8UseActiveTarget(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	path := filepath.Join(dir, "genv.json")
	lockPath := filepath.Join(dir, "genv.lock.json")
	writeTestFile(t, path, `{
		"schemaVersion":"8",
		"targets":{
			"arch":{
				"packages":[{"id":"git"}],
				"env":{"EDITOR":{"value":"nvim"}},
				"shell":{"aliases":{"ll":{"value":"ls -la"}}},
				"services":{"worker":{"start":["worker"]}}
			},
			"macos":{
				"packages":[{"id":"git"}],
				"env":{"EDITOR":{"value":"vim"}},
				"shell":{"aliases":{"ll":{"value":"ls -l"}}},
				"services":{"worker":{"start":["worker"]}}
			}
		}
	}`)
	writeLock(t, lockPath, nil)

	cases := []struct {
		name string
		args []string
	}{
		{"remove", []string{"remove", "--file", path, "--lock-file", lockPath, "--target", "arch", "--no-hooks", "git"}},
		{"env unset", []string{"env", "unset", "--file", path, "--target", "arch", "EDITOR"}},
		{"shell alias unset", []string{"shell", "alias", "unset", "--file", path, "--target", "arch", "ll"}},
		{"service remove", []string{"service", "remove", "--file", path, "--target", "arch", "worker"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if code := run(tc.args); code != exitOK {
				t.Fatalf("run(%v) = %d, want %d", tc.args, code, exitOK)
			}
		})
	}

	f, err := genvfile.Read(path)
	if err != nil {
		t.Fatalf("read spec: %v", err)
	}
	arch := f.Targets["arch"]
	if len(arch.Packages) != 0 {
		t.Fatalf("targets.arch packages not removed: %+v", arch.Packages)
	}
	if _, ok := arch.Env["EDITOR"]; ok {
		t.Fatalf("targets.arch env not removed: %+v", arch.Env)
	}
	if _, ok := arch.Shell.Aliases["ll"]; ok {
		t.Fatalf("targets.arch alias not removed: %+v", arch.Shell.Aliases)
	}
	if _, ok := arch.Services["worker"]; ok {
		t.Fatalf("targets.arch service not removed: %+v", arch.Services)
	}
	macos := f.Targets["macos"]
	if len(macos.Packages) != 1 || macos.Env["EDITOR"] == nil || macos.Shell.Aliases["ll"] == nil || macos.Services["worker"] == nil {
		t.Fatalf("targets.macos mutated unexpectedly: %+v", macos)
	}
}

// ---- genv add ----------------------------------------------------------------
// add writes to genv.json and attempts a best-effort installation.
// Installation failure is non-fatal (no package manager in CI), so all spec-update
// tests expect exitOK regardless of whether the installation succeeds.

func TestAddCmd_CreatesFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
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
	t.Setenv("XDG_CONFIG_HOME", dir)
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
	t.Setenv("XDG_CONFIG_HOME", dir)
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
	t.Setenv("XDG_CONFIG_HOME", dir)
	path := filepath.Join(dir, "genv.json")

	code := run([]string{"add", "--file", path, "--manager", "snap:hello,brew:hello", "hello"})
	if code != exitOK {
		t.Fatalf("expected exitOK, got %d", code)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	s := string(content)
	if !strings.Contains(s, `"snap"`) {
		t.Errorf("snap manager not in file: %s", s)
	}
}

func TestAddCmd_FlagsAfterID(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
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
	t.Setenv("XDG_CONFIG_HOME", dir)
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
	t.Setenv("XDG_CONFIG_HOME", dir)
	path := filepath.Join(dir, "genv.json")

	code := run([]string{"add", "--file", path, "--prefer", "yum", "git"})
	if code != exitUsage {
		t.Errorf("expected exitUsage (%d), got %d", exitUsage, code)
	}
}

func TestAddCmd_MissingIDFails(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	path := filepath.Join(dir, "genv.json")

	code := run([]string{"add", "--file", path})
	if code != exitUsage {
		t.Errorf("expected exitUsage (%d), got %d", exitUsage, code)
	}
}

func TestAddCmd_InvalidFileFails(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
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
	t.Setenv("XDG_CONFIG_HOME", dir)
	path := filepath.Join(dir, "genv.json")

	if err := os.WriteFile(path, []byte(`{"schemaVersion":"1","packages":[]}`), 0o200); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0o644) })
	code := run([]string{"add", "--file", path, "git"})
	if code != exitIO {
		t.Errorf("io error on add: expected exitIO (%d), got %d", exitIO, code)
	}
}

func TestAddCmd_BadManagerFormatFails(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	path := filepath.Join(dir, "genv.json")

	code := run([]string{"add", "--file", path, "--manager", "notaformat", "git"})
	if code != exitUsage {
		t.Errorf("bad manager format: expected exitUsage (%d), got %d", exitUsage, code)
	}
}

func TestAddCmd_UnknownManagerKeyInFlagFails(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	path := filepath.Join(dir, "genv.json")

	code := run([]string{"add", "--file", path, "--manager", "yum:git", "git"})
	if code != exitUsage {
		t.Errorf("unknown manager key: expected exitUsage (%d), got %d", exitUsage, code)
	}
}

// ---- genv remove -------------------------------------------------------------

func TestRemoveCmd_Basic(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
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
	t.Setenv("XDG_CONFIG_HOME", dir)
	path := filepath.Join(dir, "genv.json")

	run([]string{"add", "--file", path, "git"})

	code := run([]string{"remove", "--file", path, "neovim"})
	if code != exitLogic {
		t.Errorf("expected exitLogic (%d), got %d", exitLogic, code)
	}
}

func TestRemoveCmd_FileNotFound(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	path := filepath.Join(dir, "genv.json")

	code := run([]string{"remove", "--file", path, "git"})
	if code != exitLogic {
		t.Errorf("expected exitLogic (%d), got %d", exitLogic, code)
	}
}

func TestRemoveCmd_AliasRm(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	path := filepath.Join(dir, "genv.json")

	run([]string{"add", "--file", path, "git"})
	code := run([]string{"rm", "--file", path, "git"})
	if code != exitOK {
		t.Errorf("alias rm: expected exitOK, got %d", code)
	}
}

func TestRemoveCmd_InvalidFileFails(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
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
	t.Setenv("XDG_CONFIG_HOME", dir)
	path := filepath.Join(dir, "genv.json")

	run([]string{"add", "--file", path, "git"})
	code := run([]string{"remove", "--file", path})
	if code != exitUsage {
		t.Errorf("missing id: expected exitUsage (%d), got %d", exitUsage, code)
	}
}

func TestRemoveCmd_IOError(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	path := filepath.Join(dir, "genv.json")

	if err := os.WriteFile(path, []byte(`{"schemaVersion":"1","packages":[]}`), 0o200); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0o644) })
	code := run([]string{"remove", "--file", path, "git"})
	if code != exitIO {
		t.Errorf("io error: expected exitIO (%d), got %d", exitIO, code)
	}
}

func TestRemoveCmd_FlagParseError(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	path := filepath.Join(dir, "genv.json")
	code := run([]string{"remove", "--file", path, "--no-such-flag", "git"})
	if code != exitUsage {
		t.Errorf("unknown flag: expected exitUsage (%d), got %d", exitUsage, code)
	}
}

func TestRemoveCmd_MultiplePackages(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
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
	t.Setenv("XDG_CONFIG_HOME", dir)
	path := filepath.Join(dir, "genv.json")

	code := run([]string{"adopt", "--file", path})
	if code != exitUsage {
		t.Errorf("expected exitUsage (%d), got %d", exitUsage, code)
	}
}

func TestAdoptCmd_InvalidFileFails(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
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
	t.Setenv("XDG_CONFIG_HOME", dir)
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
	t.Setenv("XDG_CONFIG_HOME", dir)
	path := filepath.Join(dir, "genv.json")

	code := run([]string{"adopt", "--file", path, "--manager", "notaformat", "git"})
	if code != exitUsage {
		t.Errorf("bad manager format: expected exitUsage (%d), got %d", exitUsage, code)
	}
}

func TestAdoptCmd_UnknownPreferFails(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
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
	t.Setenv("XDG_CONFIG_HOME", dir)
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
	t.Setenv("XDG_CONFIG_HOME", dir)
	path := filepath.Join(dir, "genv.json")
	lockPath := genvfile.LockPathFrom(path)

	// Set up: git and neovim in spec and lock.
	run([]string{"add", "--file", path, "git"})
	run([]string{"add", "--file", path, "neovim"})
	writeLock(t, lockPath, []genvfile.LockedPackage{
		{ID: "git", Manager: "brew", PkgName: "git"},
		{ID: "neovim", Manager: "brew", PkgName: "neovim"},
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
	t.Setenv("XDG_CONFIG_HOME", dir)
	path := filepath.Join(dir, "genv.json")

	run([]string{"add", "--file", path, "git"})
	code := run([]string{"disown", "--file", path, "neovim"})
	if code != exitLogic {
		t.Errorf("expected exitLogic (%d), got %d", exitLogic, code)
	}
}

func TestDisownCmd_FileNotFound(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	path := filepath.Join(dir, "genv.json")

	code := run([]string{"disown", "--file", path, "git"})
	if code != exitLogic {
		t.Errorf("expected exitLogic (%d), got %d", exitLogic, code)
	}
}

func TestDisownCmd_MissingID(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	path := filepath.Join(dir, "genv.json")

	run([]string{"add", "--file", path, "git"})
	code := run([]string{"disown", "--file", path})
	if code != exitUsage {
		t.Errorf("missing id: expected exitUsage (%d), got %d", exitUsage, code)
	}
}

func TestDisownCmd_InvalidFileFails(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
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
	t.Setenv("XDG_CONFIG_HOME", dir)
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
	t.Setenv("XDG_CONFIG_HOME", dir)
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
	t.Setenv("XDG_CONFIG_HOME", dir)
	path := filepath.Join(dir, "genv.json")

	// No lock file exists — should succeed with "no packages installed".
	code := run([]string{"list", "--file", path})
	if code != exitOK {
		t.Errorf("expected exitOK, got %d", code)
	}
}

func TestListCmd_AliasLs(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	path := filepath.Join(dir, "genv.json")

	code := run([]string{"ls", "--file", path})
	if code != exitOK {
		t.Errorf("alias ls: expected exitOK, got %d", code)
	}
}

func TestListCmd_ShowsLockedPackages(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	path := filepath.Join(dir, "genv.json")
	lockPath := genvfile.LockPathFrom(path)

	writeLock(t, lockPath, []genvfile.LockedPackage{
		{ID: "git", Manager: "brew", PkgName: "git"},
		{ID: "neovim", Manager: "brew", PkgName: "neovim"},
	})

	code := run([]string{"list", "--file", path})
	if code != exitOK {
		t.Errorf("expected exitOK, got %d", code)
	}
}

func TestListCmd_IOError(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	path := filepath.Join(dir, "genv.json")
	lockPath := genvfile.LockPathFrom(path)

	// Make the lock file unreadable.
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(lockPath, []byte(`{"schemaVersion":"1","packages":[]}`), 0o200); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(lockPath, 0o644) })
	code := run([]string{"list", "--file", path})
	if code != exitIO {
		t.Errorf("io error: expected exitIO (%d), got %d", exitIO, code)
	}
}

func TestListCmd_FlagParseError(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	path := filepath.Join(dir, "genv.json")
	code := run([]string{"list", "--file", path, "--no-such-flag"})
	if code != exitUsage {
		t.Errorf("unknown flag: expected exitUsage (%d), got %d", exitUsage, code)
	}
}

// ---- genv apply --------------------------------------------------------------

func TestApplyCmd_DryRun_NoCrash(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
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
	t.Setenv("XDG_CONFIG_HOME", dir)
	path := filepath.Join(dir, "genv.json")

	code := run([]string{"apply", "--file", path, "--dry-run"})
	if code != exitIO {
		t.Errorf("missing file: expected exitIO (%d), got %d", exitIO, code)
	}
}

func TestApplyCmd_DryRun_InvalidFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
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
	t.Setenv("XDG_CONFIG_HOME", dir)
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
	t.Setenv("XDG_CONFIG_HOME", dir)
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
	t.Setenv("XDG_CONFIG_HOME", dir)
	path := filepath.Join(dir, "genv.json")
	lockPath := genvfile.LockPathFrom(path)

	// Desired: git, neovim. Previously applied: git, htop.
	// Expected: install neovim, remove htop, git unchanged.
	run([]string{"add", "--file", path, "git"})
	run([]string{"add", "--file", path, "neovim"})
	writeLock(t, lockPath, []genvfile.LockedPackage{
		{ID: "git", Manager: "brew", PkgName: "git"},
		{ID: "htop", Manager: "brew", PkgName: "htop"},
	})

	code := run([]string{"apply", "--file", path, "--dry-run"})
	if code != exitOK {
		t.Errorf("dry-run with delta: expected exitOK, got %d", code)
	}
}

func TestApplyCmd_AlreadyUpToDate(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	path := filepath.Join(dir, "genv.json")
	lockPath := genvfile.LockPathFrom(path)

	run([]string{"add", "--file", path, "git"})
	writeLock(t, lockPath, []genvfile.LockedPackage{
		{ID: "git", Manager: "brew", PkgName: "git"},
	})

	// Desired == applied → "already up to date", no prompt, exitOK.
	code := run([]string{"apply", "--file", path})
	if code != exitOK {
		t.Errorf("up to date: expected exitOK, got %d", code)
	}
}

func TestApplyCmd_FlagParseError(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	path := filepath.Join(dir, "genv.json")
	code := run([]string{"apply", "--file", path, "--no-such-flag"})
	if code != exitUsage {
		t.Errorf("unknown flag: expected exitUsage (%d), got %d", exitUsage, code)
	}
}

func TestApply_CreatesLinks(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "xdg"))
	specPath := filepath.Join(dir, "genv.json")
	lockPath := filepath.Join(dir, "genv.lock.json")
	sourcePath := filepath.Join(dir, "source.txt")
	targetPath := filepath.Join(dir, "target.txt")

	if err := os.WriteFile(sourcePath, []byte("hello\n"), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}
	writeSpec := `{"schemaVersion":"5","files":{"links":[{"source":"source.txt","target":"` + targetPath + `"}]}}`
	if err := os.WriteFile(specPath, []byte(writeSpec), 0o644); err != nil {
		t.Fatalf("write spec: %v", err)
	}

	code := run([]string{"apply", "--file", specPath, "--lock-file", lockPath, "--yes"})
	if code != exitOK {
		t.Fatalf("apply files: expected exitOK (%d), got %d", exitOK, code)
	}
	linkTarget, err := os.Readlink(targetPath)
	if err != nil {
		t.Fatalf("target should be a symlink: %v", err)
	}
	if linkTarget != sourcePath {
		t.Fatalf("link target: got %q, want %q", linkTarget, sourcePath)
	}
	lf, err := genvfile.ReadLock(lockPath)
	if err != nil {
		t.Fatalf("read lock: %v", err)
	}
	if len(lf.Files) != 1 {
		t.Fatalf("lock files: got %d entries, want 1", len(lf.Files))
	}
	if lf.Files[0].Source != "source.txt" || lf.Files[0].Target != targetPath || lf.Files[0].Mode != "link" {
		t.Fatalf("lock file entry = %#v", lf.Files[0])
	}
}

func TestApply_DryRunJSON_IncludesFilePlan(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "xdg"))
	specPath := filepath.Join(dir, "genv.json")
	lockPath := filepath.Join(dir, "genv.lock.json")
	if err := os.WriteFile(filepath.Join(dir, "source.txt"), []byte("hello\n"), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}
	targetPath := filepath.Join(dir, "target.txt")
	writeSpec := `{"schemaVersion":"5","files":{"links":[{"source":"source.txt","target":"` + targetPath + `"}]}}`
	if err := os.WriteFile(specPath, []byte(writeSpec), 0o644); err != nil {
		t.Fatalf("write spec: %v", err)
	}

	var code int
	out := captureStdout(t, func() {
		code = run([]string{"apply", "--file", specPath, "--lock-file", lockPath, "--dry-run", "--json"})
	})
	if code != exitOK {
		t.Fatalf("apply dry-run json: expected exitOK (%d), got %d\noutput: %s", exitOK, code, out)
	}
	var env map[string]interface{}
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("apply --json output is not valid JSON: %v\noutput: %q", err, out)
	}
	data, ok := env["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("JSON data field missing or wrong type: %v", env["data"])
	}
	files, ok := data["files"].([]interface{})
	if !ok || len(files) != 1 {
		t.Fatalf("data.files: got %#v, want one planned file", data["files"])
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
	_ = wp.Close()
	os.Stdout = old
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, rp); err != nil {
		t.Fatalf("io.Copy: %v", err)
	}
	_ = rp.Close()
	return buf.String()
}

func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stderr
	rp, wp, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stderr = wp
	fn()
	_ = wp.Close()
	os.Stderr = old
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, rp); err != nil {
		t.Fatalf("io.Copy: %v", err)
	}
	_ = rp.Close()
	return buf.String()
}

// ---- genv scan ---------------------------------------------------------------

func TestScanCmd_NoCrash(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	path := filepath.Join(dir, "genv.json")
	// scan must not crash regardless of what managers are available in CI.
	code := run([]string{"scan", "--file", path})
	if code != exitOK {
		t.Errorf("scan: expected exitOK (%d), got %d", exitOK, code)
	}
}

func TestScanCmd_FlagParseError(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	path := filepath.Join(dir, "genv.json")
	code := run([]string{"scan", "--file", path, "--no-such-flag"})
	if code != exitUsage {
		t.Errorf("unknown flag: expected exitUsage (%d), got %d", exitUsage, code)
	}
}

func TestScanCmd_InvalidFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
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
	t.Setenv("XDG_CONFIG_HOME", dir)
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
	t.Setenv("XDG_CONFIG_HOME", dir)
	path := filepath.Join(dir, "genv.json")
	code := run([]string{"scan", "--file", path, "--debug"})
	if code != exitOK {
		t.Errorf("scan --debug: expected exitOK (%d), got %d", exitOK, code)
	}
}

func TestScanAdapters_HomebrewPlatform(t *testing.T) {
	available := map[string]bool{"brew": true, "linuxbrew": true}

	tests := []struct {
		name    string
		goos    string
		include string
		exclude string
	}{
		{name: "Darwin scans Homebrew only", goos: "darwin", include: "brew", exclude: "linuxbrew"},
		{name: "Linux scans Linuxbrew only", goos: "linux", include: "linuxbrew", exclude: "brew"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			selected := scanAdaptersOnGOOS(available, tt.goos)
			names := make([]string, 0, len(selected))
			for _, a := range selected {
				names = append(names, a.Name())
			}

			if !slices.Contains(names, tt.include) {
				t.Errorf("selected managers %v do not include %q", names, tt.include)
			}
			if slices.Contains(names, tt.exclude) {
				t.Errorf("selected managers %v include non-native %q", names, tt.exclude)
			}
		})
	}
}

type scanCountingAdapter struct {
	name      string
	listCalls int
}

func (a *scanCountingAdapter) Name() string { return a.name }
func (a *scanCountingAdapter) Available() bool {
	return true
}

func (a *scanCountingAdapter) NormalizeID(id string, managers map[string]string) (string, bool) {
	return id, false
}
func (a *scanCountingAdapter) PlanInstall(pkgName string) []string   { return []string{"true"} }
func (a *scanCountingAdapter) PlanUninstall(pkgName string) []string { return []string{"true"} }
func (a *scanCountingAdapter) PlanUpgrade(pkgName string) []string   { return []string{"true"} }
func (a *scanCountingAdapter) PlanClean() [][]string                 { return nil }
func (a *scanCountingAdapter) Query(pkgName string) (bool, error)    { return true, nil }
func (a *scanCountingAdapter) ListInstalled() ([]string, error) {
	a.listCalls++
	return nil, nil
}
func (a *scanCountingAdapter) QueryVersion(pkgName string) (string, error) { return "", nil }

func TestScanCmd_DarwinDoesNotListLinuxbrew(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "genv.json")
	brew := &scanCountingAdapter{name: "brew"}
	linuxbrew := &scanCountingAdapter{name: "linuxbrew"}

	originalAll := adapter.All
	originalGOOS := scanGOOS
	adapter.All = []adapter.Adapter{brew, linuxbrew}
	scanGOOS = "darwin"
	t.Cleanup(func() {
		adapter.All = originalAll
		scanGOOS = originalGOOS
	})

	code := run([]string{"scan", "--file", path})

	if code != exitOK {
		t.Fatalf("scan: expected exitOK (%d), got %d", exitOK, code)
	}
	if brew.listCalls != 1 {
		t.Errorf("Homebrew ListInstalled calls = %d, want 1", brew.listCalls)
	}
	if linuxbrew.listCalls != 0 {
		t.Errorf("Linuxbrew ListInstalled calls = %d, want 0", linuxbrew.listCalls)
	}
}

type scanVersionListerAdapter struct {
	listCalls         int
	versionListCalls  int
	queryVersionCalls int
}

func (a *scanVersionListerAdapter) Name() string { return "batch" }
func (a *scanVersionListerAdapter) Available() bool {
	return true
}

func (a *scanVersionListerAdapter) NormalizeID(id string, managers map[string]string) (string, bool) {
	return id, false
}

func (a *scanVersionListerAdapter) PlanInstall(pkgName string) []string { return []string{"true"} }

func (a *scanVersionListerAdapter) PlanUninstall(pkgName string) []string { return []string{"true"} }

func (a *scanVersionListerAdapter) PlanUpgrade(pkgName string) []string { return []string{"true"} }
func (a *scanVersionListerAdapter) PlanClean() [][]string               { return nil }
func (a *scanVersionListerAdapter) Query(pkgName string) (bool, error)  { return true, nil }
func (a *scanVersionListerAdapter) ListInstalled() ([]string, error) {
	a.listCalls++
	return []string{"alpha", "beta"}, nil
}

func (a *scanVersionListerAdapter) QueryVersion(pkgName string) (string, error) {
	a.queryVersionCalls++
	return "fallback", nil
}

func (a *scanVersionListerAdapter) ListInstalledVersions() (map[string]string, error) {
	a.versionListCalls++
	return map[string]string{"beta": "2.0.0", "alpha": "1.0.0"}, nil
}

func TestScanCmd_UsesVersionListerWithoutPerPackageVersionQueries(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	path := filepath.Join(dir, "genv.json")

	originalAll := adapter.All
	batch := &scanVersionListerAdapter{}
	adapter.All = []adapter.Adapter{batch}
	t.Cleanup(func() { adapter.All = originalAll })

	code := run([]string{"scan", "--file", path})
	if code != exitOK {
		t.Fatalf("scan: expected exitOK (%d), got %d", exitOK, code)
	}
	if batch.versionListCalls != 1 {
		t.Fatalf("ListInstalledVersions calls = %d, want 1", batch.versionListCalls)
	}
	if batch.listCalls != 0 {
		t.Fatalf("ListInstalled calls = %d, want 0", batch.listCalls)
	}
	if batch.queryVersionCalls != 0 {
		t.Fatalf("QueryVersion calls = %d, want 0", batch.queryVersionCalls)
	}

	lockPath := lockPathForSpec(path, "")
	lf, err := genvfile.ReadLock(lockPath)
	if err != nil {
		t.Fatalf("ReadLock: %v", err)
	}
	versions := map[string]string{}
	for _, pkg := range lf.Packages {
		versions[pkg.ID] = pkg.InstalledVersion
	}
	want := map[string]string{"alpha": "1.0.0", "beta": "2.0.0"}
	if len(versions) != len(want) {
		t.Fatalf("versions = %#v, want %#v", versions, want)
	}
	for id, version := range want {
		if versions[id] != version {
			t.Fatalf("version for %s = %q, want %q (all versions %#v)", id, versions[id], version, versions)
		}
	}
}

// scanManagerNameAdapter mimics mas: ListInstalled reports the manager-specific
// name (a numeric App Store product ID) rather than the friendly genv ID.
type scanManagerNameAdapter struct {
	name      string
	installed []string
}

func (a *scanManagerNameAdapter) Name() string    { return a.name }
func (a *scanManagerNameAdapter) Available() bool { return true }
func (a *scanManagerNameAdapter) NormalizeID(id string, managers map[string]string) (string, bool) {
	return id, false
}
func (a *scanManagerNameAdapter) PlanInstall(pkgName string) []string   { return []string{"true"} }
func (a *scanManagerNameAdapter) PlanUninstall(pkgName string) []string { return []string{"true"} }
func (a *scanManagerNameAdapter) PlanUpgrade(pkgName string) []string   { return []string{"true"} }
func (a *scanManagerNameAdapter) PlanClean() [][]string                 { return nil }
func (a *scanManagerNameAdapter) Query(pkgName string) (bool, error)    { return true, nil }
func (a *scanManagerNameAdapter) ListInstalled() ([]string, error)      { return a.installed, nil }
func (a *scanManagerNameAdapter) QueryVersion(pkgName string) (string, error) {
	return "", nil
}

// TestScanCmd_SkipsPackageTrackedByManagerName guards against re-adopting a
// package whose friendly ID differs from its manager-specific name — the mas
// case where {"id":"xcode","managers":{"mas":"497799835"}} was previously
// duplicated as a second bare-numeric "497799835" entry on every scan.
func TestScanCmd_SkipsPackageTrackedByManagerName(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "genv.json")

	seed := &schema.GenvFile{
		SchemaVersion: schema.Version,
		Packages: []schema.Package{
			{ID: "xcode", Managers: map[string]string{"mas": "497799835"}},
		},
	}
	if err := genvfile.Write(path, seed); err != nil {
		t.Fatalf("seeding spec: %v", err)
	}

	mas := &scanManagerNameAdapter{name: "mas", installed: []string{"497799835"}}
	originalAll := adapter.All
	adapter.All = []adapter.Adapter{mas}
	t.Cleanup(func() { adapter.All = originalAll })

	code := run([]string{"scan", "--file", path})
	if code != exitOK {
		t.Fatalf("scan: expected exitOK (%d), got %d", exitOK, code)
	}

	f, _, err := genvfile.ReadOrNew(path)
	if err != nil {
		t.Fatalf("ReadOrNew: %v", err)
	}
	if len(f.Packages) != 1 {
		t.Fatalf("packages = %d, want 1 (no duplicate adopted): %+v", len(f.Packages), f.Packages)
	}
	for _, p := range f.Packages {
		if p.ID == "497799835" {
			t.Fatalf("bare-numeric duplicate %q was re-adopted", p.ID)
		}
	}
}

// ---- genv status -------------------------------------------------------------

func TestStatusCmd_FileNotFound(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	path := filepath.Join(dir, "genv.json")
	code := run([]string{"status", "--file", path})
	if code != exitIO {
		t.Errorf("missing spec: expected exitIO (%d), got %d", exitIO, code)
	}
}

func TestStatusCmd_NothingTracked(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
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
	t.Setenv("XDG_CONFIG_HOME", dir)
	path := filepath.Join(dir, "genv.json")
	lockPath := genvfile.LockPathFrom(path)

	run([]string{"add", "--file", path, "git"})
	writeLock(t, lockPath, []genvfile.LockedPackage{
		{ID: "git", Manager: "brew", PkgName: "git"},
	})

	code := run([]string{"status", "--file", path})
	if code != exitOK {
		t.Errorf("all ok: expected exitOK (%d), got %d", exitOK, code)
	}
}

func TestStatusCmd_MissingEntry(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
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
	t.Setenv("XDG_CONFIG_HOME", dir)
	path := filepath.Join(dir, "genv.json")
	lockPath := genvfile.LockPathFrom(path)

	// Empty spec but lock has git → "extra" exits with exitLogic.
	if err := os.WriteFile(path, []byte(`{"schemaVersion":"1","packages":[]}`), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	writeLock(t, lockPath, []genvfile.LockedPackage{
		{ID: "git", Manager: "brew", PkgName: "git"},
	})
	code := run([]string{"status", "--file", path})
	if code != exitLogic {
		t.Errorf("extra: expected exitLogic (%d), got %d", exitLogic, code)
	}
}

func TestStatusCmd_DriftEntry(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	path := filepath.Join(dir, "genv.json")
	lockPath := genvfile.LockPathFrom(path)

	run([]string{"add", "--file", path, "--version", "2.0.*", "git"})
	// Lock records version 1.x — does not satisfy "2.0.*" → drift.
	writeLock(t, lockPath, []genvfile.LockedPackage{
		{ID: "git", Manager: "brew", PkgName: "git", InstalledVersion: "1.9.0"},
	})
	code := run([]string{"status", "--file", path})
	if code != exitLogic {
		t.Errorf("drift: expected exitLogic (%d), got %d", exitLogic, code)
	}
}

func TestStatusCmd_InvalidFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
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
	t.Setenv("XDG_CONFIG_HOME", dir)
	path := filepath.Join(dir, "genv.json")
	code := run([]string{"status", "--file", path, "--no-such-flag"})
	if code != exitUsage {
		t.Errorf("unknown flag: expected exitUsage (%d), got %d", exitUsage, code)
	}
}

func TestStatusCmd_JsonOutput(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	path := filepath.Join(dir, "genv.json")
	lockPath := genvfile.LockPathFrom(path)

	run([]string{"add", "--file", path, "git"})
	writeLock(t, lockPath, []genvfile.LockedPackage{
		{ID: "git", Manager: "brew", PkgName: "git"},
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
	t.Setenv("XDG_CONFIG_HOME", dir)
	path := filepath.Join(dir, "genv.json")
	if err := os.WriteFile(path, []byte(`{"schemaVersion":"1","packages":[]}`), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	code := run([]string{"status", "--file", path, "--debug"})
	if code != exitOK {
		t.Errorf("status --debug: expected exitOK (%d), got %d", exitOK, code)
	}
}

func TestStatusFiles_DispatchesFilesOnlyMode(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "xdg"))
	specPath := filepath.Join(dir, "genv.json")
	lockPath := filepath.Join(dir, "genv.lock.json")
	if err := os.WriteFile(specPath, []byte(`{"schemaVersion":"5","files":{"dirs":[{"target":"`+filepath.Join(dir, "managed")+`"}]}}`), 0o644); err != nil {
		t.Fatalf("write spec: %v", err)
	}

	code := run([]string{"status", "--files", "--file", specPath, "--lock-file", lockPath})
	if code != exitLogic {
		t.Fatalf("status --files: expected exitLogic (%d) from current stub, got %d", exitLogic, code)
	}
}

func TestLockFileFlag_OverridesDefaultForList(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "xdg"))
	specPath := filepath.Join(dir, "genv.json")
	lockPath := filepath.Join(dir, "custom.lock.json")
	if err := os.WriteFile(specPath, []byte(`{"schemaVersion":"1","packages":[]}`), 0o644); err != nil {
		t.Fatalf("write spec: %v", err)
	}
	writeLock(t, lockPath, []genvfile.LockedPackage{{ID: "git", Manager: "brew", PkgName: "git"}})

	var code int
	out := captureStdout(t, func() {
		code = run([]string{"list", "--file", specPath, "--lock-file", lockPath})
	})
	if code != exitOK {
		t.Fatalf("list custom lock: expected exitOK (%d), got %d", exitOK, code)
	}
	if !strings.Contains(out, "git") {
		t.Fatalf("list custom lock output = %q, want tracked package", out)
	}
}

func TestUpgrade_RunsHooks(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "xdg"))
	specPath := filepath.Join(dir, "genv.json")
	lockPath := filepath.Join(dir, "genv.lock.json")
	marker := filepath.Join(dir, "hook.log")
	spec := `{"schemaVersion":"5","packages":[{"id":"git"}],"hooks":{"preUpgrade":[{"command":"printf pre >> ` + marker + `"}],"postUpgrade":[{"command":"printf post >> ` + marker + `"}]}}`
	if err := os.WriteFile(specPath, []byte(spec), 0o644); err != nil {
		t.Fatalf("write spec: %v", err)
	}
	writeLock(t, lockPath, []genvfile.LockedPackage{{ID: "git", Manager: "missing-manager", PkgName: "git"}})

	code := run([]string{"upgrade", "--file", specPath, "--lock-file", lockPath, "--yes"})
	if code != exitOK {
		t.Fatalf("upgrade hooks: expected exitOK (%d), got %d", exitOK, code)
	}
	got, err := os.ReadFile(marker)
	if err != nil {
		t.Fatalf("read hook marker: %v", err)
	}
	if string(got) != "prepost" {
		t.Fatalf("hook marker: got %q, want %q", string(got), "prepost")
	}
}

type upgradeNoHooksAdapter struct {
	marker string
}

func (a upgradeNoHooksAdapter) Name() string { return "test-upgrade-no-hooks" }
func (a upgradeNoHooksAdapter) Available() bool {
	return true
}

func (a upgradeNoHooksAdapter) NormalizeID(id string, managers map[string]string) (string, bool) {
	return id, false
}
func (a upgradeNoHooksAdapter) PlanInstall(pkgName string) []string { return []string{"true"} }
func (a upgradeNoHooksAdapter) PlanUninstall(pkgName string) []string {
	return []string{"true"}
}

func (a upgradeNoHooksAdapter) PlanUpgrade(pkgName string) []string {
	return []string{"sh", "-c", "printf upgrade >> " + a.marker}
}
func (a upgradeNoHooksAdapter) PlanClean() [][]string              { return nil }
func (a upgradeNoHooksAdapter) Query(pkgName string) (bool, error) { return true, nil }
func (a upgradeNoHooksAdapter) ListInstalled() ([]string, error) {
	return []string{pkgNameForTest}, nil
}

func (a upgradeNoHooksAdapter) QueryVersion(pkgName string) (string, error) {
	return "2.0.0", nil
}

const pkgNameForTest = "alpha"

type lifecycleHookAdapter struct {
	installMarker   string
	uninstallMarker string
	upgradeMarker   string
}

func (a lifecycleHookAdapter) Name() string    { return "test-hook-manager" }
func (a lifecycleHookAdapter) Available() bool { return true }
func (a lifecycleHookAdapter) NormalizeID(id string, managers map[string]string) (string, bool) {
	return id, false
}

func (a lifecycleHookAdapter) PlanInstall(pkgName string) []string {
	return []string{"sh", "-c", "printf install >> " + a.installMarker}
}

func (a lifecycleHookAdapter) PlanUninstall(pkgName string) []string {
	return []string{"sh", "-c", "printf uninstall >> " + a.uninstallMarker}
}

func (a lifecycleHookAdapter) PlanUpgrade(pkgName string) []string {
	return []string{"sh", "-c", "printf upgrade >> " + a.upgradeMarker}
}
func (a lifecycleHookAdapter) PlanClean() [][]string              { return nil }
func (a lifecycleHookAdapter) Query(pkgName string) (bool, error) { return true, nil }
func (a lifecycleHookAdapter) ListInstalled() ([]string, error)   { return []string{pkgNameForTest}, nil }

func (a lifecycleHookAdapter) QueryVersion(pkgName string) (string, error) {
	return "2.0.0", nil
}

func registerLifecycleHookAdapter(t *testing.T, a lifecycleHookAdapter) {
	t.Helper()
	originalAll := adapter.All
	originalKnown := schema.KnownManagers["test-hook-manager"]
	adapter.All = append([]adapter.Adapter{a}, originalAll...)
	schema.KnownManagers["test-hook-manager"] = true
	t.Cleanup(func() {
		adapter.All = originalAll
		if originalKnown {
			schema.KnownManagers["test-hook-manager"] = true
		} else {
			delete(schema.KnownManagers, "test-hook-manager")
		}
	})
}

func writeLockFile(t *testing.T, lockPath string, lf *genvfile.LockFile) {
	t.Helper()
	if err := genvfile.WriteLock(lockPath, lf); err != nil {
		t.Fatalf("write lock: %v", err)
	}
}

func TestApply_LifecycleHooks_receive_env_context(t *testing.T) {
	// Given
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "xdg"))
	specPath := filepath.Join(dir, "genv.json")
	lockPath := filepath.Join(dir, "genv.lock.json")
	hookLog := filepath.Join(dir, "hooks.log")
	installLog := filepath.Join(dir, "install.log")
	registerLifecycleHookAdapter(t, lifecycleHookAdapter{installMarker: installLog})
	spec := `{"schemaVersion":"6","packages":[{"id":"alpha","prefer":"test-hook-manager"}],"hooks":{"preApply":[{"command":"printf '%s:%s:%s:%s:%s\n' \"$GENV_EVENT\" \"$GENV_PHASE\" \"$GENV_HOST\" \"$GENV_INSTALLED\" \"$GENV_PROFILE\" >> ` + hookLog + `"}],"postApply":[{"command":"printf '%s:%s:%s:%s:%s\n' \"$GENV_EVENT\" \"$GENV_PHASE\" \"$GENV_HOST\" \"$GENV_INSTALLED\" \"$GENV_PROFILE\" >> ` + hookLog + `"}]}}`
	if err := os.WriteFile(specPath, []byte(spec), 0o644); err != nil {
		t.Fatalf("write spec: %v", err)
	}
	profileDir := filepath.Join(dir, "profiles")
	if err := os.MkdirAll(profileDir, 0o755); err != nil {
		t.Fatalf("create profile dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(profileDir, "work.json"), []byte(`{"schemaVersion":"6","packages":[]}`), 0o644); err != nil {
		t.Fatalf("write profile: %v", err)
	}
	writeLockFile(t, lockPath, &genvfile.LockFile{SchemaVersion: "1", ActiveProfile: "work", Packages: nil})

	// When
	code := run([]string{"apply", "--file", specPath, "--lock-file", lockPath, "--host", "ci", "--yes", "--hook-timeout", "1s"})

	// Then
	if code != exitOK {
		t.Fatalf("apply lifecycle hooks: expected exitOK (%d), got %d", exitOK, code)
	}
	got, err := os.ReadFile(hookLog)
	if err != nil {
		t.Fatalf("read hook log: %v", err)
	}
	want := "apply:pre-apply:ci:alpha:work\napply:post-apply:ci:alpha:work\n"
	if string(got) != want {
		t.Fatalf("hook log = %q, want %q", string(got), want)
	}
}

func TestAdd_NoHooks_skips_hooks_but_installs_package(t *testing.T) {
	// Given
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "xdg"))
	specPath := filepath.Join(dir, "genv.json")
	lockPath := filepath.Join(dir, "genv.lock.json")
	hookLog := filepath.Join(dir, "hook.log")
	installLog := filepath.Join(dir, "install.log")
	registerLifecycleHookAdapter(t, lifecycleHookAdapter{installMarker: installLog})
	spec := `{"schemaVersion":"6","packages":[],"hooks":{"preAdd":[{"command":"printf pre >> ` + hookLog + `; exit 99"}],"postAdd":[{"command":"printf post >> ` + hookLog + `; exit 99"}]}}`
	if err := os.WriteFile(specPath, []byte(spec), 0o644); err != nil {
		t.Fatalf("write spec: %v", err)
	}

	// When
	code := run([]string{"add", "--file", specPath, "--lock-file", lockPath, "--prefer", "test-hook-manager", "--no-search", "--no-hooks", "--hook-timeout", "1s", "alpha"})

	// Then
	if code != exitOK {
		t.Fatalf("add --no-hooks: expected exitOK (%d), got %d", exitOK, code)
	}
	if _, err := os.Stat(hookLog); !os.IsNotExist(err) {
		t.Fatalf("hook log stat = %v, want not exist", err)
	}
	got, err := os.ReadFile(installLog)
	if err != nil {
		t.Fatalf("read install log: %v", err)
	}
	if string(got) != "install" {
		t.Fatalf("install log = %q, want install", string(got))
	}
}

func TestRemove_LifecycleHooks_receive_removed_env(t *testing.T) {
	// Given
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "xdg"))
	specPath := filepath.Join(dir, "genv.json")
	lockPath := filepath.Join(dir, "genv.lock.json")
	hookLog := filepath.Join(dir, "hooks.log")
	uninstallLog := filepath.Join(dir, "uninstall.log")
	registerLifecycleHookAdapter(t, lifecycleHookAdapter{uninstallMarker: uninstallLog})
	spec := `{"schemaVersion":"6","packages":[{"id":"alpha","prefer":"test-hook-manager"}],"hooks":{"preRemove":[{"command":"printf '%s:%s:%s\n' \"$GENV_EVENT\" \"$GENV_PHASE\" \"$GENV_REMOVED\" >> ` + hookLog + `"}],"postRemove":[{"command":"printf '%s:%s:%s\n' \"$GENV_EVENT\" \"$GENV_PHASE\" \"$GENV_REMOVED\" >> ` + hookLog + `"}]}}`
	if err := os.WriteFile(specPath, []byte(spec), 0o644); err != nil {
		t.Fatalf("write spec: %v", err)
	}
	writeLock(t, lockPath, []genvfile.LockedPackage{{ID: "alpha", Manager: "test-hook-manager", PkgName: "alpha"}})

	// When
	code := run([]string{"remove", "--file", specPath, "--lock-file", lockPath, "--host", "ci", "--hook-timeout", "1s", "alpha"})

	// Then
	if code != exitOK {
		t.Fatalf("remove lifecycle hooks: expected exitOK (%d), got %d", exitOK, code)
	}
	got, err := os.ReadFile(hookLog)
	if err != nil {
		t.Fatalf("read hook log: %v", err)
	}
	want := "remove:pre-remove:alpha\nremove:post-remove:alpha\n"
	if string(got) != want {
		t.Fatalf("hook log = %q, want %q", string(got), want)
	}
}

func TestUpgrade_NoHooks_executes_package_plan_without_running_hooks(t *testing.T) {
	// Given: a tracked package with failing hooks and an adapter whose upgrade command records execution.
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "xdg"))
	specPath := filepath.Join(dir, "genv.json")
	lockPath := filepath.Join(dir, "genv.lock.json")
	upgradeMarker := filepath.Join(dir, "upgrade.log")
	hookMarker := filepath.Join(dir, "hook.log")
	spec := `{"schemaVersion":"5","packages":[{"id":"alpha"}],"hooks":{"preUpgrade":[{"command":"printf pre >> ` + hookMarker + `; exit 99"}],"postUpgrade":[{"command":"printf post >> ` + hookMarker + `; exit 99"}]}}`
	if err := os.WriteFile(specPath, []byte(spec), 0o644); err != nil {
		t.Fatalf("write spec: %v", err)
	}
	writeLock(t, lockPath, []genvfile.LockedPackage{{ID: "alpha", Manager: "test-upgrade-no-hooks", PkgName: pkgNameForTest, InstalledVersion: "1.0.0"}})
	originalAll := adapter.All
	adapter.All = append([]adapter.Adapter{upgradeNoHooksAdapter{marker: upgradeMarker}}, originalAll...)
	t.Cleanup(func() { adapter.All = originalAll })

	// When: hooks are explicitly skipped.
	code := run([]string{"upgrade", "--file", specPath, "--lock-file", lockPath, "--yes", "--no-hooks"})

	// Then: the package upgrade command ran and neither hook ran.
	if code != exitOK {
		t.Fatalf("upgrade --no-hooks: expected exitOK (%d), got %d", exitOK, code)
	}
	got, err := os.ReadFile(upgradeMarker)
	if err != nil {
		t.Fatalf("read upgrade marker: %v", err)
	}
	if string(got) != "upgrade" {
		t.Fatalf("upgrade marker = %q, want %q", string(got), "upgrade")
	}
	if _, err := os.Stat(hookMarker); !os.IsNotExist(err) {
		t.Fatalf("hook marker stat = %v, want not exist", err)
	}
}

func TestUpgrade_NoHooks_with_no_upgradeable_packages_does_not_run_hooks(t *testing.T) {
	// Given: a tracked package whose manager is unavailable, so upgrade has no executable plan.
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "xdg"))
	specPath := filepath.Join(dir, "genv.json")
	lockPath := filepath.Join(dir, "genv.lock.json")
	marker := filepath.Join(dir, "hook.log")
	spec := `{"schemaVersion":"5","packages":[{"id":"git"}],"hooks":{"preUpgrade":[{"command":"printf pre >> ` + marker + `; exit 99"}],"postUpgrade":[{"command":"printf post >> ` + marker + `; exit 99"}]}}`
	if err := os.WriteFile(specPath, []byte(spec), 0o644); err != nil {
		t.Fatalf("write spec: %v", err)
	}
	writeLock(t, lockPath, []genvfile.LockedPackage{{ID: "git", Manager: "missing-manager", PkgName: "git"}})

	// When: no-hooks is used on the no-upgradeable-packages branch.
	code := run([]string{"upgrade", "--file", specPath, "--lock-file", lockPath, "--yes", "--no-hooks"})

	// Then: the branch succeeds without executing either failing hook.
	if code != exitOK {
		t.Fatalf("upgrade --no-hooks with no plan: expected exitOK (%d), got %d", exitOK, code)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("hook marker stat = %v, want not exist", err)
	}
}

func TestUpgrade_NoHooks_JSON_reports_hooks_skipped(t *testing.T) {
	// Given: a no-plan upgrade with hooks configured.
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "xdg"))
	specPath := filepath.Join(dir, "genv.json")
	lockPath := filepath.Join(dir, "genv.lock.json")
	if err := os.WriteFile(specPath, []byte(`{"schemaVersion":"5","packages":[{"id":"git"}],"hooks":{"preUpgrade":[{"command":"exit 99"}]}}`), 0o644); err != nil {
		t.Fatalf("write spec: %v", err)
	}
	writeLock(t, lockPath, []genvfile.LockedPackage{{ID: "git", Manager: "missing-manager", PkgName: "git"}})

	// When: JSON output is requested with hooks skipped.
	var code int
	out := captureStdout(t, func() {
		code = run([]string{"upgrade", "--file", specPath, "--lock-file", lockPath, "--json", "--no-hooks"})
	})

	// Then: the JSON filter context records that hooks were intentionally skipped.
	if code != exitOK {
		t.Fatalf("upgrade --json --no-hooks: expected exitOK (%d), got %d\noutput: %s", exitOK, code, out)
	}
	var env map[string]any
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("upgrade --json output is not valid JSON: %v\noutput: %q", err, out)
	}
	data, ok := env["data"].(map[string]any)
	if !ok {
		t.Fatalf("JSON data field missing or wrong type: %v", env["data"])
	}
	filters, ok := data["filters"].(map[string]any)
	if !ok {
		t.Fatalf("JSON filters field missing or wrong type: %v", data["filters"])
	}
	if got := filters["hooksSkipped"]; got != true {
		t.Fatalf("filters.hooksSkipped = %v, want true", got)
	}
}

func TestUpgrade_DryRun_does_not_run_hooks_or_write_lock(t *testing.T) {
	// Given: a tracked package with upgrade hooks and an existing lock version.
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "xdg"))
	specPath := filepath.Join(dir, "genv.json")
	lockPath := filepath.Join(dir, "genv.lock.json")
	marker := filepath.Join(dir, "hook.log")
	spec := `{"schemaVersion":"5","packages":[{"id":"git"}],"hooks":{"preUpgrade":[{"command":"printf pre >> ` + marker + `"}],"postUpgrade":[{"command":"printf post >> ` + marker + `"}]}}`
	if err := os.WriteFile(specPath, []byte(spec), 0o644); err != nil {
		t.Fatalf("write spec: %v", err)
	}
	writeLock(t, lockPath, []genvfile.LockedPackage{{ID: "git", Manager: "brew", PkgName: "git", InstalledVersion: "1.0.0"}})

	// When: upgrade is planned in dry-run mode.
	code := run([]string{"upgrade", "--file", specPath, "--lock-file", lockPath, "--dry-run"})

	// Then: no hook marker appears and the lock version remains untouched.
	if code != exitOK {
		t.Fatalf("upgrade dry-run: expected exitOK (%d), got %d", exitOK, code)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("hook marker stat = %v, want not exist", err)
	}
	lf, err := genvfile.ReadLock(lockPath)
	if err != nil {
		t.Fatalf("read lock: %v", err)
	}
	if got := lf.Packages[0].InstalledVersion; got != "1.0.0" {
		t.Fatalf("lock version = %q, want %q", got, "1.0.0")
	}
}

func TestUpgrade_HookTimeout_bad_duration_fails_usage(t *testing.T) {
	// Given: a valid spec and lock so flag parsing reaches hook-timeout validation.
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "xdg"))
	specPath := filepath.Join(dir, "genv.json")
	lockPath := filepath.Join(dir, "genv.lock.json")
	if err := os.WriteFile(specPath, []byte(`{"schemaVersion":"5","packages":[{"id":"git"}]}`), 0o644); err != nil {
		t.Fatalf("write spec: %v", err)
	}
	writeLock(t, lockPath, []genvfile.LockedPackage{{ID: "git", Manager: "brew", PkgName: "git"}})

	// When: the duration cannot be parsed.
	code := run([]string{"upgrade", "--file", specPath, "--lock-file", lockPath, "--hook-timeout", "nope"})

	// Then: CLI treats it as usage rather than silently disabling the timeout.
	if code != exitUsage {
		t.Fatalf("upgrade bad --hook-timeout: expected exitUsage (%d), got %d", exitUsage, code)
	}
}

func TestUpgrade_HookTimeout_times_out_hung_hook(t *testing.T) {
	// Given: a no-plan upgrade still runs hooks, and the pre hook blocks longer than the timeout.
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "xdg"))
	specPath := filepath.Join(dir, "genv.json")
	lockPath := filepath.Join(dir, "genv.lock.json")
	spec := `{"schemaVersion":"5","packages":[{"id":"git"}],"hooks":{"preUpgrade":[{"command":"sleep 1"}]}}`
	if err := os.WriteFile(specPath, []byte(spec), 0o644); err != nil {
		t.Fatalf("write spec: %v", err)
	}
	writeLock(t, lockPath, []genvfile.LockedPackage{{ID: "git", Manager: "missing-manager", PkgName: "git"}})

	// When: hook timeout is shorter than the hook command.
	code := run([]string{"upgrade", "--file", specPath, "--lock-file", lockPath, "--yes", "--hook-timeout", "1ms"})

	// Then: the hook timeout is surfaced as a lifecycle failure.
	if code != exitLogic {
		t.Fatalf("upgrade timed-out hook: expected exitLogic (%d), got %d", exitLogic, code)
	}
}

func TestUpdatesCheck_plans_batches_without_mutating_lock_or_running_commands(t *testing.T) {
	// Given: a tracked package with an upgrade command that would create a marker if executed.
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "xdg"))
	specPath := filepath.Join(dir, "genv.json")
	lockPath := filepath.Join(dir, "genv.lock.json")
	upgradeMarker := filepath.Join(dir, "upgrade.log")
	if err := os.WriteFile(specPath, []byte(`{"schemaVersion":"5","packages":[{"id":"alpha"}]}`), 0o644); err != nil {
		t.Fatalf("write spec: %v", err)
	}
	writeLock(t, lockPath, []genvfile.LockedPackage{{ID: "alpha", Manager: "test-upgrade-no-hooks", PkgName: pkgNameForTest, InstalledVersion: "1.0.0"}})
	before, err := os.ReadFile(lockPath)
	if err != nil {
		t.Fatalf("read lock before: %v", err)
	}
	infoBefore, err := os.Stat(lockPath)
	if err != nil {
		t.Fatalf("stat lock before: %v", err)
	}
	originalAll := adapter.All
	adapter.All = append([]adapter.Adapter{upgradeNoHooksAdapter{marker: upgradeMarker}}, originalAll...)
	t.Cleanup(func() { adapter.All = originalAll })

	// When: updates check is run through the public CLI surface.
	var code int
	out := captureStdout(t, func() {
		code = run([]string{"updates", "check", "--file", specPath, "--lock-file", lockPath})
	})

	// Then: the batch is reported, but no command ran and the lock file is byte-identical.
	if code != exitOK {
		t.Fatalf("updates check: expected exitOK (%d), got %d\noutput: %s", exitOK, code, out)
	}
	if !strings.Contains(out, "genv-tracked packages only") || !strings.Contains(out, "alpha") || !strings.Contains(out, "test-upgrade-no-hooks") {
		t.Fatalf("updates check output = %q, want tracked-only plan with alpha batch", out)
	}
	after, err := os.ReadFile(lockPath)
	if err != nil {
		t.Fatalf("read lock after: %v", err)
	}
	if !bytes.Equal(after, before) {
		t.Fatalf("lock content changed\nbefore: %s\nafter: %s", before, after)
	}
	infoAfter, err := os.Stat(lockPath)
	if err != nil {
		t.Fatalf("stat lock after: %v", err)
	}
	if !infoAfter.ModTime().Equal(infoBefore.ModTime()) {
		t.Fatalf("lock mtime changed: before %s after %s", infoBefore.ModTime(), infoAfter.ModTime())
	}
	if _, err := os.Stat(upgradeMarker); !os.IsNotExist(err) {
		t.Fatalf("upgrade marker stat = %v, want not exist", err)
	}
}

func TestUpdatesCheck_JSON_is_deterministic_dry_run_success(t *testing.T) {
	// Given: a lock entry whose manager can produce a dry-run upgrade plan.
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "xdg"))
	specPath := filepath.Join(dir, "genv.json")
	lockPath := filepath.Join(dir, "genv.lock.json")
	if err := os.WriteFile(specPath, []byte(`{"schemaVersion":"5","packages":[{"id":"alpha"}]}`), 0o644); err != nil {
		t.Fatalf("write spec: %v", err)
	}
	writeLock(t, lockPath, []genvfile.LockedPackage{{ID: "alpha", Manager: "test-upgrade-no-hooks", PkgName: pkgNameForTest, InstalledVersion: "1.0.0"}})
	originalAll := adapter.All
	adapter.All = append([]adapter.Adapter{upgradeNoHooksAdapter{marker: filepath.Join(dir, "upgrade.log")}}, originalAll...)
	t.Cleanup(func() { adapter.All = originalAll })

	// When: machine-readable updates check output is requested twice.
	var firstCode int
	first := captureStdout(t, func() {
		firstCode = run([]string{"updates", "check", "--file", specPath, "--lock-file", lockPath, "--json"})
	})
	var secondCode int
	second := captureStdout(t, func() {
		secondCode = run([]string{"updates", "check", "--file", specPath, "--lock-file", lockPath, "--json"})
	})

	// Then: both runs produce the same ok:true dry-run envelope.
	if firstCode != exitOK || secondCode != exitOK {
		t.Fatalf("updates check --json codes = %d/%d, want %d\nfirst: %s\nsecond: %s", firstCode, secondCode, exitOK, first, second)
	}
	if first != second {
		t.Fatalf("updates check --json not deterministic\nfirst: %s\nsecond: %s", first, second)
	}
	var env struct {
		Command string `json:"command"`
		OK      bool   `json:"ok"`
		Data    struct {
			DryRun  bool `json:"dryRun"`
			Batches []struct {
				Manager string   `json:"manager"`
				IDs     []string `json:"ids"`
				Status  string   `json:"status"`
			} `json:"batches"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(first), &env); err != nil {
		t.Fatalf("updates check --json output is not valid JSON: %v\noutput: %q", err, first)
	}
	if env.Command != "updates check" || !env.OK || !env.Data.DryRun {
		t.Fatalf("envelope = %+v, want command updates check ok true dryRun true", env)
	}
	if len(env.Data.Batches) != 1 || env.Data.Batches[0].Manager != "test-upgrade-no-hooks" || !slices.Equal(env.Data.Batches[0].IDs, []string{"alpha"}) || env.Data.Batches[0].Status != "planned" {
		t.Fatalf("batches = %+v, want one planned alpha batch", env.Data.Batches)
	}
}

func TestUpdatesCheck_missing_spec_or_lock_returns_actionable_error_without_mutation(t *testing.T) {
	tests := []struct {
		name      string
		writeSpec bool
		writeLock bool
		want      string
	}{
		{name: "missing spec", writeLock: true, want: "run 'genv init'"},
		{name: "missing lock", writeSpec: true, want: "reading lock"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given: either the spec or lock is absent.
			dir := t.TempDir()
			t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "xdg"))
			specPath := filepath.Join(dir, "genv.json")
			lockPath := filepath.Join(dir, "genv.lock.json")
			if tt.writeSpec {
				if err := os.WriteFile(specPath, []byte(`{"schemaVersion":"5","packages":[{"id":"alpha"}]}`), 0o644); err != nil {
					t.Fatalf("write spec: %v", err)
				}
			}
			if tt.writeLock {
				writeLock(t, lockPath, []genvfile.LockedPackage{{ID: "alpha", Manager: "brew", PkgName: "alpha"}})
			}

			// When: updates check is invoked.
			var code int
			errOut := captureStderr(t, func() {
				code = run([]string{"updates", "check", "--file", specPath, "--lock-file", lockPath})
			})

			// Then: the error is actionable and no missing file is created as a side effect.
			if code != exitIO {
				t.Fatalf("updates check missing file: expected exitIO (%d), got %d\nstderr: %s", exitIO, code, errOut)
			}
			if !strings.Contains(errOut, tt.want) {
				t.Fatalf("stderr = %q, want to contain %q", errOut, tt.want)
			}
			if !tt.writeSpec {
				if _, err := os.Stat(specPath); !os.IsNotExist(err) {
					t.Fatalf("spec stat = %v, want not exist", err)
				}
			}
			if !tt.writeLock {
				if _, err := os.Stat(lockPath); !os.IsNotExist(err) {
					t.Fatalf("lock stat = %v, want not exist", err)
				}
			}
		})
	}
}

func TestUpdatesCheck_JSON_missing_spec_or_lock_returns_error_envelope(t *testing.T) {
	tests := []struct {
		name      string
		writeSpec bool
		writeLock bool
		wantCode  int
	}{
		{name: "missing spec", writeLock: true, wantCode: exitLogic},
		{name: "missing lock", writeSpec: true, wantCode: exitIO},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given: either the spec or lock is absent.
			dir := t.TempDir()
			t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "xdg"))
			specPath := filepath.Join(dir, "genv.json")
			lockPath := filepath.Join(dir, "genv.lock.json")
			if tt.writeSpec {
				if err := os.WriteFile(specPath, []byte(`{"schemaVersion":"5","packages":[{"id":"alpha"}]}`), 0o644); err != nil {
					t.Fatalf("write spec: %v", err)
				}
			}
			if tt.writeLock {
				writeLock(t, lockPath, []genvfile.LockedPackage{{ID: "alpha", Manager: "brew", PkgName: "alpha"}})
			}

			// When: JSON output is requested.
			var code int
			out := captureStdout(t, func() {
				code = run([]string{"updates", "check", "--file", specPath, "--lock-file", lockPath, "--json"})
			})

			// Then: stdout is a parseable ok:false envelope and no missing file is created.
			if code != tt.wantCode {
				t.Fatalf("updates check --json missing file: expected %d, got %d\nstdout: %s", tt.wantCode, code, out)
			}
			var env struct {
				Command string   `json:"command"`
				OK      bool     `json:"ok"`
				Errors  []string `json:"errors"`
			}
			if err := json.Unmarshal([]byte(out), &env); err != nil {
				t.Fatalf("updates check --json error output is not valid JSON: %v\noutput: %q", err, out)
			}
			if env.Command != "updates check" || env.OK || len(env.Errors) == 0 {
				t.Fatalf("envelope = %+v, want command updates check ok false with errors", env)
			}
			if !tt.writeSpec {
				if _, err := os.Stat(specPath); !os.IsNotExist(err) {
					t.Fatalf("spec stat = %v, want not exist", err)
				}
			}
			if !tt.writeLock {
				if _, err := os.Stat(lockPath); !os.IsNotExist(err) {
					t.Fatalf("lock stat = %v, want not exist", err)
				}
			}
		})
	}
}

func TestUpdatesCheck_does_not_run_hooks(t *testing.T) {
	// Given: a spec with failing upgrade hooks that would create a marker if executed.
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "xdg"))
	specPath := filepath.Join(dir, "genv.json")
	lockPath := filepath.Join(dir, "genv.lock.json")
	hookMarker := filepath.Join(dir, "hook.log")
	spec := `{"schemaVersion":"5","packages":[{"id":"alpha"}],"hooks":{"preUpgrade":[{"command":"printf pre >> ` + hookMarker + `; exit 99"}],"postUpgrade":[{"command":"printf post >> ` + hookMarker + `; exit 99"}]}}`
	if err := os.WriteFile(specPath, []byte(spec), 0o644); err != nil {
		t.Fatalf("write spec: %v", err)
	}
	writeLock(t, lockPath, []genvfile.LockedPackage{{ID: "alpha", Manager: "missing-manager", PkgName: "alpha"}})

	// When: updates check runs with no executable package plan.
	code := run([]string{"updates", "check", "--file", specPath, "--lock-file", lockPath})

	// Then: failing hooks are ignored because check only plans.
	if code != exitOK {
		t.Fatalf("updates check with failing hooks: expected exitOK (%d), got %d", exitOK, code)
	}
	if _, err := os.Stat(hookMarker); !os.IsNotExist(err) {
		t.Fatalf("hook marker stat = %v, want not exist", err)
	}
}

func TestUpdates_UsageErrors_are_actionable(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "missing subcommand", args: []string{"updates"}, want: "usage: genv updates <check|start|stop|status>"},
		{name: "unknown subcommand", args: []string{"updates", "frobnicate"}, want: "unknown subcommand"},
		{name: "bad flag", args: []string{"updates", "check", "--definitely-bad"}, want: "flag provided but not defined"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// When: malformed updates invocations are routed through the CLI.
			var code int
			errOut := captureStderr(t, func() { code = run(tt.args) })

			// Then: they fail as usage errors with corrective text.
			if code != exitUsage {
				t.Fatalf("run(%v): expected exitUsage (%d), got %d", tt.args, exitUsage, code)
			}
			if !strings.Contains(errOut, tt.want) {
				t.Fatalf("stderr = %q, want to contain %q", errOut, tt.want)
			}
		})
	}
}

type fakeUpdatesSupervisor struct {
	supported bool
	status    service.ScheduledJobStatus
	statusErr error
	started   []service.ScheduledJob
	stopped   []string
}

func (f *fakeUpdatesSupervisor) Supported() bool { return f.supported }

func (f *fakeUpdatesSupervisor) Start(ctx context.Context, job service.ScheduledJob) error {
	f.started = append(f.started, job)
	return nil
}

func (f *fakeUpdatesSupervisor) Stop(ctx context.Context, name string) error {
	f.stopped = append(f.stopped, name)
	return nil
}

func (f *fakeUpdatesSupervisor) Status(ctx context.Context, name string) (service.ScheduledJobStatus, error) {
	status := f.status
	status.Supported = f.supported
	if status.Detail == "" {
		status.Detail = name
	}
	return status, f.statusErr
}

func withUpdatesSupervisor(t *testing.T, supervisor *fakeUpdatesSupervisor) {
	t.Helper()
	original := newUpdatesSupervisor
	newUpdatesSupervisor = func() updatesSupervisor { return supervisor }
	t.Cleanup(func() { newUpdatesSupervisor = original })
}

func TestUpdatesStart_rejects_missing_disabled_or_invalid_config(t *testing.T) {
	tests := []struct {
		name string
		spec string
		want string
	}{
		{name: "missing updates block", spec: `{"schemaVersion":"6","packages":[]}`, want: "updates block is missing"},
		{name: "disabled updates block", spec: `{"schemaVersion":"6","packages":[],"updates":{"enabled":false,"interval":"24h"}}`, want: "updates.enabled is false"},
		{name: "invalid interval", spec: `{"schemaVersion":"6","packages":[],"updates":{"enabled":true,"interval":"0s"}}`, want: "updates.interval"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given: a spec whose updates config cannot start a scheduler.
			dir := t.TempDir()
			t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "xdg"))
			specPath := filepath.Join(dir, "genv.json")
			if err := os.WriteFile(specPath, []byte(tt.spec), 0o644); err != nil {
				t.Fatalf("write spec: %v", err)
			}
			supervisor := &fakeUpdatesSupervisor{supported: true}
			withUpdatesSupervisor(t, supervisor)

			// When: updates start is invoked.
			var code int
			errOut := captureStderr(t, func() { code = run([]string{"updates", "start", "--file", specPath}) })

			// Then: it fails with a corrective hint and never creates a unit.
			if code != exitValidation {
				t.Fatalf("updates start: expected exitValidation (%d), got %d\nstderr: %s", exitValidation, code, errOut)
			}
			if !strings.Contains(errOut, tt.want) || !strings.Contains(errOut, "enabled updates block") {
				t.Fatalf("stderr = %q, want %q and corrective hint", errOut, tt.want)
			}
			if len(supervisor.started) != 0 {
				t.Fatalf("started jobs = %#v, want none", supervisor.started)
			}
		})
	}
}

func TestUpdatesStart_valid_config_registers_scheduler_without_auto_apply(t *testing.T) {
	// Given: a valid check-only updates config and a fake scheduler backend.
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "xdg"))
	specPath := filepath.Join(dir, "genv.json")
	lockPath := filepath.Join(dir, "genv.lock.json")
	if err := os.WriteFile(specPath, []byte(`{"schemaVersion":"6","packages":[],"updates":{"enabled":true,"interval":"2h","notify":true}}`), 0o644); err != nil {
		t.Fatalf("write spec: %v", err)
	}
	supervisor := &fakeUpdatesSupervisor{supported: true}
	withUpdatesSupervisor(t, supervisor)
	originalExecutable := updatesExecutable
	updatesExecutable = func() (string, error) { return "/tmp/genv-test-bin", nil }
	t.Cleanup(func() { updatesExecutable = originalExecutable })
	invokingPath := "/custom/bin:relative:/usr/bin:/custom/bin"
	t.Setenv("PATH", invokingPath)

	// When: updates start is invoked.
	var code int
	out := captureStdout(t, func() {
		code = run([]string{"updates", "start", "--file", specPath, "--lock-file", lockPath, "--host", "qa-host"})
	})

	// Then: one scheduled job is registered, its command is check-only by default, and start returns immediately.
	if code != exitOK {
		t.Fatalf("updates start: expected exitOK (%d), got %d\nstdout: %s", exitOK, code, out)
	}
	if len(supervisor.started) != 1 {
		t.Fatalf("started jobs = %#v, want one", supervisor.started)
	}
	job := supervisor.started[0]
	if job.Name != updatesServiceName || job.Interval != 2*time.Hour {
		t.Fatalf("job = %#v, want updates interval 2h", job)
	}
	wantCommand := []string{"/tmp/genv-test-bin", "updates", "__run-once", "--file", specPath, "--lock-file", lockPath, "--host", "qa-host"}
	if !slices.Equal(job.Command, wantCommand) {
		t.Fatalf("job command = %v, want %v", job.Command, wantCommand)
	}
	wantPath := service.ScheduledPath(invokingPath, runtime.GOOS)
	if got := job.Environment["PATH"]; got != wantPath {
		t.Fatalf("job PATH = %q, want sanitized augmented PATH %q", got, wantPath)
	}
	if !strings.Contains(out, "check/log/notify only") || strings.Contains(out, "auto-apply enabled") {
		t.Fatalf("stdout = %q, want explicit default check-only mode", out)
	}
}

func TestUpdatesStatusAndStop_unsupported_platform_do_not_crash(t *testing.T) {
	// Given: a fake backend that reports no systemd or launchd support.
	supervisor := &fakeUpdatesSupervisor{supported: false}
	withUpdatesSupervisor(t, supervisor)

	// When: status and stop are invoked.
	var statusCode int
	statusOut := captureStdout(t, func() { statusCode = run([]string{"updates", "status"}) })
	var stopCode int
	stopOut := captureStdout(t, func() { stopCode = run([]string{"updates", "stop"}) })

	// Then: both return cleanly with clear state and no stop call is required.
	if statusCode != exitOK || stopCode != exitOK {
		t.Fatalf("status/stop codes = %d/%d, want %d", statusCode, stopCode, exitOK)
	}
	if !strings.Contains(statusOut, "not supported") || !strings.Contains(statusOut, "not running") {
		t.Fatalf("status output = %q, want unsupported not running", statusOut)
	}
	if !strings.Contains(stopOut, "not supported") || !strings.Contains(stopOut, "nothing to stop") {
		t.Fatalf("stop output = %q, want unsupported nothing to stop", stopOut)
	}
}

func TestUpdatesStatus_prints_typed_scheduler_state(t *testing.T) {
	exitZero := 0
	exitSeven := 7
	tests := []struct {
		name     string
		status   service.ScheduledJobStatus
		want     []string
		dontWant []string
	}{
		{name: "not registered", status: service.ScheduledJobStatus{Supported: true}, want: []string{"not registered"}},
		{name: "registered and executing", status: service.ScheduledJobStatus{Supported: true, Registered: true, Executing: true}, want: []string{"registered and executing"}, dontWant: []string{"idle", "last run", "no completed run"}},
		{name: "registered idle no known run", status: service.ScheduledJobStatus{Supported: true, Registered: true, LastRun: service.ScheduledRunUnknown}, want: []string{"registered and idle", "no completed run is known"}},
		{name: "registered idle last run succeeded", status: service.ScheduledJobStatus{Supported: true, Registered: true, LastRun: service.ScheduledRunSuccess, ExitCode: &exitZero}, want: []string{"registered and idle", "last run succeeded", "status 0"}},
		{name: "registered idle last run failed", status: service.ScheduledJobStatus{Supported: true, Registered: true, LastRun: service.ScheduledRunFailure, ExitCode: &exitSeven, LastRunDetail: "exit-code"}, want: []string{"registered and idle", "last run failed", "status 7", "exit-code"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			supervisor := &fakeUpdatesSupervisor{supported: true, status: tt.status}
			withUpdatesSupervisor(t, supervisor)

			var code int
			out := captureStdout(t, func() { code = run([]string{"updates", "status"}) })

			if code != exitOK {
				t.Fatalf("updates status code = %d, want %d", code, exitOK)
			}
			for _, want := range tt.want {
				if !strings.Contains(out, want) {
					t.Fatalf("stdout = %q, want %q", out, want)
				}
			}
			for _, dontWant := range tt.dontWant {
				if strings.Contains(out, dontWant) {
					t.Fatalf("stdout = %q, do not want contradictory %q wording", out, dontWant)
				}
			}
		})
	}
}

func TestUpdatesStatus_returns_io_error_when_inspection_fails(t *testing.T) {
	supervisor := &fakeUpdatesSupervisor{supported: true, statusErr: errors.New("supervisor unavailable")}
	withUpdatesSupervisor(t, supervisor)

	var code int
	errOut := captureStderr(t, func() { code = run([]string{"updates", "status"}) })

	if code != exitIO {
		t.Fatalf("updates status code = %d, want %d", code, exitIO)
	}
	if !strings.Contains(errOut, "supervisor unavailable") {
		t.Fatalf("stderr = %q, want inspection error", errOut)
	}
}

func TestUpdatesRunOnce_auto_apply_only_when_explicit(t *testing.T) {
	tests := []struct {
		name      string
		autoApply bool
		wantRuns  int
	}{
		{name: "default check only", autoApply: false, wantRuns: 0},
		{name: "explicit auto apply", autoApply: true, wantRuns: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given: a valid updates config and a tracked package with a fake adapter plan.
			dir := t.TempDir()
			t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "xdg"))
			specPath := filepath.Join(dir, "genv.json")
			lockPath := filepath.Join(dir, "genv.lock.json")
			spec := fmt.Sprintf(`{"schemaVersion":"6","packages":[{"id":"alpha"}],"updates":{"enabled":true,"interval":"1h","autoApply":%t}}`, tt.autoApply)
			if err := os.WriteFile(specPath, []byte(spec), 0o644); err != nil {
				t.Fatalf("write spec: %v", err)
			}
			writeLock(t, lockPath, []genvfile.LockedPackage{{ID: "alpha", Manager: "test-upgrade-no-hooks", PkgName: pkgNameForTest, InstalledVersion: "1.0.0"}})
			originalAll := adapter.All
			adapter.All = append([]adapter.Adapter{upgradeNoHooksAdapter{marker: filepath.Join(dir, "upgrade.log")}}, originalAll...)
			t.Cleanup(func() { adapter.All = originalAll })
			originalRun := updatesRunUpgrade
			runCalls := 0
			updatesRunUpgrade = func(ctx context.Context, opts upgrade.UpgradeRunOptions) upgrade.UpgradeRunResult {
				runCalls++
				return upgrade.UpgradeRunResult{Plan: opts.Plan}
			}
			t.Cleanup(func() { updatesRunUpgrade = originalRun })

			// When: the hidden one-shot worker runs.
			code := run([]string{"updates", "__run-once", "--file", specPath, "--lock-file", lockPath})

			// Then: the shared executor is called only for explicit autoApply:true.
			if code != exitOK {
				t.Fatalf("updates __run-once: expected exitOK (%d), got %d", exitOK, code)
			}
			if runCalls != tt.wantRuns {
				t.Fatalf("RunUpgrade calls = %d, want %d", runCalls, tt.wantRuns)
			}
		})
	}
}

func TestUpdatesRunOnce_CheckOnly_notifies_only_when_packages_outdated(t *testing.T) {
	tests := []struct {
		name       string
		plan       upgrade.UpgradePlan
		wantNotify bool
	}{
		{
			name:       "nothing outdated stays silent",
			plan:       upgrade.UpgradePlan{},
			wantNotify: false,
		},
		{
			name: "outdated packages notify",
			plan: upgrade.UpgradePlan{Actions: []resolver.UpgradeAction{
				{LPs: []genvfile.LockedPackage{{ID: "alpha"}, {ID: "beta"}}},
			}},
			wantNotify: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given: a notify-enabled, check-only updates config.
			dir := t.TempDir()
			t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "xdg"))
			specPath := filepath.Join(dir, "genv.json")
			lockPath := filepath.Join(dir, "genv.lock.json")
			if err := os.WriteFile(specPath, []byte(`{"schemaVersion":"6","packages":[{"id":"alpha"}],"updates":{"enabled":true,"interval":"1h","autoApply":false,"notify":true}}`), 0o644); err != nil {
				t.Fatalf("write spec: %v", err)
			}
			writeLock(t, lockPath, nil)

			originalBuild := updatesBuildPlan
			updatesBuildPlan = func(opts upgrade.UpgradeOptions) (upgrade.UpgradePlan, error) {
				if opts.Filters.All {
					t.Error("check worker built plan with Filters.All (should filter outdated)")
				}
				return tt.plan, nil
			}
			t.Cleanup(func() { updatesBuildPlan = originalBuild })

			// updatesLookPath is consulted only when a notification is actually
			// sent; make it fail so no real notifier runs, and count the calls.
			lookCalls := 0
			originalLookPath := updatesLookPath
			updatesLookPath = func(string) (string, error) {
				lookCalls++
				return "", os.ErrNotExist
			}
			t.Cleanup(func() { updatesLookPath = originalLookPath })

			// When: the hidden one-shot checker runs.
			code := run([]string{"updates", "__run-once", "--file", specPath, "--lock-file", lockPath})
			if code != exitOK {
				t.Fatalf("updates __run-once code = %d, want %d", code, exitOK)
			}

			// Then: a notification is attempted only when packages are outdated.
			if gotNotify := lookCalls > 0; gotNotify != tt.wantNotify {
				t.Fatalf("notification attempted = %v (lookCalls=%d), want %v", gotNotify, lookCalls, tt.wantNotify)
			}
		})
	}
}

func TestUpdatesLogger_returns_error_for_unusable_log_path(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T, xdg string)
	}{
		{
			name: "directory creation failure",
			setup: func(t *testing.T, xdg string) {
				t.Helper()
				if err := os.WriteFile(xdg, []byte("not a directory"), 0o600); err != nil {
					t.Fatalf("write XDG_CONFIG_HOME file: %v", err)
				}
			},
		},
		{
			name: "log open failure",
			setup: func(t *testing.T, xdg string) {
				t.Helper()
				if err := os.MkdirAll(filepath.Join(xdg, "genv", "updates.log"), 0o700); err != nil {
					t.Fatalf("mkdir updates.log directory: %v", err)
				}
			},
		},
		{
			name: "log rotation failure",
			setup: func(t *testing.T, xdg string) {
				t.Helper()
				dir := filepath.Join(xdg, "genv")
				if err := os.MkdirAll(filepath.Join(dir, "updates.log.1", "occupied"), 0o700); err != nil {
					t.Fatalf("mkdir rotation target: %v", err)
				}
				if err := os.WriteFile(filepath.Join(dir, "updates.log"), make([]byte, (1<<20)+1), 0o600); err != nil {
					t.Fatalf("write oversized updates.log: %v", err)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given: the audit log path cannot be prepared for the named reason.
			xdg := filepath.Join(t.TempDir(), "xdg")
			tt.setup(t, xdg)
			t.Setenv("XDG_CONFIG_HOME", xdg)

			// When: the scheduled audit logger is initialized.
			logger, closeLog, err := updatesLogger()

			// Then: initialization reports the I/O failure instead of discarding logs.
			if err == nil {
				if closeLog != nil {
					closeLog()
				}
				t.Fatalf("updatesLogger returned logger %v without an error", logger)
			}
		})
	}
}

func TestUpdatesRunOnce_LoggerFailure_stops_before_executor(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T, xdg string)
	}{
		{
			name: "directory creation failure",
			setup: func(t *testing.T, xdg string) {
				t.Helper()
				if err := os.WriteFile(xdg, []byte("not a directory"), 0o600); err != nil {
					t.Fatalf("write XDG_CONFIG_HOME file: %v", err)
				}
			},
		},
		{
			name: "log open failure",
			setup: func(t *testing.T, xdg string) {
				t.Helper()
				if err := os.MkdirAll(filepath.Join(xdg, "genv", "updates.log"), 0o700); err != nil {
					t.Fatalf("mkdir updates.log directory: %v", err)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given: audit logger initialization fails and the executor is observable.
			xdg := filepath.Join(t.TempDir(), "xdg")
			tt.setup(t, xdg)
			t.Setenv("XDG_CONFIG_HOME", xdg)
			originalRun := updatesRunUpgrade
			runCalls := 0
			updatesRunUpgrade = func(ctx context.Context, opts upgrade.UpgradeRunOptions) upgrade.UpgradeRunResult {
				runCalls++
				return upgrade.UpgradeRunResult{}
			}
			t.Cleanup(func() { updatesRunUpgrade = originalRun })

			// When: the one-shot worker starts.
			var code int
			errOut := captureStderr(t, func() {
				code = run([]string{"updates", "__run-once", "--file", "unused.json", "--lock-file", "unused.lock.json"})
			})

			// Then: it exits as I/O failure before any auto-apply execution.
			if code != exitIO {
				t.Fatalf("updates __run-once code = %d, want %d", code, exitIO)
			}
			if runCalls != 0 {
				t.Fatalf("RunUpgrade calls = %d, want zero", runCalls)
			}
			if errOut != "genv updates: audit log unavailable\n" {
				t.Fatalf("stderr = %q, want minimal audit log error", errOut)
			}
		})
	}
}

func TestUpdatesRunOnce_UpgradeFailure_logs_typed_failures_and_unmatched_legacy_errors(t *testing.T) {
	// Given: auto-apply has typed failures, one unmatched legacy error, and oversized diagnostics.
	dir := t.TempDir()
	xdg := filepath.Join(dir, "xdg")
	t.Setenv("XDG_CONFIG_HOME", xdg)
	specPath := filepath.Join(dir, "genv.json")
	lockPath := filepath.Join(dir, "genv.lock.json")
	if err := os.WriteFile(specPath, []byte(`{"schemaVersion":"6","packages":[{"id":"alpha"},{"id":"beta"},{"id":"gamma"}],"updates":{"enabled":true,"interval":"1h","autoApply":true}}`), 0o644); err != nil {
		t.Fatalf("write spec: %v", err)
	}
	writeLock(t, lockPath, nil)
	plan := upgrade.UpgradePlan{Actions: []resolver.UpgradeAction{
		{LPs: []genvfile.LockedPackage{{ID: "alpha"}, {ID: "beta"}}},
		{LPs: []genvfile.LockedPackage{{ID: "gamma"}}},
	}}
	originalBuild := updatesBuildPlan
	updatesBuildPlan = func(opts upgrade.UpgradeOptions) (upgrade.UpgradePlan, error) { return plan, nil }
	t.Cleanup(func() { updatesBuildPlan = originalBuild })
	originalRun := updatesRunUpgrade
	updatesRunUpgrade = func(ctx context.Context, opts upgrade.UpgradeRunOptions) upgrade.UpgradeRunResult {
		_, _ = io.WriteString(opts.Stderr, "TOKEN=synthetic-token\nAuthorization: Bearer synthetic-bearer\n"+strings.Repeat("diagnostic", 10_000))
		firstErr := errors.New("first synthetic failure PASSWORD=synthetic-password")
		secondErr := errors.New("second synthetic failure")
		legacyErr := errors.New("legacy synthetic failure")
		return upgrade.UpgradeRunResult{
			Plan:   opts.Plan,
			Errors: []error{secondErr, legacyErr, firstErr},
			Failures: []resolver.UpgradeFailure{
				{IDs: []string{"alpha", "beta", "pkg?auth=credential-id-secret"}, Err: firstErr},
				{IDs: []string{"gamma"}, Err: secondErr},
			},
		}
	}
	t.Cleanup(func() { updatesRunUpgrade = originalRun })

	// When: the scheduled worker executes the synthetic upgrade result.
	code := run([]string{"updates", "__run-once", "--file", specPath, "--lock-file", lockPath})

	// Then: each action failure and one bounded sanitized diagnostic event are audited.
	if code != exitLogic {
		t.Fatalf("updates __run-once code = %d, want %d", code, exitLogic)
	}
	logBytes, err := os.ReadFile(filepath.Join(xdg, "genv", "updates.log"))
	if err != nil {
		t.Fatalf("read updates log: %v", err)
	}
	logText := string(logBytes)
	if strings.Count(logText, "updates.apply.failed") != 3 {
		t.Fatalf("updates log has %d failure events, want 3: %q", strings.Count(logText, "updates.apply.failed"), logText)
	}
	for _, want := range []string{"alpha", "beta", "gamma", "second synthetic failure", "legacy synthetic failure"} {
		if !strings.Contains(logText, want) {
			t.Fatalf("updates log missing %q: %q", want, logText)
		}
	}
	for _, want := range []struct {
		err string
		ids []string
	}{
		{err: "second synthetic failure", ids: []string{"gamma"}},
	} {
		for _, line := range strings.Split(logText, "\n") {
			if strings.Contains(line, want.err) {
				for _, id := range want.ids {
					if !strings.Contains(line, id) {
						t.Fatalf("failure line %q missing correlated ID %q", line, id)
					}
				}
			}
		}
	}
	for _, line := range strings.Split(logText, "\n") {
		if strings.Contains(line, "alpha") && strings.Contains(line, "beta") && strings.Count(line, updatesDiagnosticRedactionMarker) < 2 {
			t.Fatalf("credential-bearing failure line is not fully redacted: %q", line)
		}
	}
	for _, secret := range []string{"synthetic-token", "synthetic-bearer", "synthetic-password"} {
		if strings.Contains(logText, secret) {
			t.Fatalf("updates log contains synthetic secret %q", secret)
		}
	}
	if strings.Contains(logText, "credential-id-secret") {
		t.Fatalf("updates log contains credential-bearing package ID: %q", logText)
	}
	if !strings.Contains(logText, updatesDiagnosticRedactionMarker) {
		t.Fatalf("updates log missing package ID redaction marker: %q", logText)
	}
	if strings.Count(logText, "updates.apply.diagnostics") != 1 {
		t.Fatalf("updates log has %d diagnostic events, want 1", strings.Count(logText, "updates.apply.diagnostics"))
	}
	if !strings.Contains(logText, updatesDiagnosticTruncationMarker) {
		t.Fatalf("updates log missing truncation marker")
	}
	if !strings.Contains(logText, "updates.apply.completed") {
		t.Fatalf("updates log missing updates.apply.completed: %q", logText)
	}
}

func TestUpdatesRunOnce_unsupported_notifier_logs_warning_without_crashing(t *testing.T) {
	// Given: notify is enabled but the notifier lookup fails.
	dir := t.TempDir()
	xdg := filepath.Join(dir, "xdg")
	t.Setenv("XDG_CONFIG_HOME", xdg)
	specPath := filepath.Join(dir, "genv.json")
	lockPath := filepath.Join(dir, "genv.lock.json")
	if err := os.WriteFile(specPath, []byte(`{"schemaVersion":"6","packages":[{"id":"alpha"}],"updates":{"enabled":true,"interval":"1h","notify":true}}`), 0o644); err != nil {
		t.Fatalf("write spec: %v", err)
	}
	writeLock(t, lockPath, []genvfile.LockedPackage{{ID: "alpha", Manager: "missing-manager", PkgName: "alpha"}})
	// A notification is only attempted when packages are outdated, so inject a
	// non-empty plan; the point of this test is the notifier-unavailable path.
	originalBuild := updatesBuildPlan
	updatesBuildPlan = func(opts upgrade.UpgradeOptions) (upgrade.UpgradePlan, error) {
		return upgrade.UpgradePlan{Actions: []resolver.UpgradeAction{
			{LPs: []genvfile.LockedPackage{{ID: "alpha"}}},
		}}, nil
	}
	t.Cleanup(func() { updatesBuildPlan = originalBuild })
	originalLookPath := updatesLookPath
	updatesLookPath = func(file string) (string, error) { return "", os.ErrNotExist }
	t.Cleanup(func() { updatesLookPath = originalLookPath })

	// When: the check-only worker runs.
	code := run([]string{"updates", "__run-once", "--file", specPath, "--lock-file", lockPath})

	// Then: it logs the notifier warning but does not fail the check.
	if code != exitOK {
		t.Fatalf("updates __run-once notify missing: expected exitOK (%d), got %d", exitOK, code)
	}
	logBytes, err := os.ReadFile(filepath.Join(xdg, "genv", "updates.log"))
	if err != nil {
		t.Fatalf("read updates log: %v", err)
	}
	if !strings.Contains(string(logBytes), "updates.notify.unavailable") {
		t.Fatalf("updates log = %q, want notifier warning", string(logBytes))
	}
}

func TestUpgradeHookEnv_builds_deterministic_context_in_plan_order(t *testing.T) {
	// Given: plan, skipped, upgraded, and failed packages deliberately not sorted alphabetically.
	plan := []resolver.UpgradeAction{
		{LPs: []genvfile.LockedPackage{{ID: "zed", Manager: "brew", PkgName: "zed"}, {ID: "alpha", Manager: "brew", PkgName: "alpha"}}},
		{LPs: []genvfile.LockedPackage{{ID: "bun-tool", Manager: "bun", PkgName: "bun-tool"}}},
	}
	skipped := []resolver.SkippedPackage{{ID: "skip-z", Manager: "missing"}, {ID: "skip-a", Manager: "missing"}}
	upgraded := []genvfile.LockedPackage{{ID: "zed"}, {ID: "bun-tool"}}

	// When: post-upgrade hook env is built.
	env := upgradeHookEnv(upgradeHookOptions{
		Phase:    "post",
		Host:     "ci-host",
		Plan:     plan,
		Skipped:  skipped,
		Upgraded: upgraded,
		Failed:   []string{"alpha"},
	})

	// Then: comma-separated lists follow plan/result order, not map or lexical order.
	want := []string{
		"GENV_EVENT=upgrade",
		"GENV_PHASE=post-upgrade",
		"GENV_HOST=ci-host",
		"GENV_PROFILE=",
		"GENV_DRY_RUN=false",
		"GENV_INSTALLED=",
		"GENV_REMOVED=",
		"GENV_UPGRADED=zed,bun-tool",
		"GENV_FAILED=alpha",
		"GENV_SKIPPED=skip-z,skip-a",
		"GENV_UPGRADE_MANAGERS=brew,bun",
	}
	if !slices.Equal(env, want) {
		t.Fatalf("upgradeHookEnv() = %v, want %v", env, want)
	}
}

// ---- genv apply new flags ----------------------------------------------------

func TestApplyCmd_Yes_AlreadyUpToDate(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	path := filepath.Join(dir, "genv.json")
	lockPath := genvfile.LockPathFrom(path)

	run([]string{"add", "--file", path, "git"})
	writeLock(t, lockPath, []genvfile.LockedPackage{
		{ID: "git", Manager: "brew", PkgName: "git"},
	})

	// --yes with an up-to-date state exits OK immediately (no prompt, no work).
	code := run([]string{"apply", "--file", path, "--yes"})
	if code != exitOK {
		t.Errorf("--yes up to date: expected exitOK (%d), got %d", exitOK, code)
	}
}

func TestApplyCmd_Debug_DryRun_NoCrash(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	path := filepath.Join(dir, "genv.json")
	run([]string{"add", "--file", path, "git"})
	code := run([]string{"apply", "--file", path, "--dry-run", "--debug"})
	if code != exitOK {
		t.Errorf("--debug dry-run: expected exitOK (%d), got %d", exitOK, code)
	}
}

func TestApplyCmd_Timeout_DryRun_NoCrash(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	path := filepath.Join(dir, "genv.json")
	run([]string{"add", "--file", path, "git"})
	code := run([]string{"apply", "--file", path, "--dry-run", "--timeout", "5m"})
	if code != exitOK {
		t.Errorf("--timeout dry-run: expected exitOK (%d), got %d", exitOK, code)
	}
}

func TestApplyCmd_DryRun_JsonOutput(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
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
	t.Setenv("XDG_CONFIG_HOME", dir)
	path := filepath.Join(dir, "genv.json")
	lockPath := genvfile.LockPathFrom(path)

	run([]string{"add", "--file", path, "git"})
	writeLock(t, lockPath, []genvfile.LockedPackage{
		{ID: "git", Manager: "brew", PkgName: "git"},
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

func TestBuildEditorCmd(t *testing.T) {
	tests := []struct {
		editor   string
		file     string
		wantErr  bool
		wantPath string // only check base name if not empty
		wantArgs []string
	}{
		{"vi", "test.json", false, "vi", []string{"vi", "test.json"}},
		{"code --wait", "test.json", false, "code", []string{"code", "--wait", "test.json"}},
		{"/usr/bin/vim -R", "test.json", false, "vim", []string{"/usr/bin/vim", "-R", "test.json"}},
		{"rm -rf", "test.json", true, "", nil},
		{"", "test.json", false, "vi", []string{"vi", "test.json"}},
		{"   ", "test.json", false, "vi", []string{"vi", "test.json"}},
		{"code", "test.json", false, "code", []string{"code", "test.json"}},
		{"/usr/bin/nano", "test.json", false, "nano", []string{"/usr/bin/nano", "test.json"}},
		{"emacs -nw", "test.json", false, "emacs", []string{"emacs", "-nw", "test.json"}},
		{"/bin/sh -c 'rm -rf /'", "test.json", true, "", nil},
	}

	for _, tc := range tests {
		t.Run(tc.editor, func(t *testing.T) {
			cmd, err := buildEditorCmd(tc.editor, tc.file)
			if tc.wantErr {
				if err == nil {
					t.Errorf("buildEditorCmd(%q, %q): expected error, got nil", tc.editor, tc.file)
				}
				return
			}
			if err != nil {
				t.Fatalf("buildEditorCmd(%q, %q): unexpected error: %v", tc.editor, tc.file, err)
			}
			if filepath.Base(cmd.Path) != tc.wantPath {
				t.Errorf("buildEditorCmd path: got %q, want base %q", cmd.Path, tc.wantPath)
			}
			if len(cmd.Args) != len(tc.wantArgs) {
				t.Errorf("buildEditorCmd args length: got %d, want %d (args: %v, want: %v)", len(cmd.Args), len(tc.wantArgs), cmd.Args, tc.wantArgs)
			} else {
				for i := range cmd.Args {
					if cmd.Args[i] != tc.wantArgs[i] {
						t.Errorf("buildEditorCmd args[%d]: got %q, want %q", i, cmd.Args[i], tc.wantArgs[i])
					}
				}
			}
		})
	}
}

func TestParseCommandWords(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    []string
		wantErr bool
	}{
		{name: "simple", input: "sleep 1", want: []string{"sleep", "1"}},
		{name: "double quoted", input: `sh -c "echo hello world"`, want: []string{"sh", "-c", "echo hello world"}},
		{name: "single quoted", input: `sh -c 'echo hello world'`, want: []string{"sh", "-c", "echo hello world"}},
		{name: "escaped spaces", input: `echo hello\ world`, want: []string{"echo", "hello world"}},
		{name: "unterminated quote", input: `sh -c "echo`, wantErr: true},
		{name: "empty", input: `   `, wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseCommandWords(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("parseCommandWords(%q): expected error", tc.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseCommandWords(%q): %v", tc.input, err)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("parseCommandWords(%q): len=%d want=%d got=%v", tc.input, len(got), len(tc.want), got)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("parseCommandWords(%q)[%d]=%q want %q", tc.input, i, got[i], tc.want[i])
				}
			}
		})
	}
}

// ---- genv pull ---------------------------------------------------------------

func TestResolvePullSource(t *testing.T) {
	tests := []struct {
		name    string
		repo    *schema.Repo
		urlFlag string
		refFlag string
		wantURL string
		wantRef string
		wantErr bool
	}{
		{
			name:    "from repo only",
			repo:    &schema.Repo{URL: "https://example.com/repo.git", Ref: "stable"},
			wantURL: "https://example.com/repo.git",
			wantRef: "stable",
		},
		{
			name:    "from repo with default ref",
			repo:    &schema.Repo{URL: "https://example.com/repo.git"},
			wantURL: "https://example.com/repo.git",
			wantRef: "main",
		},
		{
			name:    "url flag overrides repo url",
			repo:    &schema.Repo{URL: "https://example.com/repo.git", Ref: "stable"},
			urlFlag: "https://other.example/repo.git",
			wantURL: "https://other.example/repo.git",
			wantRef: "stable",
		},
		{
			name:    "ref flag overrides repo ref",
			repo:    &schema.Repo{URL: "https://example.com/repo.git", Ref: "stable"},
			refFlag: "develop",
			wantURL: "https://example.com/repo.git",
			wantRef: "develop",
		},
		{
			name:    "flags alone are sufficient",
			urlFlag: "https://example.com/repo.git",
			refFlag: "v1.0.0",
			wantURL: "https://example.com/repo.git",
			wantRef: "v1.0.0",
		},
		{
			name:    "empty repo and no flags errors",
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			url, ref, err := resolvePullSource(tc.repo, tc.urlFlag, tc.refFlag)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("resolvePullSource: expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("resolvePullSource: unexpected error: %v", err)
			}
			if url != tc.wantURL {
				t.Errorf("url: got %q, want %q", url, tc.wantURL)
			}
			if ref != tc.wantRef {
				t.Errorf("ref: got %q, want %q", ref, tc.wantRef)
			}
		})
	}
}

func TestPullCmd_StdoutTargetFails(t *testing.T) {
	code := run([]string{"pull", "--file", "-"})
	if code != exitUsage {
		t.Errorf("pull to stdout: expected exitUsage (%d), got %d", exitUsage, code)
	}
}

func TestPullCmd_NoURLError(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	path := filepath.Join(dir, "genv.json")

	if err := os.WriteFile(path, []byte(`{"schemaVersion":"5","packages":[]}`), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	code := run([]string{"pull", "--file", path})
	if code != exitUsage {
		t.Errorf("no url: expected exitUsage (%d), got %d", exitUsage, code)
	}
}

func TestPullCmd_DryRun(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	path := filepath.Join(dir, "genv.json")

	if err := os.WriteFile(path, []byte(`{"schemaVersion":"5","packages":[],"repo":{"url":"https://example.com/spec.git","ref":"main"}}`), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	var code int
	out := captureStdout(t, func() {
		code = run([]string{"pull", "--file", path, "--dry-run"})
	})

	if code != exitOK {
		t.Fatalf("dry-run: expected exitOK (%d), got %d", exitOK, code)
	}
	if !strings.Contains(out, "https://example.com/spec.git") {
		t.Errorf("dry-run output missing url: %q", out)
	}
	if !strings.Contains(out, "main") {
		t.Errorf("dry-run output missing ref: %q", out)
	}
}

func TestPullCmd_DryRunListsLocalFileAssets(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	path := filepath.Join(dir, "genv.json")

	spec := `{
  "schemaVersion": "7",
  "packages": [],
  "repo": {"url": "https://example.com/spec.git", "ref": "main"},
  "files": {
    "links": [
      {"source": "assets/profile", "target": "~/.profile"},
      {"source": "secrets/token", "target": "~/.token"}
    ]
  }
}`
	if err := os.WriteFile(path, []byte(spec), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	var code int
	out := captureStdout(t, func() {
		code = run([]string{"pull", "--file", path, "--dry-run"})
	})

	if code != exitOK {
		t.Fatalf("dry-run: expected exitOK (%d), got %d", exitOK, code)
	}
	if !strings.Contains(out, "would copy assets:") || !strings.Contains(out, "assets/profile") {
		t.Errorf("dry-run output missing asset list: %q", out)
	}
	if strings.Contains(out, "secrets/token") {
		t.Errorf("dry-run output included skipped secret asset: %q", out)
	}
}

func TestPullCmd_DryRunFlagOverridesRepo(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	path := filepath.Join(dir, "genv.json")

	if err := os.WriteFile(path, []byte(`{"schemaVersion":"5","packages":[],"repo":{"url":"https://example.com/spec.git","ref":"main"}}`), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	var code int
	out := captureStdout(t, func() {
		code = run([]string{"pull", "--file", path, "--url", "https://other.example/spec.git", "--ref", "dev", "--dry-run"})
	})

	if code != exitOK {
		t.Fatalf("dry-run override: expected exitOK (%d), got %d", exitOK, code)
	}
	if !strings.Contains(out, "https://other.example/spec.git") {
		t.Errorf("dry-run output missing override url: %q", out)
	}
	if !strings.Contains(out, "dev") {
		t.Errorf("dry-run output missing override ref: %q", out)
	}
}

func TestPull_CopiesRelativeFileAssets(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}

	dir := t.TempDir()
	repoDir := filepath.Join(dir, "remote")
	destDir := filepath.Join(dir, "dest")
	t.Setenv("XDG_CACHE_HOME", filepath.Join(dir, "cache"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "config"))

	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(repo): %v", err)
	}
	runGitForTest(t, repoDir, "init", "-b", "main")
	if err := os.MkdirAll(filepath.Join(repoDir, "assets"), 0o755); err != nil {
		t.Fatalf("MkdirAll(assets): %v", err)
	}
	if err := os.MkdirAll(filepath.Join(repoDir, "templates"), 0o755); err != nil {
		t.Fatalf("MkdirAll(templates): %v", err)
	}
	if err := os.MkdirAll(filepath.Join(repoDir, "secrets"), 0o755); err != nil {
		t.Fatalf("MkdirAll(secrets): %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoDir, "assets", "profile"), []byte("profile"), 0o644); err != nil {
		t.Fatalf("WriteFile(profile): %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoDir, "templates", "app.tmpl"), []byte("template"), 0o644); err != nil {
		t.Fatalf("WriteFile(template): %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoDir, "secrets", "token"), []byte("secret"), 0o644); err != nil {
		t.Fatalf("WriteFile(secret): %v", err)
	}
	remoteSpec := fmt.Sprintf(`{
  "schemaVersion": "7",
  "packages": [],
  "repo": {"url": %q, "ref": "main"},
  "files": {
    "links": [
      {"source": "assets/profile", "target": "~/.profile"},
      {"source": "secrets/token", "target": "~/.token"}
    ],
    "templates": [
      {"source": "templates/app.tmpl", "target": "~/.config/app/config"}
    ]
  }
}`, repoDir)
	if err := os.WriteFile(filepath.Join(repoDir, "genv.json"), []byte(remoteSpec), 0o644); err != nil {
		t.Fatalf("WriteFile(remote spec): %v", err)
	}
	runGitForTest(t, repoDir, "add", ".")
	runGitForTest(t, repoDir, "-c", "user.name=genv test", "-c", "user.email=test@example.com", "commit", "-m", "initial")

	specPath := filepath.Join(destDir, "genv.json")
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(dest): %v", err)
	}
	localSpec := fmt.Sprintf(`{"schemaVersion":"7","packages":[],"repo":{"url":%q,"ref":"main"}}`, repoDir)
	if err := os.WriteFile(specPath, []byte(localSpec), 0o644); err != nil {
		t.Fatalf("WriteFile(local spec): %v", err)
	}

	code := run([]string{"pull", "--file", specPath})
	if code != exitOK {
		t.Fatalf("pull: expected exitOK (%d), got %d", exitOK, code)
	}
	assertPulledFileContent(t, filepath.Join(destDir, "assets", "profile"), "profile")
	assertPulledFileContent(t, filepath.Join(destDir, "templates", "app.tmpl"), "template")
	assertPulledPathNotExists(t, filepath.Join(destDir, "secrets", "token"))
}

func runGitForTest(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}

func assertPulledFileContent(t *testing.T, path, want string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", path, err)
	}
	if string(got) != want {
		t.Fatalf("%s = %q, want %q", path, got, want)
	}
}

func assertPulledPathNotExists(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err == nil {
		t.Fatalf("%s exists, want absent", path)
	} else if !os.IsNotExist(err) {
		t.Fatalf("Stat(%s): %v", path, err)
	}
}
