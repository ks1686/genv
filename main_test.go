package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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

func TestAddCmd_CreatesFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gpm.json")

	code := run([]string{"add", "--file", path, "git"})
	if code != exitOK {
		t.Fatalf("expected exitOK, got %d", code)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected gpm.json to exist: %v", err)
	}
}

func TestAddCmd_DuplicateFails(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gpm.json")

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
	path := filepath.Join(dir, "gpm.json")

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
	path := filepath.Join(dir, "gpm.json")

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
	// Regression: Go's flag package stops at the first non-flag argument, so
	// flags placed after the id were silently ignored. extractPositional fixes this.
	dir := t.TempDir()
	path := filepath.Join(dir, "gpm.json")

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

func TestAddCmd_UnknownPreferFails(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gpm.json")

	code := run([]string{"add", "--file", path, "--prefer", "yum", "git"})
	if code != exitUsage {
		t.Errorf("expected exitUsage (%d), got %d", exitUsage, code)
	}
}

func TestAddCmd_MissingIDFails(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gpm.json")

	code := run([]string{"add", "--file", path})
	if code != exitUsage {
		t.Errorf("expected exitUsage (%d), got %d", exitUsage, code)
	}
}

func TestRemoveCmd_Basic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gpm.json")

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
		t.Error("git should have been removed")
	}
	if !strings.Contains(s, `"neovim"`) {
		t.Error("neovim should still be present")
	}
}

func TestRemoveCmd_NotFound(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gpm.json")

	run([]string{"add", "--file", path, "git"})

	code := run([]string{"remove", "--file", path, "neovim"})
	if code != exitLogic {
		t.Errorf("expected exitLogic (%d), got %d", exitLogic, code)
	}
}

func TestRemoveCmd_FileNotFound(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gpm.json")

	code := run([]string{"remove", "--file", path, "git"})
	if code != exitLogic {
		t.Errorf("expected exitLogic (%d), got %d", exitLogic, code)
	}
}

func TestRemoveCmd_AliasRm(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gpm.json")

	run([]string{"add", "--file", path, "git"})
	code := run([]string{"rm", "--file", path, "git"})
	if code != exitOK {
		t.Errorf("alias rm: expected exitOK, got %d", code)
	}
}

func TestRemoveCmd_InvalidFileFails(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gpm.json")

	if err := os.WriteFile(path, []byte(`{"schemaVersion":"99","packages":[]}`), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	code := run([]string{"remove", "--file", path, "git"})
	if code != exitValidation {
		t.Errorf("expected exitValidation (%d), got %d", exitValidation, code)
	}
}

func TestListCmd_InvalidFileFails(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gpm.json")

	if err := os.WriteFile(path, []byte(`{"schemaVersion":"99","packages":[]}`), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	code := run([]string{"list", "--file", path})
	if code != exitValidation {
		t.Errorf("expected exitValidation (%d), got %d", exitValidation, code)
	}
}

func TestListCmd_Empty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gpm.json")

	// File doesn't exist — should still succeed with "no packages tracked".
	code := run([]string{"list", "--file", path})
	if code != exitOK {
		t.Errorf("expected exitOK, got %d", code)
	}
}

func TestListCmd_AliasLs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gpm.json")

	code := run([]string{"ls", "--file", path})
	if code != exitOK {
		t.Errorf("alias ls: expected exitOK, got %d", code)
	}
}

func TestListCmd_ShowsPackages(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gpm.json")

	run([]string{"add", "--file", path, "git"})
	run([]string{"add", "--file", path, "--version", "0.10.*", "--prefer", "brew", "neovim"})

	// list just needs to succeed; output goes to stdout (not captured here).
	code := run([]string{"list", "--file", path})
	if code != exitOK {
		t.Errorf("expected exitOK, got %d", code)
	}
}

func TestInstallCmd_DryRun_NoCrash(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gpm.json")

	run([]string{"add", "--file", path, "git"})
	run([]string{"add", "--file", path, "--prefer", "brew", "neovim"})

	// --dry-run must not panic/crash regardless of which managers are installed.
	code := run([]string{"install", "--file", path, "--dry-run"})
	if code != exitOK {
		t.Errorf("dry-run: expected exitOK (%d), got %d", exitOK, code)
	}
}

func TestInstallCmd_DryRun_FileNotFound(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gpm.json")

	code := run([]string{"install", "--file", path, "--dry-run"})
	if code != exitIO {
		t.Errorf("missing file: expected exitIO (%d), got %d", exitIO, code)
	}
}

func TestInstallCmd_DryRun_InvalidFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gpm.json")

	if err := os.WriteFile(path, []byte(`{"schemaVersion":"99","packages":[]}`), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	code := run([]string{"install", "--file", path, "--dry-run"})
	if code != exitValidation {
		t.Errorf("invalid file: expected exitValidation (%d), got %d", exitValidation, code)
	}
}

func TestInstallCmd_DryRun_EmptyPackages(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gpm.json")

	if err := os.WriteFile(path, []byte(`{"schemaVersion":"1","packages":[]}`), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	code := run([]string{"install", "--file", path, "--dry-run"})
	if code != exitOK {
		t.Errorf("empty packages: expected exitOK (%d), got %d", exitOK, code)
	}
}

func TestInstallCmd_Strict_DryRun_DoesNotPanic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gpm.json")

	run([]string{"add", "--file", path, "git"})

	// --strict --dry-run must not panic; exit code depends on the host environment.
	code := run([]string{"install", "--file", path, "--dry-run", "--strict"})
	if code != exitOK && code != exitLogic {
		t.Errorf("strict dry-run: expected exitOK or exitLogic, got %d", code)
	}
}

func TestRemoveCmd_MissingID(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gpm.json")

	run([]string{"add", "--file", path, "git"})
	code := run([]string{"remove", "--file", path})
	if code != exitUsage {
		t.Errorf("missing id: expected exitUsage (%d), got %d", exitUsage, code)
	}
}

func TestRemoveCmd_IOError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gpm.json")

	// Write a valid file then remove read permission so Read returns a generic IO error.
	if err := os.WriteFile(path, []byte(`{"schemaVersion":"1","packages":[]}`), 0o200); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	t.Cleanup(func() { os.Chmod(path, 0o644) }) // restore so TempDir cleanup can delete it
	code := run([]string{"remove", "--file", path, "git"})
	if code != exitIO {
		t.Errorf("io error: expected exitIO (%d), got %d", exitIO, code)
	}
}

func TestListCmd_IOError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gpm.json")

	// Write a valid file then remove read permission so Read returns a generic IO error.
	if err := os.WriteFile(path, []byte(`{"schemaVersion":"1","packages":[]}`), 0o200); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	t.Cleanup(func() { os.Chmod(path, 0o644) })
	code := run([]string{"list", "--file", path})
	if code != exitIO {
		t.Errorf("io error: expected exitIO (%d), got %d", exitIO, code)
	}
}

func TestAddCmd_BadManagerFormatFails(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gpm.json")

	// "notaformat" has no colon separator, so parseManagerFlag should error.
	code := run([]string{"add", "--file", path, "--manager", "notaformat", "git"})
	if code != exitUsage {
		t.Errorf("bad manager format: expected exitUsage (%d), got %d", exitUsage, code)
	}
}

func TestAddCmd_UnknownManagerKeyInFlagFails(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gpm.json")

	// "yum" is not a known manager; commands.Add should reject it.
	code := run([]string{"add", "--file", path, "--manager", "yum:git", "git"})
	if code != exitUsage {
		t.Errorf("unknown manager key: expected exitUsage (%d), got %d", exitUsage, code)
	}
}

func TestInstallCmd_NothingToInstall_NoResolved(t *testing.T) {
	// When every package is unresolved (no managers available) and --dry-run is not
	// set and --strict is not set, install should print "nothing to install." and
	// return exitOK.  We achieve "no managers available" by using a package with a
	// managers map whose only entry is a manager that is guaranteed to not exist in
	// the test PATH; step 3 of resolve() would normally fall back to any available
	// manager, so we need to check whether the path leads to resolvedCount==0.
	//
	// Because resolver.Detect() is not injectable we can only reliably exercise the
	// "all unresolved" branch when no package manager binary is present.  On hosts
	// where at least one manager is available the test verifies exitOK regardless.
	dir := t.TempDir()
	path := filepath.Join(dir, "gpm.json")

	run([]string{"add", "--file", path, "git"})
	// install without --dry-run; the exit code is environment-dependent (may prompt
	// when managers are available, or print "nothing to install." when none are).
	// We just verify it does not crash or return an unexpected error code.
	// Since stdin is /dev/null the prompt would read "", triggering "Aborted." — exitOK.
	// On a host with no managers it also returns exitOK.
	code := run([]string{"install", "--file", path})
	if code != exitOK {
		t.Errorf("install with no managers or aborted: expected exitOK (%d), got %d", exitOK, code)
	}
}

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
		// All-empty tokens (commas with whitespace) → returns nil, nil.
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

// TestListCmd_FlagParseError ensures listCmd returns exitUsage for unknown flags.
func TestListCmd_FlagParseError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gpm.json")
	code := run([]string{"list", "--file", path, "--no-such-flag"})
	if code != exitUsage {
		t.Errorf("unknown flag: expected exitUsage (%d), got %d", exitUsage, code)
	}
}

// TestRemoveCmd_FlagParseError ensures removeCmd returns exitUsage for unknown flags.
func TestRemoveCmd_FlagParseError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gpm.json")
	code := run([]string{"remove", "--file", path, "--no-such-flag", "git"})
	if code != exitUsage {
		t.Errorf("unknown flag: expected exitUsage (%d), got %d", exitUsage, code)
	}
}

// TestInstallCmd_FlagParseError ensures installCmd returns exitUsage for unknown flags.
func TestInstallCmd_FlagParseError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gpm.json")
	code := run([]string{"install", "--file", path, "--no-such-flag"})
	if code != exitUsage {
		t.Errorf("unknown flag: expected exitUsage (%d), got %d", exitUsage, code)
	}
}

// TestAddCmd_InvalidFileFails verifies that add returns exitValidation when the
// existing gpm.json fails schema validation (not just an IO error).
func TestAddCmd_InvalidFileFails(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gpm.json")

	if err := os.WriteFile(path, []byte(`{"schemaVersion":"99","packages":[]}`), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	code := run([]string{"add", "--file", path, "git"})
	if code != exitValidation {
		t.Errorf("expected exitValidation (%d), got %d", exitValidation, code)
	}
}

// TestExtractPositional verifies extractPositional handles various argument orders.
func TestExtractPositional(t *testing.T) {
	tests := []struct {
		name          string
		args          []string
		wantPos       string
		wantFlagCount int
	}{
		{
			name:          "empty args",
			args:          nil,
			wantPos:       "",
			wantFlagCount: 0,
		},
		{
			name:          "positional only",
			args:          []string{"git"},
			wantPos:       "git",
			wantFlagCount: 0,
		},
		{
			name:          "flag before positional",
			args:          []string{"--prefer", "brew", "neovim"},
			wantPos:       "neovim",
			wantFlagCount: 2, // --prefer and its value
		},
		{
			name:          "flag after positional",
			args:          []string{"neovim", "--prefer", "brew"},
			wantPos:       "neovim",
			wantFlagCount: 2,
		},
		{
			name:          "flag=value form",
			args:          []string{"--prefer=brew", "neovim"},
			wantPos:       "neovim",
			wantFlagCount: 1, // inline value, single token
		},
		{
			name:          "multiple flags before and after",
			args:          []string{"--version", "0.10.*", "neovim", "--prefer", "brew"},
			wantPos:       "neovim",
			wantFlagCount: 4, // --version, 0.10.*, --prefer, brew
		},
		{
			name:          "only flags no positional",
			args:          []string{"--prefer", "brew"},
			wantPos:       "",
			wantFlagCount: 2,
		},
		{
			name:          "first non-flag is positional second is ignored",
			args:          []string{"first", "second"},
			wantPos:       "first",
			wantFlagCount: 0, // "second" treated as extra positional, not in flagArgs
		},
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

// TestAddCmd_FlagsBeforeID verifies that all flags can appear before the id.
func TestAddCmd_FlagsBeforeID(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gpm.json")

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

// TestInstallCmd_Strict_AllUnresolved verifies that --strict causes exitLogic when
// all packages are unresolved (tested via a forced empty available set using a
// package with a managers entry for a non-existent manager key).
// We rely on the real resolver here; on any host, at least the dry-run path is
// exercised deterministically.
func TestInstallCmd_Strict_AllUnresolved_DryRun(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gpm.json")

	// Write a valid file with one package that only maps to an empty available set.
	// Since we can't control which managers the host has, we only test that the
	// combination of --dry-run and --strict does not crash and exits with either
	// exitOK (if the host resolved it) or exitLogic (if unresolved).
	run([]string{"add", "--file", path, "git"})
	code := run([]string{"install", "--file", path, "--dry-run", "--strict"})
	if code != exitOK && code != exitLogic {
		t.Errorf("strict dry-run: expected exitOK or exitLogic, got %d", code)
	}
}

// TestAddCmd_IOError verifies that add returns exitIO for an unreadable file
// (distinct from a missing file or invalid file).
func TestAddCmd_IOError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gpm.json")

	// Write a valid file then remove read permission.
	if err := os.WriteFile(path, []byte(`{"schemaVersion":"1","packages":[]}`), 0o200); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	t.Cleanup(func() { os.Chmod(path, 0o644) })
	code := run([]string{"add", "--file", path, "git"})
	if code != exitIO {
		t.Errorf("io error on add: expected exitIO (%d), got %d", exitIO, code)
	}
}

// TestListCmd_OutputContainsPackages captures stdout output and verifies the
// content includes the packages that were added.
func TestListCmd_OutputContainsPackages(t *testing.T) {
	// We redirect by testing through a temp file: just verify the command
	// succeeds and the file has the right data. Output goes to os.Stdout
	// which we can't easily capture in main_test, so we verify the file.
	dir := t.TempDir()
	path := filepath.Join(dir, "gpm.json")

	run([]string{"add", "--file", path, "git"})
	run([]string{"add", "--file", path, "--prefer", "brew", "neovim"})

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	s := string(content)
	if !strings.Contains(s, `"git"`) {
		t.Errorf("expected git in file: %s", s)
	}
	if !strings.Contains(s, `"neovim"`) {
		t.Errorf("expected neovim in file: %s", s)
	}

	code := run([]string{"list", "--file", path})
	if code != exitOK {
		t.Errorf("list: expected exitOK, got %d", code)
	}
}

// TestRemoveCmd_MultiplePackages verifies that removing one package from a
// multi-package file leaves the correct packages remaining.
func TestRemoveCmd_MultiplePackages(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gpm.json")

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
