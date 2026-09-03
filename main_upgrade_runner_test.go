package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ks1686/genv/internal/adapter"
	"github.com/ks1686/genv/internal/genvfile"
	"github.com/ks1686/genv/internal/output"
)

func TestUpgrade_DryRun_lists_system_or_firmware_steps(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "xdg"))
	specPath := filepath.Join(dir, "genv.json")
	lockPath := filepath.Join(dir, "genv.lock.json")
	upgradeMarker := filepath.Join(dir, "upgrade.log")
	if err := os.WriteFile(specPath, []byte(`{"schemaVersion":"5","packages":[{"id":"alpha"}]}`), 0o644); err != nil {
		t.Fatalf("write spec: %v", err)
	}
	writeLock(t, lockPath, []genvfile.LockedPackage{{ID: "alpha", Manager: "test-upgrade-no-hooks", PkgName: pkgNameForTest, InstalledVersion: "1.0.0"}})
	originalAll := adapter.All
	adapter.All = append([]adapter.Adapter{upgradeNoHooksAdapter{marker: upgradeMarker}}, originalAll...)
	t.Cleanup(func() { adapter.All = originalAll })

	var code int
	out := captureStdout(t, func() {
		code = run([]string{"upgrade", "--dry-run", "--all", "--file", specPath, "--lock-file", lockPath})
	})
	if code != exitOK {
		t.Fatalf("upgrade --dry-run: expected exitOK (%d), got %d\n%s", exitOK, code, out)
	}
	if !strings.Contains(out, "alpha") {
		t.Fatalf("tracked package missing from plan:\n%s", out)
	}
	if !strings.Contains(out, "  system  ==>") && !strings.Contains(out, "  system: skipped") {
		t.Fatalf("system step missing from upgrade plan:\n%s", out)
	}
	if !strings.Contains(out, "  firmware  ==>") && !strings.Contains(out, "  firmware: skipped") {
		t.Fatalf("firmware step missing from upgrade plan:\n%s", out)
	}
	if _, err := os.Stat(upgradeMarker); !os.IsNotExist(err) {
		t.Fatalf("dry-run executed tracked upgrade; marker stat=%v", err)
	}
}

func TestUpdatesCheck_stays_tracked_packages_only(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "xdg"))
	specPath := filepath.Join(dir, "genv.json")
	lockPath := filepath.Join(dir, "genv.lock.json")
	upgradeMarker := filepath.Join(dir, "upgrade.log")
	if err := os.WriteFile(specPath, []byte(`{"schemaVersion":"5","packages":[{"id":"alpha"}]}`), 0o644); err != nil {
		t.Fatalf("write spec: %v", err)
	}
	writeLock(t, lockPath, []genvfile.LockedPackage{{ID: "alpha", Manager: "test-upgrade-no-hooks", PkgName: pkgNameForTest, InstalledVersion: "1.0.0"}})
	originalAll := adapter.All
	adapter.All = append([]adapter.Adapter{upgradeNoHooksAdapter{marker: upgradeMarker}}, originalAll...)
	t.Cleanup(func() { adapter.All = originalAll })

	var code int
	out := captureStdout(t, func() {
		code = run([]string{"updates", "check", "--file", specPath, "--lock-file", lockPath})
	})
	if code != exitOK {
		t.Fatalf("updates check: expected exitOK (%d), got %d\n%s", exitOK, code, out)
	}
	if !strings.Contains(out, "genv-tracked packages only") || !strings.Contains(out, "alpha") {
		t.Fatalf("updates check lost tracked plan:\n%s", out)
	}
	if strings.Contains(out, "  system  ==>") || strings.Contains(out, "  system: skipped") ||
		strings.Contains(out, "firmware") || strings.Contains(out, "softwareupdate") ||
		strings.Contains(out, "pacman -Syu") || strings.Contains(out, "fwupdmgr") {
		t.Fatalf("updates check listed OS/firmware steps:\n%s", out)
	}
	if _, err := os.Stat(upgradeMarker); !os.IsNotExist(err) {
		t.Fatalf("updates check executed an upgrade; marker stat=%v", err)
	}
}

func TestUpgrade_JSON_DryRun_includes_steps_keeps_tracked_batches(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "xdg"))
	specPath := filepath.Join(dir, "genv.json")
	lockPath := filepath.Join(dir, "genv.lock.json")
	if err := os.WriteFile(specPath, []byte(`{"schemaVersion":"5","packages":[{"id":"alpha"}]}`), 0o644); err != nil {
		t.Fatalf("write spec: %v", err)
	}
	writeLock(t, lockPath, []genvfile.LockedPackage{{ID: "alpha", Manager: "test-upgrade-no-hooks", PkgName: pkgNameForTest, InstalledVersion: "1.0.0"}})
	originalAll := adapter.All
	adapter.All = append([]adapter.Adapter{upgradeNoHooksAdapter{}}, originalAll...)
	t.Cleanup(func() { adapter.All = originalAll })

	var code int
	out := captureStdout(t, func() {
		code = run([]string{"upgrade", "--json", "--dry-run", "--all", "--file", specPath, "--lock-file", lockPath})
	})
	if code != exitOK {
		t.Fatalf("upgrade --json --dry-run: expected exitOK (%d), got %d\n%s", exitOK, code, out)
	}
	var env struct {
		OK   bool `json:"ok"`
		Data struct {
			DryRun  bool                  `json:"dryRun"`
			Batches []output.UpgradeBatch `json:"batches"`
			Steps   []output.UpgradeStep  `json:"steps"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("json: %v\n%s", err, out)
	}
	if !env.OK || !env.Data.DryRun {
		t.Fatalf("envelope = %+v", env)
	}
	if len(env.Data.Batches) == 0 || env.Data.Batches[0].IDs[0] != "alpha" {
		t.Fatalf("tracked batches = %+v", env.Data.Batches)
	}
	if len(env.Data.Steps) < 2 {
		t.Fatalf("steps = %+v, want system and firmware", env.Data.Steps)
	}
	names := env.Data.Steps[0].Name + " " + env.Data.Steps[1].Name
	if !strings.Contains(names, "system") || !strings.Contains(names, "firmware") {
		t.Fatalf("step names = %q", names)
	}
}
