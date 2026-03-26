package env_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ks1686/genv/internal/env"
)

func TestFragmentPath(t *testing.T) {
	t.Run("with XDG_CONFIG_HOME", func(t *testing.T) {
		xdgDir := "/tmp/myxdg"
		t.Setenv("XDG_CONFIG_HOME", xdgDir)

		got, err := env.FragmentPath()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := filepath.Join(xdgDir, "genv", "env.sh")
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("without XDG_CONFIG_HOME", func(t *testing.T) {
		t.Setenv("XDG_CONFIG_HOME", "")
		home, err := os.UserHomeDir()
		if err != nil {
			t.Skipf("skipping test: os.UserHomeDir failed: %v", err)
		}

		got, err := env.FragmentPath()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := filepath.Join(home, ".config", "genv", "env.sh")
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})
}
