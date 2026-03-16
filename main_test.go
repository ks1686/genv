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
	t.Cleanup(func() { _ = os.Chdir(orig) })
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

func TestAddCmd_UnknownPreferFails(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gpm.json")

	code := run([]string{"add", "--file", path, "--prefer", "yum", "git"})
	if code == exitOK {
		t.Error("expected failure for unknown prefer, got exitOK")
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

	content, _ := os.ReadFile(path)
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
