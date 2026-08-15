package testutil

import (
	"os"
	"testing"
)

func TestSetHome(t *testing.T) {
	dir := t.TempDir()
	SetHome(t, dir)
	if os.Getenv("HOME") != dir {
		t.Fatalf("HOME = %q, want %q", os.Getenv("HOME"), dir)
	}
	if os.Getenv("USERPROFILE") != dir {
		t.Fatalf("USERPROFILE = %q, want %q", os.Getenv("USERPROFILE"), dir)
	}
}

func TestInstallFakeBinary(t *testing.T) {
	InstallFakeBinary(t, "true-ish", "exit 0")
}
