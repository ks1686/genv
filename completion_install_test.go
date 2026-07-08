package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// wantFile pairs a shell with the filename and content marker its completion
// script must be installed as.
var completionInstallCases = []struct {
	shell    string
	filename string
	marker   string
}{
	{"bash", "genv", "# bash completion for genv"},
	{"zsh", "_genv", "#compdef genv"},
	{"fish", "genv.fish", "# fish completion for genv"},
}

func TestCompletionInstall_WritesEachShell(t *testing.T) {
	for _, tc := range completionInstallCases {
		t.Run(tc.shell, func(t *testing.T) {
			dir := t.TempDir()
			if code := completionInstallCmd([]string{tc.shell, "--dir", dir}); code != exitOK {
				t.Fatalf("completionInstallCmd(%s) = %d, want exitOK", tc.shell, code)
			}
			got, err := os.ReadFile(filepath.Join(dir, tc.filename))
			if err != nil {
				t.Fatalf("expected %s to be written: %v", tc.filename, err)
			}
			if !strings.Contains(string(got), tc.marker) {
				t.Errorf("installed %s script missing marker %q", tc.shell, tc.marker)
			}
		})
	}
}

// TestCompletionInstall_FlagOrdering verifies the positional shell may appear
// after the --dir flag and that the --dir=value form is honored — the case that
// exposed a Go flag-parsing bug (flag.Parse stops at the first positional).
func TestCompletionInstall_FlagOrdering(t *testing.T) {
	dir := t.TempDir()
	if code := completionInstallCmd([]string{"--dir=" + dir, "zsh"}); code != exitOK {
		t.Fatalf("completionInstallCmd(--dir= then shell) = %d, want exitOK", code)
	}
	if _, err := os.Stat(filepath.Join(dir, "_genv")); err != nil {
		t.Errorf("expected _genv written with flag before positional: %v", err)
	}
}

func TestCompletionInstall_DetectsShellFromEnv(t *testing.T) {
	t.Setenv("SHELL", "/usr/bin/zsh")
	dir := t.TempDir()
	if code := completionInstallCmd([]string{"--dir", dir}); code != exitOK {
		t.Fatalf("completionInstallCmd(no shell arg) = %d, want exitOK", code)
	}
	if _, err := os.Stat(filepath.Join(dir, "_genv")); err != nil {
		t.Errorf("expected zsh completion detected from $SHELL: %v", err)
	}
}

func TestCompletionInstall_UnknownShell(t *testing.T) {
	if code := completionInstallCmd([]string{"tcsh", "--dir", t.TempDir()}); code != exitUsage {
		t.Errorf("completionInstallCmd(tcsh) = %d, want exitUsage", code)
	}
}

func TestCompletionInstall_NoShellNoEnv(t *testing.T) {
	t.Setenv("SHELL", "/usr/bin/nonsense")
	if code := completionInstallCmd([]string{"--dir", t.TempDir()}); code != exitUsage {
		t.Errorf("completionInstallCmd(undetectable shell) = %d, want exitUsage", code)
	}
}

func TestDetectShell(t *testing.T) {
	cases := map[string]string{
		"/bin/bash": "bash", "/usr/bin/zsh": "zsh",
		"/opt/homebrew/bin/fish": "fish", "/bin/sh": "", "": "",
	}
	for shellPath, want := range cases {
		t.Setenv("SHELL", shellPath)
		if got := detectShell(); got != want {
			t.Errorf("detectShell() with SHELL=%q = %q, want %q", shellPath, got, want)
		}
	}
}
