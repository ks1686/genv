package main

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ks1686/genv/internal/genvfile"
	"github.com/ks1686/genv/internal/service"
)

func TestReportApplyRestartsCoversHumanOutcomes(t *testing.T) {
	outcomes := []restartOutcome{
		{Service: "pending", PendingError: errors.New("receipt failed")},
		{Service: "failed", Err: errors.New("stop failed")},
		{Service: "unready", ReadinessError: errors.New("health failed")},
		{Service: "skip", Action: service.ActionSkip, Reason: "stopped"},
		{Service: "defer", Action: service.ActionDefer, Reason: "unknown version"},
		{Service: "restart", Action: service.ActionRestart},
	}
	var code int
	stderr := captureStderr(t, func() {
		stdout := captureStdout(t, func() { code = reportApplyRestarts(outcomes, "apply: ") })
		if !strings.Contains(stdout, "apply: service restart: restarted") {
			t.Errorf("stdout = %q", stdout)
		}
	})
	if code != exitLogic {
		t.Fatalf("code = %d, want %d", code, exitLogic)
	}
	for _, want := range []string{"receipt failed", "stop failed", "health failed", "stopped", "unknown version"} {
		if !strings.Contains(stderr, want) {
			t.Errorf("stderr missing %q: %q", want, stderr)
		}
	}
}

func TestUpdatesCheckReadSpecErrorExitCodes(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing.json")
	if code := updatesCheckReadSpecError(missing, false, genvfile.ErrNotFound); code != exitIO {
		t.Fatalf("not found = %d", code)
	}
	if code := updatesCheckReadSpecError(missing, false, genvfile.ErrInvalidFile); code != exitValidation {
		t.Fatalf("invalid = %d", code)
	}
	if code := updatesCheckReadSpecError(missing, false, errors.New("disk error")); code != exitIO {
		t.Fatalf("io = %d", code)
	}
	if code := updatesCheckReadSpecError(missing, true, errors.New("json error")); code != exitLogic {
		t.Fatalf("json = %d, want %d", code, exitLogic)
	}
}

func TestReportSpecReadErrorExitCodes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "genv.json")
	if code := reportSpecReadError("config", path, genvfile.ErrNotFound); code != exitIO {
		t.Fatalf("not found = %d", code)
	}
	if code := reportSpecReadError("config", path, genvfile.ErrInvalidFile); code != exitValidation {
		t.Fatalf("invalid = %d", code)
	}
}
