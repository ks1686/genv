package service

import (
	"runtime"
	"strings"
	"testing"

	"github.com/ks1686/genv/internal/testutil"
)

func TestIsBrewServicesAvailable_NonDarwin(t *testing.T) {
	if runtime.GOOS == "darwin" {
		t.Skip("skipping non-darwin test on macOS")
	}
	if IsBrewServicesAvailable() {
		t.Error("IsBrewServicesAvailable() should return false on non-darwin platforms")
	}
}

func TestIsBrewServicesAvailable_Darwin(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("skipping darwin-only test")
	}
	// On macOS in CI or dev, brew may or may not be installed.
	// We just ensure the function doesn't panic and returns a bool.
	_ = IsBrewServicesAvailable()
}

func TestBrewServicesRunning_NoBrew(t *testing.T) {
	if runtime.GOOS != "darwin" {
		// On non-darwin, brew is not available so BrewServicesRunning should return false.
		result := BrewServicesRunning("some-formula")
		if result {
			t.Error("BrewServicesRunning() should return false when brew is unavailable")
		}
	}
}

func TestBrewServicesRunning_UnknownFormula(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("skipping darwin-only test")
	}
	if !IsBrewServicesAvailable() {
		t.Skip("brew not available")
	}
	// A formula that is certainly not a running service.
	if BrewServicesRunning("genv-nonexistent-formula-xyz") {
		t.Error("BrewServicesRunning() should return false for unknown formula")
	}
}

// TestBrewServicesList shadows brew with a fake rather than calling the real one.
//
// The previous version ran `brew services list` on the developer's machine and
// asserted the output was non-empty, so it failed on any Mac with no brew
// services registered and passed on any Mac that happened to have some. That is
// a test of the machine, not of the code, and it made `make ci` red on a clean
// checkout.
func TestBrewServicesList(t *testing.T) {
	testutil.InstallFakeBinary(t, "brew", `case "$1 $2" in
"services list") printf 'Name Status User File\nfoo started user /opt/homebrew/var/log/foo.log\n' ;;
*) exit 1 ;;
esac`)
	out, err := BrewServicesList()
	if err != nil {
		t.Fatalf("BrewServicesList() returned error: %v", err)
	}
	if !strings.Contains(out, "foo") {
		t.Errorf("BrewServicesList() = %q, want the fake service row", out)
	}
}

func TestBrewServicesList_reports_command_failure(t *testing.T) {
	testutil.InstallFakeBinary(t, "brew", "exit 1")
	if _, err := BrewServicesList(); err == nil {
		t.Error("BrewServicesList() should report an error when brew fails")
	}
}
