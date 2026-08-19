package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ks1686/genv/internal/adapter"
	"github.com/ks1686/genv/internal/genvfile"
	"github.com/ks1686/genv/internal/schema"
)

func TestResolveEffectiveSpec_V8AndLegacy(t *testing.T) {
	t.Run("nil file", func(t *testing.T) {
		if _, _, err := resolveEffectiveSpec(nil, "", "macos"); err == nil {
			t.Fatal("expected error for nil file")
		}
	})

	t.Run("v8 merge", func(t *testing.T) {
		f := &schema.GenvFile{
			SchemaVersion: schema.Version8,
			Targets: map[string]*schema.TargetBundle{
				"macos": {Packages: []schema.Package{{ID: "git"}}},
			},
		}
		got, id, err := resolveEffectiveSpec(f, "", "macos")
		if err != nil {
			t.Fatalf("resolveEffectiveSpec: %v", err)
		}
		if id != "macos" {
			t.Fatalf("target id = %q, want macos", id)
		}
		if len(got.Packages) != 1 || got.Packages[0].ID != "git" {
			t.Fatalf("packages = %+v, want [{git}]", got.Packages)
		}
	})

	t.Run("v8 missing target", func(t *testing.T) {
		f := &schema.GenvFile{
			SchemaVersion: schema.Version8,
			Targets:       map[string]*schema.TargetBundle{"macos": {Packages: []schema.Package{{ID: "git"}}}},
		}
		_, _, err := resolveEffectiveSpec(f, "", "ubuntu")
		if err == nil || !strings.Contains(err.Error(), "targets.ubuntu") {
			t.Fatalf("err = %v, want missing targets.ubuntu", err)
		}
	})

	t.Run("legacy host filter", func(t *testing.T) {
		f := &schema.GenvFile{
			SchemaVersion: "6",
			Packages: []schema.Package{
				{ID: "git"},
				{ID: "winget-only", Host: schema.HostPredicate{"windows"}},
			},
		}
		got, id, err := resolveEffectiveSpec(f, "macos", "")
		if err != nil {
			t.Fatalf("resolveEffectiveSpec: %v", err)
		}
		if id != "" {
			t.Fatalf("legacy target id = %q, want empty", id)
		}
		if len(got.Packages) != 1 || got.Packages[0].ID != "git" {
			t.Fatalf("packages = %+v, want only git", got.Packages)
		}
	})
}

func TestMaterializeSpecForCommand_ErrorPaths(t *testing.T) {
	t.Run("nil file", func(t *testing.T) {
		var code int
		errOut := captureStderr(t, func() {
			_, _, code = materializeSpecForCommand("status", "genv.json", nil, "", "")
		})
		if code != exitValidation {
			t.Fatalf("code = %d, want %d; stderr=%s", code, exitValidation, errOut)
		}
	})

	t.Run("missing target", func(t *testing.T) {
		f := &schema.GenvFile{
			SchemaVersion: schema.Version8,
			Targets:       map[string]*schema.TargetBundle{"macos": {Packages: []schema.Package{{ID: "git"}}}},
		}
		var code int
		errOut := captureStderr(t, func() {
			_, _, code = materializeSpecForCommand("upgrade", "genv.json", f, "", "ubuntu")
		})
		if code != exitValidation {
			t.Fatalf("code = %d, want %d; stderr=%s", code, exitValidation, errOut)
		}
		if !strings.Contains(errOut, "targets.ubuntu") {
			t.Fatalf("stderr = %q, want targets.ubuntu", errOut)
		}
	})

	t.Run("legacy success", func(t *testing.T) {
		f := &schema.GenvFile{
			SchemaVersion: "6",
			Packages:      []schema.Package{{ID: "git"}},
		}
		got, id, code := materializeSpecForCommand("status", "genv.json", f, "macos", "")
		if code != exitOK || id != "" || len(got.Packages) != 1 {
			t.Fatalf("got packages=%v id=%q code=%d", got.Packages, id, code)
		}
	})
}

// Regression: schemaVersion 8 stores packages under targets.<id>, but status /
// upgrade / updates check historically read only the empty top-level packages
// slice and treated every lock entry as extra / unupgradeable.
func TestStatusUpgradeUpdates_V8TargetPackagesMatchLock(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "xdg"))
	specPath := filepath.Join(dir, "genv.json")
	lockPath := filepath.Join(dir, "genv.lock.json")
	upgradeMarker := filepath.Join(dir, "upgrade.log")

	writeTestFile(t, specPath, `{"schemaVersion":"8","targets":{"macos":{"packages":[{"id":"alpha"}]}}}`)
	writeLock(t, lockPath, []genvfile.LockedPackage{
		{ID: "alpha", Manager: "test-upgrade-no-hooks", PkgName: pkgNameForTest, InstalledVersion: "1.0.0"},
	})

	originalAll := adapter.All
	adapter.All = append([]adapter.Adapter{upgradeNoHooksAdapter{marker: upgradeMarker}}, originalAll...)
	t.Cleanup(func() { adapter.All = originalAll })

	var statusCode int
	statusOut := captureStdout(t, func() {
		statusCode = run([]string{"status", "--file", specPath, "--lock-file", lockPath, "--target", "macos"})
	})
	if statusCode != exitOK {
		t.Fatalf("status v8: expected exitOK (%d), got %d; stdout=%s", exitOK, statusCode, statusOut)
	}
	if strings.Contains(statusOut, "extra") {
		t.Fatalf("status v8 should not report lock packages as extra; stdout=%s", statusOut)
	}

	var upgradeCode int
	upgradeOut := captureStdout(t, func() {
		upgradeCode = run([]string{"upgrade", "--dry-run", "--all", "--file", specPath, "--lock-file", lockPath, "--target", "macos"})
	})
	if upgradeCode != exitOK {
		t.Fatalf("upgrade v8: expected exitOK (%d), got %d; stdout=%s", exitOK, upgradeCode, upgradeOut)
	}
	if strings.Contains(upgradeOut, "no upgradeable packages found") {
		t.Fatalf("upgrade v8 planned nothing; stdout=%s", upgradeOut)
	}
	if !strings.Contains(upgradeOut, "alpha") {
		t.Fatalf("upgrade v8 missing alpha plan; stdout=%s", upgradeOut)
	}

	var updatesCode int
	updatesOut := captureStdout(t, func() {
		updatesCode = run([]string{"updates", "check", "--file", specPath, "--lock-file", lockPath, "--target", "macos"})
	})
	if updatesCode != exitOK {
		t.Fatalf("updates check v8: expected exitOK (%d), got %d; stdout=%s", exitOK, updatesCode, updatesOut)
	}
	if strings.Contains(updatesOut, "no upgradeable genv-tracked packages found") {
		t.Fatalf("updates check v8 planned nothing; stdout=%s", updatesOut)
	}
	if !strings.Contains(updatesOut, "alpha") {
		t.Fatalf("updates check v8 missing alpha plan; stdout=%s", updatesOut)
	}

	if _, err := os.Stat(upgradeMarker); !os.IsNotExist(err) {
		t.Fatalf("dry-run paths must not execute upgrades; marker stat=%v", err)
	}
}

func TestStatusUpgradeUpdates_V8RejectUnknownTargetFlag(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "xdg"))
	specPath := filepath.Join(dir, "genv.json")
	writeTestFile(t, specPath, `{"schemaVersion":"8","targets":{"macos":{"packages":[{"id":"alpha"}]}}}`)

	for _, args := range [][]string{
		{"status", "--file", specPath, "--target", "ubuntu"},
		{"upgrade", "--dry-run", "--file", specPath, "--target", "ubuntu"},
		{"updates", "check", "--file", specPath, "--target", "ubuntu"},
	} {
		var code int
		errOut := captureStderr(t, func() {
			code = run(args)
		})
		if code != exitValidation {
			t.Fatalf("%v: expected exitValidation (%d), got %d; stderr=%s", args, exitValidation, code, errOut)
		}
		if !strings.Contains(errOut, "targets.ubuntu") {
			t.Fatalf("%v: expected missing-target guidance, got stderr=%s", args, errOut)
		}
	}
}

func TestCompletePackages_V8UsesActiveTarget(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "xdg"))
	t.Setenv("GENV_TARGET", "macos")
	specPath := filepath.Join(dir, "genv.json")
	writeTestFile(t, specPath, `{"schemaVersion":"8","targets":{"macos":{"packages":[{"id":"alpha"},{"id":"beta"}]}}}`)

	var code int
	out := captureStdout(t, func() {
		code = completeInternalCmd([]string{"packages", "--file", specPath})
	})
	if code != exitOK {
		t.Fatalf("complete packages: expected exitOK (%d), got %d", exitOK, code)
	}
	if !strings.Contains(out, "alpha") || !strings.Contains(out, "beta") {
		t.Fatalf("complete packages output = %q, want alpha and beta", out)
	}
}

func TestEnvListShellStatusServiceList_V8Materialize(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "xdg"))
	t.Setenv("GENV_TARGET", "macos")
	specPath := filepath.Join(dir, "genv.json")
	lockPath := filepath.Join(dir, "genv.lock.json")
	writeTestFile(t, specPath, `{
		"schemaVersion":"8",
		"defaults":{
			"env":{"EDITOR":{"value":"nvim"}},
			"shell":{"aliases":{"ll":{"value":"ls -la"}}}
		},
		"targets":{
			"macos":{
				"packages":[{"id":"git"}],
				"services":{"pulse":{"start":["true"],"stop":["true"],"status":["true"]}}
			}
		}
	}`)
	writeLock(t, lockPath, nil)

	var code int
	out := captureStdout(t, func() {
		code = run([]string{"env", "list", "--file", specPath, "--target", "macos"})
	})
	if code != exitOK {
		t.Fatalf("env list: code=%d out=%s", code, out)
	}
	if !strings.Contains(out, "EDITOR") {
		t.Fatalf("env list missing EDITOR: %s", out)
	}

	out = captureStdout(t, func() {
		code = run([]string{"shell", "status", "--file", specPath, "--target", "macos"})
	})
	if code != exitOK && code != exitLogic {
		t.Fatalf("shell status: code=%d out=%s", code, out)
	}
	if !strings.Contains(out, "ll") {
		t.Fatalf("shell status missing ll: %s", out)
	}

	out = captureStdout(t, func() {
		code = run([]string{"service", "list", "--file", specPath, "--target", "macos"})
	})
	if code != exitOK {
		t.Fatalf("service list: code=%d out=%s", code, out)
	}
	if !strings.Contains(out, "pulse") {
		t.Fatalf("service list missing pulse: %s", out)
	}
}

func TestAddRemoveHooks_V8Defaults(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "xdg"))
	specPath := filepath.Join(dir, "genv.json")
	lockPath := filepath.Join(dir, "genv.lock.json")
	hookLog := filepath.Join(dir, "hooks.log")
	installLog := filepath.Join(dir, "install.log")
	uninstallLog := filepath.Join(dir, "uninstall.log")
	registerLifecycleHookAdapter(t, lifecycleHookAdapter{installMarker: installLog, uninstallMarker: uninstallLog})
	writeTestFile(t, specPath, `{
		"schemaVersion":"8",
		"defaults":{"hooks":{
			"preAdd":[{`+jsonHook(hookAppend(hookLog, "preadd"))+`}],
			"postAdd":[{`+jsonHook(hookAppend(hookLog, "postadd"))+`}],
			"preRemove":[{`+jsonHook(hookAppend(hookLog, "preremove"))+`}],
			"postRemove":[{`+jsonHook(hookAppend(hookLog, "postremove"))+`}]
		}},
		"targets":{"macos":{"packages":[]}}
	}`)

	var code int
	errOut := captureStderr(t, func() {
		code = run([]string{"add", "--file", specPath, "--lock-file", lockPath, "--target", "macos", "--prefer", "test-hook-manager", "--no-search", "alpha"})
	})
	if code != exitOK {
		t.Fatalf("add: code=%d stderr=%s", code, errOut)
	}
	got, err := os.ReadFile(hookLog)
	if err != nil {
		t.Fatalf("read hook log: %v", err)
	}
	if string(got) != "preaddpostadd" {
		t.Fatalf("add hooks = %q, want preaddpostadd", got)
	}

	if err := os.WriteFile(hookLog, nil, 0o644); err != nil {
		t.Fatalf("truncate hook log: %v", err)
	}
	code = run([]string{"remove", "--file", specPath, "--lock-file", lockPath, "--target", "macos", "alpha"})
	if code != exitOK {
		t.Fatalf("remove: code=%d", code)
	}
	got, err = os.ReadFile(hookLog)
	if err != nil {
		t.Fatalf("read hook log: %v", err)
	}
	if string(got) != "preremovepostremove" {
		t.Fatalf("remove hooks = %q, want preremovepostremove", got)
	}
}
