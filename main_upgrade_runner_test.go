package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
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

func TestUpgrade_JSON_wet_run_without_yes_does_not_execute(t *testing.T) {
	// Given: a tracked package whose upgrade command would write a marker.
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

	// When: wet-run JSON is requested without --yes.
	var code int
	out := captureStdout(t, func() {
		code = run([]string{"upgrade", "--json", "--all", "--no-hooks", "--file", specPath, "--lock-file", lockPath})
	})

	// Then: the command refuses to execute and leaves the marker unwritten.
	if code != exitLogic {
		t.Fatalf("upgrade --json wet without --yes: expected exitLogic (%d), got %d\n%s", exitLogic, code, out)
	}
	if _, err := os.Stat(upgradeMarker); !os.IsNotExist(err) {
		t.Fatalf("wet --json without --yes executed upgrade; marker stat=%v", err)
	}
	var env struct {
		OK     bool     `json:"ok"`
		Errors []string `json:"errors"`
		Data   struct {
			DryRun  bool                  `json:"dryRun"`
			Batches []output.UpgradeBatch `json:"batches"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("json: %v\n%s", err, out)
	}
	if env.OK || env.Data.DryRun {
		t.Fatalf("envelope = %+v, want ok=false dryRun=false", env)
	}
	if len(env.Data.Batches) == 0 || env.Data.Batches[0].Status != "planned" {
		t.Fatalf("batches = %+v, want planned work in the refused envelope", env.Data.Batches)
	}
	joined := strings.Join(env.Errors, "\n")
	if !strings.Contains(joined, "--yes") || !strings.Contains(joined, "--dry-run") {
		t.Fatalf("errors = %v, want --yes and --dry-run guidance", env.Errors)
	}
}

func TestUpgrade_JSON_wet_run_with_yes_executes(t *testing.T) {
	// Given: a tracked package whose upgrade command writes a marker.
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

	// When: wet-run JSON is requested with --yes.
	var code int
	out := captureStdout(t, func() {
		code = run([]string{"upgrade", "--json", "--yes", "--all", "--no-hooks", "--file", specPath, "--lock-file", lockPath})
	})

	// Then: the upgrade runs and the envelope reports success.
	if code != exitOK {
		t.Fatalf("upgrade --json --yes: expected exitOK (%d), got %d\n%s", exitOK, code, out)
	}
	got, err := os.ReadFile(upgradeMarker)
	if err != nil {
		t.Fatalf("read upgrade marker: %v", err)
	}
	if string(got) != "upgrade" {
		t.Fatalf("upgrade marker = %q, want %q", string(got), "upgrade")
	}
	var env struct {
		OK   bool `json:"ok"`
		Data struct {
			DryRun bool `json:"dryRun"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("json: %v\n%s", err, out)
	}
	if !env.OK || env.Data.DryRun {
		t.Fatalf("envelope = %+v, want ok=true dryRun=false", env)
	}
}

func TestUpgrade_PositionalIDs_apply_as_only(t *testing.T) {
	// Given: two tracked packages on a test manager.
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "xdg"))
	specPath := filepath.Join(dir, "genv.json")
	lockPath := filepath.Join(dir, "genv.lock.json")
	if err := os.WriteFile(specPath, []byte(`{"schemaVersion":"5","packages":[{"id":"alpha"},{"id":"beta"}]}`), 0o644); err != nil {
		t.Fatalf("write spec: %v", err)
	}
	writeLock(t, lockPath, []genvfile.LockedPackage{
		{ID: "alpha", Manager: "test-upgrade-no-hooks", PkgName: "alpha", InstalledVersion: "1.0.0"},
		{ID: "beta", Manager: "test-upgrade-no-hooks", PkgName: "beta", InstalledVersion: "1.0.0"},
	})
	originalAll := adapter.All
	adapter.All = append([]adapter.Adapter{upgradeNoHooksAdapter{}}, originalAll...)
	t.Cleanup(func() { adapter.All = originalAll })

	// When: a leftover positional ID is supplied without --only.
	var code int
	out := captureStdout(t, func() {
		code = run([]string{"upgrade", "--dry-run", "--all", "--json", "--file", specPath, "--lock-file", lockPath, "alpha"})
	})

	// Then: the positional ID is treated as --only.
	if code != exitOK {
		t.Fatalf("upgrade alpha: expected exitOK (%d), got %d\n%s", exitOK, code, out)
	}
	var env struct {
		Data struct {
			Batches []output.UpgradeBatch `json:"batches"`
			Filters output.UpgradeFilters `json:"filters"`
			Skipped []output.UpgradeSkipped
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("json: %v\n%s", err, out)
	}
	if len(env.Data.Filters.Only) != 1 || env.Data.Filters.Only[0] != "alpha" {
		t.Fatalf("filters.only = %v, want [alpha]", env.Data.Filters.Only)
	}
	if len(env.Data.Batches) != 1 || len(env.Data.Batches[0].IDs) != 1 || env.Data.Batches[0].IDs[0] != "alpha" {
		t.Fatalf("batches = %+v, want only alpha", env.Data.Batches)
	}
}

func TestUpgrade_PositionalIDs_merge_with_only_flag(t *testing.T) {
	// Given: three tracked packages on a test manager.
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "xdg"))
	specPath := filepath.Join(dir, "genv.json")
	lockPath := filepath.Join(dir, "genv.lock.json")
	if err := os.WriteFile(specPath, []byte(`{"schemaVersion":"5","packages":[{"id":"alpha"},{"id":"beta"},{"id":"gamma"}]}`), 0o644); err != nil {
		t.Fatalf("write spec: %v", err)
	}
	writeLock(t, lockPath, []genvfile.LockedPackage{
		{ID: "alpha", Manager: "test-upgrade-no-hooks", PkgName: "alpha", InstalledVersion: "1.0.0"},
		{ID: "beta", Manager: "test-upgrade-no-hooks", PkgName: "beta", InstalledVersion: "1.0.0"},
		{ID: "gamma", Manager: "test-upgrade-no-hooks", PkgName: "gamma", InstalledVersion: "1.0.0"},
	})
	originalAll := adapter.All
	adapter.All = append([]adapter.Adapter{upgradeNoHooksAdapter{}}, originalAll...)
	t.Cleanup(func() { adapter.All = originalAll })

	// When: --only and leftover positionals are combined.
	var code int
	out := captureStdout(t, func() {
		code = run([]string{"upgrade", "--dry-run", "--all", "--json", "--only", "alpha", "--file", specPath, "--lock-file", lockPath, "beta"})
	})

	// Then: both the flag and the positional IDs are selected.
	if code != exitOK {
		t.Fatalf("upgrade --only alpha beta: expected exitOK (%d), got %d\n%s", exitOK, code, out)
	}
	var env struct {
		Data struct {
			Batches []output.UpgradeBatch `json:"batches"`
			Filters output.UpgradeFilters `json:"filters"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("json: %v\n%s", err, out)
	}
	if !slices.Equal(env.Data.Filters.Only, []string{"alpha", "beta"}) {
		t.Fatalf("filters.only = %v, want [alpha beta]", env.Data.Filters.Only)
	}
	var gotIDs []string
	for _, b := range env.Data.Batches {
		gotIDs = append(gotIDs, b.IDs...)
	}
	if !slices.Equal(gotIDs, []string{"alpha", "beta"}) {
		t.Fatalf("batch IDs = %v, want [alpha beta]", gotIDs)
	}
}

func TestUpgrade_Yes_with_absent_os_tools_still_runs_tracked_hooks(t *testing.T) {
	orig := upgradeLookPath
	upgradeLookPath = func(string) (string, error) { return "", exec.ErrNotFound }
	t.Cleanup(func() { upgradeLookPath = orig })

	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "xdg"))
	specPath := filepath.Join(dir, "genv.json")
	lockPath := filepath.Join(dir, "genv.lock.json")
	marker := filepath.Join(dir, "hook.log")
	spec := `{"schemaVersion":"5","packages":[{"id":"git"}],"hooks":{"preUpgrade":[{` + jsonHook(hookAppend(marker, "pre")) + `}],"postUpgrade":[{` + jsonHook(hookAppend(marker, "post")) + `}]}}`
	if err := os.WriteFile(specPath, []byte(spec), 0o644); err != nil {
		t.Fatalf("write spec: %v", err)
	}
	writeLock(t, lockPath, []genvfile.LockedPackage{{ID: "git", Manager: "missing-manager", PkgName: "git"}})

	var code int
	out := captureStdout(t, func() {
		code = run([]string{"upgrade", "--file", specPath, "--lock-file", lockPath, "--yes"})
	})
	if code != exitOK {
		t.Fatalf("upgrade --yes with absent OS tools: expected exitOK (%d), got %d\n%s", exitOK, code, out)
	}
	if !strings.Contains(out, "system: skipped") {
		t.Fatalf("expected system step skipped, got:\n%s", out)
	}
	got, err := os.ReadFile(marker)
	if err != nil {
		t.Fatalf("read hook marker: %v", err)
	}
	if string(got) != "prepost" {
		t.Fatalf("hook marker: got %q, want %q", string(got), "prepost")
	}
}
