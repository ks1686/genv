package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ks1686/genv/internal/adapter"
	"github.com/ks1686/genv/internal/genvfile"
	"github.com/ks1686/genv/internal/upgrade"
)

func TestUpdatesRunOnce_SkipNoMatch_logsConfigDriftAtWarn(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "xdg"))
	specPath := filepath.Join(dir, "genv.json")
	lockPath := filepath.Join(dir, "genv.lock.json")
	spec := `{"schemaVersion":"6","packages":[{"id":"alpha"}],"updates":{"enabled":true,"interval":"1h","autoApply":false}}`
	if err := os.WriteFile(specPath, []byte(spec), 0o644); err != nil {
		t.Fatal(err)
	}
	writeLock(t, lockPath, []genvfile.LockedPackage{{ID: "alpha", Manager: "brew", PkgName: "alpha", InstalledVersion: "1.0.0"}})

	origPlan := updatesBuildPlan
	updatesBuildPlan = func(opts upgrade.UpgradeOptions) (upgrade.UpgradePlan, error) {
		return upgrade.UpgradePlan{
			Warnings: []string{`config-drift: --skip filter "docker-desktop" matched no tracked packages`},
		}, nil
	}
	t.Cleanup(func() { updatesBuildPlan = origPlan })

	code := run([]string{"updates", "__run-once", "--file", specPath, "--lock-file", lockPath})
	if code != exitOK {
		t.Fatalf("code = %d, want %d", code, exitOK)
	}

	logPath := filepath.Join(dir, "xdg", "genv", "updates.log")
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read updates.log: %v", err)
	}
	got := string(data)
	if !strings.Contains(got, "level=WARN") || !strings.Contains(got, "msg=updates.check.warning") {
		t.Fatalf("updates.log = %q, want WARN for unmatched skip", got)
	}
	if strings.Contains(got, "level=INFO msg=updates.check.warning") {
		t.Fatalf("updates.log = %q, unmatched skip must not be INFO", got)
	}
	if !strings.Contains(got, "kind=config-drift") {
		t.Fatalf("updates.log = %q, want kind=config-drift", got)
	}
	if !strings.Contains(got, "docker-desktop") {
		t.Fatalf("updates.log = %q, want skip filter name", got)
	}
}

func TestUpdatesCheck_SkipNoMatch_printsConfigDriftOnStderr(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "xdg"))
	specPath := filepath.Join(dir, "genv.json")
	lockPath := filepath.Join(dir, "genv.lock.json")
	if err := os.WriteFile(specPath, []byte(`{"schemaVersion":"5","packages":[{"id":"alpha"}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	writeLock(t, lockPath, []genvfile.LockedPackage{{ID: "alpha", Manager: "test-upgrade-no-hooks", PkgName: pkgNameForTest, InstalledVersion: "1.0.0"}})
	originalAll := adapter.All
	adapter.All = append([]adapter.Adapter{upgradeNoHooksAdapter{marker: filepath.Join(dir, "upgrade.log")}}, originalAll...)
	t.Cleanup(func() { adapter.All = originalAll })

	var code int
	errOut := captureStderr(t, func() {
		code = run([]string{"updates", "check", "--file", specPath, "--lock-file", lockPath, "--skip", "docker-desktop"})
	})
	if code != exitOK {
		t.Fatalf("updates check: expected exitOK (%d), got %d\nstderr: %s", exitOK, code, errOut)
	}
	if !strings.Contains(errOut, "config-drift") {
		t.Fatalf("stderr = %q, want config-drift on unmatched skip", errOut)
	}
	if !strings.Contains(errOut, "docker-desktop") || !strings.Contains(errOut, "matched no tracked packages") {
		t.Fatalf("stderr = %q, want skip filter name and no-match reason", errOut)
	}
}
