package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The suite must not inherit the developer's real config.
//
// TestMain redirects XDG_CONFIG_HOME to a temp dir unconditionally. It used to
// do that only when the variable was unset, so on a machine where it is set —
// a common setup — every command resolving its default spec path wrote to the
// real ~/.config/genv, and an ordinary `go test ./...` ran the unattended
// updates worker against the live config.
//
// Scope: this covers the ambient environment, which is the case that happened.
// A test that explicitly calls t.Setenv with a real path still wins, because
// that is a deliberate act by the test author, not an accident of the
// environment.
func TestHarness_redirects_config_root_away_from_home(t *testing.T) {
	xdg := os.Getenv("XDG_CONFIG_HOME")
	if xdg == "" {
		t.Fatal("XDG_CONFIG_HOME should be set by TestMain")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("no home directory: %v", err)
	}
	realConfig := filepath.Join(home, ".config")
	if sameDir(xdg, realConfig) {
		t.Fatalf("XDG_CONFIG_HOME still points at the real config dir: %s", xdg)
	}

	spec := defaultSpecPath()
	if strings.HasPrefix(spec, realDirPrefix(xdg)) && sameDir(filepath.Dir(spec), realConfig) {
		t.Fatalf("default spec path resolves into the real config dir: %s", spec)
	}
	if !strings.HasPrefix(spec, xdg) {
		t.Fatalf("default spec path %s is not under the test config root %s", spec, xdg)
	}
}

func sameDir(a, b string) bool {
	ra, err := filepath.EvalSymlinks(a)
	if err != nil {
		ra = a
	}
	rb, err := filepath.EvalSymlinks(b)
	if err != nil {
		rb = b
	}
	return filepath.Clean(ra) == filepath.Clean(rb)
}

func realDirPrefix(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	return abs + string(filepath.Separator)
}

// Service reconciliation writes supervisor agents under the home directory and
// validates them by reading it back. A test that applied a service was
// therefore loading a real launchd job on the developer's machine; the leftover
// plist then failed the next `genv validate` with a dangling
// ProgramArguments[0], which reads like a product bug and is not one.
func TestHarness_redirects_home_away_from_the_developer(t *testing.T) {
	home := os.Getenv("HOME")
	if home == "" {
		home = os.Getenv("USERPROFILE")
	}
	if home == "" {
		t.Fatal("neither HOME nor USERPROFILE is set; TestMain should set both")
	}
	if os.Getenv("USERPROFILE") != home {
		t.Errorf("USERPROFILE = %q, want the same temp home as HOME (%q)", os.Getenv("USERPROFILE"), home)
	}
	if harnessRealHome != "" && sameDir(home, harnessRealHome) {
		t.Fatalf("$HOME is still the developer's home (%s); a test could install a supervisor agent", home)
	}
	// The directory genv writes macOS launch agents into must sit under the
	// redirected home, so it lands in the temp dir rather than ~/Library.
	agents := filepath.Join(home, "Library", "LaunchAgents")
	if !strings.HasPrefix(agents, home) {
		t.Errorf("launch agent path %s escaped the test home %s", agents, home)
	}
}
