package service

import (
	"runtime"
	"testing"
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

func TestBrewServicesList(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("skipping darwin-only test")
	}
	if !IsBrewServicesAvailable() {
		t.Skip("brew not available")
	}
	out, err := BrewServicesList()
	if err != nil {
		t.Fatalf("BrewServicesList() returned error: %v", err)
	}
	// Output should at minimum contain the header line.
	if len(out) == 0 {
		t.Error("BrewServicesList() returned empty output")
	}
}
