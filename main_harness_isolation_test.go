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
