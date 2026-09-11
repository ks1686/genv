package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/ks1686/genv/internal/genvfile"
	"github.com/ks1686/genv/internal/upgrade"
)

func TestUpdatesRunOnce_marksPlanAndApplyUnattended(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "xdg"))
	specPath := filepath.Join(dir, "genv.json")
	lockPath := filepath.Join(dir, "genv.lock.json")
	spec := `{"schemaVersion":"6","packages":[{"id":"alpha"}],"updates":{"enabled":true,"interval":"1h","autoApply":true}}`
	if err := os.WriteFile(specPath, []byte(spec), 0o644); err != nil {
		t.Fatal(err)
	}
	writeLock(t, lockPath, []genvfile.LockedPackage{{ID: "alpha", Manager: "brew", PkgName: "alpha", InstalledVersion: "1.0.0"}})

	origPlan := updatesBuildPlan
	origRun := updatesRunUpgrade
	t.Cleanup(func() {
		updatesBuildPlan = origPlan
		updatesRunUpgrade = origRun
	})
	var gotPlan upgrade.UpgradeOptions
	var gotRun upgrade.UpgradeRunOptions
	updatesBuildPlan = func(opts upgrade.UpgradeOptions) (upgrade.UpgradePlan, error) {
		gotPlan = opts
		return upgrade.UpgradePlan{}, nil
	}
	updatesRunUpgrade = func(_ context.Context, opts upgrade.UpgradeRunOptions) upgrade.UpgradeRunResult {
		gotRun = opts
		return upgrade.UpgradeRunResult{Plan: opts.Plan}
	}

	code := run([]string{"updates", "__run-once", "--file", specPath, "--lock-file", lockPath})
	if code != exitOK {
		t.Fatalf("code = %d, want %d", code, exitOK)
	}
	if !gotPlan.Unattended {
		t.Fatal("BuildUpgradePlan must run unattended for __run-once")
	}
	if !gotRun.Unattended {
		t.Fatal("RunUpgrade must run unattended for __run-once autoApply")
	}
}
