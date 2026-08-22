package adapter

import (
	"errors"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// swapProbeTimeout shortens the probe deadline (and the post-exit pipe-drain
// grace, so killed grandchildren don't pad the test) for the test's duration.
func swapProbeTimeout(t *testing.T, d time.Duration) {
	t.Helper()
	origTimeout := probeTimeout
	origWait := probeWaitDelay
	probeTimeout = d
	probeWaitDelay = d
	t.Cleanup(func() {
		probeTimeout = origTimeout
		probeWaitDelay = origWait
	})
}

// TestRunProbe_TimeoutReclassified is the regression test for the Windows CI
// scan hang: a manager subprocess that blocks past the deadline must surface
// as a timeout error, NOT as a plain *exec.ExitError (CommandContext kills the
// child, which used to look identical to "manager exited with an error").
func TestRunProbe_TimeoutReclassified(t *testing.T) {
	swapProbeTimeout(t, 80*time.Millisecond)
	installFakeBinary(t, "sleep", "sleep 30")

	out, err := runProbe("sleep", "30")
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("error = %v, want it to mention the timeout", err)
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		t.Fatalf("timeout must not be an *exec.ExitError (callers treat that as a manager verdict), got %v", err)
	}
	if out != nil {
		t.Fatalf("expected nil output on timeout, got %q", out)
	}
}

// TestRunProbe_ExitCodeStillSurfaces guards the other side of the contract:
// real manager exit codes (e.g. choco's "2 means outdated found") must still
// arrive as *exec.ExitError so callers can apply their exit-code conventions.
func TestRunProbe_ExitCodeStillSurfaces(t *testing.T) {
	installFakeBinary(t, "fakeprobe", "echo out; exit 3")

	out, err := runProbe("fakeprobe")
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected *exec.ExitError, got %v", err)
	}
	if exitErr.ExitCode() != 3 {
		t.Fatalf("exit code = %d, want 3", exitErr.ExitCode())
	}
	if string(out) != "out\n" {
		t.Fatalf("output = %q, want %q", out, "out\n")
	}
}

// TestRunQuery_TimeoutIsError pins the scan-hang fix at the helper level:
// a timed-out Query must return an error (so callers warn and move on)
// instead of (false, nil), which would silently mean "not installed".
func TestRunQuery_TimeoutIsError(t *testing.T) {
	swapProbeTimeout(t, 80*time.Millisecond)
	installFakeBinary(t, "sleepquery", "sleep 30")

	ok, err := runQuery("sleepquery")
	if ok {
		t.Fatal("ok = true, want false")
	}
	if err == nil {
		t.Fatal("a hung manager must return an error, not a silent not-installed")
	}
}

// TestRunQuery_NonZeroStillMeansAbsent keeps the pre-existing convention.
func TestRunQuery_NonZeroStillMeansAbsent(t *testing.T) {
	installFakeBinary(t, "absentquery", "exit 1")

	ok, err := runQuery("absentquery")
	if ok || err != nil {
		t.Fatalf("got (%v, %v), want (false, nil)", ok, err)
	}
}

// TestRunListOutput_TimeoutIsError covers the ListInstalled path.
func TestRunListOutput_TimeoutIsError(t *testing.T) {
	swapProbeTimeout(t, 80*time.Millisecond)
	installFakeBinary(t, "sleeplist", "sleep 30")

	lines, err := runListOutput("sleeplist")
	if err == nil {
		t.Fatal("expected timeout error from list")
	}
	if lines != nil {
		t.Fatalf("expected nil lines, got %v", lines)
	}
}

// TestRunVersionOutput_TimeoutIsError covers the QueryVersion path.
func TestRunVersionOutput_TimeoutIsError(t *testing.T) {
	swapProbeTimeout(t, 80*time.Millisecond)
	installFakeBinary(t, "sleepver", "sleep 30")

	ver, err := runVersionOutput("sleepver")
	if err == nil {
		t.Fatal("expected timeout error from version probe")
	}
	if ver != "" {
		t.Fatalf("expected empty version, got %q", ver)
	}
}

// TestRunListOutput_FastPathUnchanged makes sure the capping did not disturb
// normal parsing: a manager that prints lines still parses as before.
func TestRunListOutput_FastPathUnchanged(t *testing.T) {
	installFakeBinary(t, "fastlist", "echo one; echo; echo two")

	lines, err := runListOutput("fastlist")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(lines) != 2 || lines[0] != "one" || lines[1] != "two" {
		t.Fatalf("lines = %v, want [one two]", lines)
	}
}
