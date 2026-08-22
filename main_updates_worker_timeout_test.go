package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ks1686/genv/internal/genvfile"
	"github.com/ks1686/genv/internal/resolver"
	"github.com/ks1686/genv/internal/testutil"
	"github.com/ks1686/genv/internal/upgrade"
)

// swapUpdatesRunUpgrade replaces the RunUpgrade seam for a test.
func swapUpdatesRunUpgrade(t *testing.T, fn func(ctx context.Context, opts upgrade.UpgradeRunOptions) upgrade.UpgradeRunResult) {
	t.Helper()
	orig := updatesRunUpgrade
	updatesRunUpgrade = fn
	t.Cleanup(func() { updatesRunUpgrade = orig })
}

// writeUpdatesSpec seeds a minimal auto-apply v6 spec + lock and returns paths.
func writeUpdatesSpec(t *testing.T) (specPath, lockPath string) {
	t.Helper()
	dir := t.TempDir()
	specPath = filepath.Join(dir, "genv.json")
	lockPath = filepath.Join(dir, "genv.lock.json")
	spec := `{"schemaVersion":"6","packages":[{"id":"alpha"}],"updates":{"enabled":true,"interval":"1h","autoApply":true}}`
	if err := os.WriteFile(specPath, []byte(spec), 0o644); err != nil {
		t.Fatalf("write spec: %v", err)
	}
	writeLock(t, lockPath, nil)
	return specPath, lockPath
}

// TestUpdatesRunOnce_TimeoutWaitsForInFlightUpgrade: when the scheduled budget
// expires mid-upgrade, the worker must wait briefly for the in-flight run to
// finish instead of exiting immediately — an early exit kills package-manager
// subprocesses mid-transaction (half-installed packages, broken databases).
func TestUpdatesRunOnce_TimeoutWaitsForInFlightUpgrade(t *testing.T) {
	if testing.Short() {
		t.Skip("timing-sensitive test")
	}
	specPath, lockPath := writeUpdatesSpec(t)

	// Shrink both budgets: the job timeout drives when cancellation fires,
	// the shutdown grace bounds how long the worker waits afterwards.
	origTimeout := updatesJobTimeout
	updatesJobTimeout = func(time.Duration) time.Duration { return 200 * time.Millisecond }
	t.Cleanup(func() { updatesJobTimeout = origTimeout })
	origGrace := updatesShutdownGrace
	updatesShutdownGrace = 500 * time.Millisecond
	t.Cleanup(func() { updatesShutdownGrace = origGrace })

	upgradeStarted := make(chan struct{})
	releaseUpgrade := make(chan struct{})
	swapUpdatesRunUpgrade(t, func(ctx context.Context, opts upgrade.UpgradeRunOptions) upgrade.UpgradeRunResult {
		close(upgradeStarted)
		<-releaseUpgrade
		return upgrade.UpgradeRunResult{Plan: opts.Plan}
	})

	go func() {
		<-upgradeStarted
		time.Sleep(100 * time.Millisecond)
		close(releaseUpgrade)
	}()

	code := run([]string{"updates", "__run-once", "--file", specPath, "--lock-file", lockPath})
	if code != exitOK {
		t.Fatalf("expected exitOK after the in-flight run settled within the grace window, got %d", code)
	}
}

// TestUpdatesRunOnce_NotificationCompletesBeforeLogCloses: the async desktop
// notification must finish before the worker exits — otherwise it races the
// closed log file and its failures vanish. A fake notifier that sleeps before
// writing a marker proves the worker waited: the marker must exist by the
// time run() returns.
func TestUpdatesRunOnce_NotificationCompletesBeforeLogCloses(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "notified")
	specPath := filepath.Join(dir, "genv.json")
	lockPath := filepath.Join(dir, "genv.lock.json")
	spec := `{"schemaVersion":"6","packages":[{"id":"alpha"}],"updates":{"enabled":true,"interval":"1h","autoApply":false,"notify":true}}`
	if err := os.WriteFile(specPath, []byte(spec), 0o644); err != nil {
		t.Fatalf("write spec: %v", err)
	}
	writeLock(t, lockPath, nil)

	// Fake binaries for BOTH notifier branches (LookPath only checks
	// existence, so shadowing launchctl isn't enough to force the notify-send
	// branch on darwin): whichever branch runs, the fake sleeps before
	// dropping the marker. The marker path is normalized to forward slashes
	// because /bin/sh eats backslashes (Windows Temp paths).
	markerSh := strings.ReplaceAll(marker, "\\", "/")
	testutil.InstallFakeBinary(t, "launchctl", "exit 1")
	testutil.InstallFakeBinary(t, "notify-send", "sleep 0.4\ntouch '"+markerSh+"'")
	testutil.InstallFakeBinary(t, "osascript", "sleep 0.4\ntouch '"+markerSh+"'")

	// The real planner reports nothing outdated against the fakes; stub a
	// one-package plan so the check-only path actually notifies.
	origPlan := updatesBuildPlan
	updatesBuildPlan = func(opts upgrade.UpgradeOptions) (upgrade.UpgradePlan, error) {
		return upgrade.UpgradePlan{Actions: []resolver.UpgradeAction{
			{LPs: []genvfile.LockedPackage{{ID: "alpha", Manager: "brew", PkgName: "alpha"}}},
		}}, nil
	}
	t.Cleanup(func() { updatesBuildPlan = origPlan })

	code := run([]string{"updates", "__run-once", "--file", specPath, "--lock-file", lockPath})
	if code != exitOK {
		t.Fatalf("updates __run-once: expected exitOK (%d), got %d", exitOK, code)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatal("worker exited before the notification goroutine finished; notifications can race the closing log file")
	}
}
