package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ks1686/genv/internal/genvfile"
)

func TestApply_LifecycleHooks_receive_spec_and_lock_env(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "xdg"))
	specPath := filepath.Join(dir, "genv.json")
	lockPath := filepath.Join(dir, "genv.lock.json")
	hookLog := filepath.Join(dir, "hooks.log")
	installLog := filepath.Join(dir, "install.log")
	registerLifecycleHookAdapter(t, lifecycleHookAdapter{installMarker: installLog})
	spec := `{"schemaVersion":"6","packages":[{"id":"alpha","prefer":"test-hook-manager"}],"hooks":{"preApply":[{` + jsonHook(hookPrintEnvLine(hookLog, "GENV_SPEC_FILE", "GENV_SPEC_DIR", "GENV_LOCK_FILE")) + `}],"postApply":[{` + jsonHook(hookPrintEnvLine(hookLog, "GENV_SPEC_FILE", "GENV_SPEC_DIR", "GENV_LOCK_FILE")) + `}]}}`
	if err := os.WriteFile(specPath, []byte(spec), 0o644); err != nil {
		t.Fatalf("write spec: %v", err)
	}

	code := run([]string{"apply", "--file", specPath, "--lock-file", lockPath, "--host", "ci", "--yes", "--hook-timeout", "1s"})

	if code != exitOK {
		t.Fatalf("apply spec/lock hook env: expected exitOK (%d), got %d", exitOK, code)
	}
	got, err := os.ReadFile(hookLog)
	if err != nil {
		t.Fatalf("read hook log: %v", err)
	}
	wantLine := specPath + ":" + dir + ":" + lockPath + "\n"
	want := wantLine + wantLine
	if normalizeHookLog(got) != want {
		t.Fatalf("hook log = %q, want %q", got, want)
	}
}

func TestApply_ContinueOnError_reports_but_does_not_fail_apply(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "xdg"))
	specPath := filepath.Join(dir, "genv.json")
	lockPath := filepath.Join(dir, "genv.lock.json")
	hookLog := filepath.Join(dir, "hooks.log")
	installLog := filepath.Join(dir, "install.log")
	registerLifecycleHookAdapter(t, lifecycleHookAdapter{installMarker: installLog})
	failHook := `"name":"diag","continueOnError":true,` + jsonHook(hookAppendFail(hookLog, "diag"))
	okHook := jsonHook(hookAppend(hookLog, "ok"))
	spec := `{"schemaVersion":"6","packages":[{"id":"alpha","prefer":"test-hook-manager"}],"hooks":{"postApply":[{` + failHook + `},{` + okHook + `}]}}`
	if err := os.WriteFile(specPath, []byte(spec), 0o644); err != nil {
		t.Fatalf("write spec: %v", err)
	}

	var code int
	errOut := captureStderr(t, func() {
		code = run([]string{"apply", "--file", specPath, "--lock-file", lockPath, "--yes", "--hook-timeout", "1s"})
	})

	if code != exitOK {
		t.Fatalf("apply continueOnError: expected exitOK (%d), got %d; stderr=%s", exitOK, code, errOut)
	}
	got, err := os.ReadFile(hookLog)
	if err != nil {
		t.Fatalf("read hook log: %v", err)
	}
	if !strings.Contains(normalizeHookLog(got), "diag") || !strings.Contains(normalizeHookLog(got), "ok") {
		t.Fatalf("hook log = %q, want both diag and ok", got)
	}
	if !strings.Contains(errOut, "diag") && !strings.Contains(errOut, "exit") {
		t.Fatalf("stderr should report the failed hook, got: %s", errOut)
	}
}

func TestApply_HookSummary_lists_name_exit_and_duration(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "xdg"))
	specPath := filepath.Join(dir, "genv.json")
	lockPath := filepath.Join(dir, "genv.lock.json")
	installLog := filepath.Join(dir, "install.log")
	registerLifecycleHookAdapter(t, lifecycleHookAdapter{installMarker: installLog})
	longCmd := "true " + strings.Repeat("n", 50)
	spec := `{"schemaVersion":"6","packages":[{"id":"alpha","prefer":"test-hook-manager"}],"hooks":{"postApply":[{"name":"selftest","command":"true"},{"command":` + jsonString(longCmd) + `}]}}`
	if err := os.WriteFile(specPath, []byte(spec), 0o644); err != nil {
		t.Fatalf("write spec: %v", err)
	}

	var code int
	out := captureStdout(t, func() {
		code = run([]string{"apply", "--file", specPath, "--lock-file", lockPath, "--yes", "--hook-timeout", "1s"})
	})

	if code != exitOK {
		t.Fatalf("apply hook summary: expected exitOK (%d), got %d; stdout=%s", exitOK, code, out)
	}
	if !strings.Contains(out, "selftest") {
		t.Fatalf("summary missing named hook, stdout=%s", out)
	}
	if !strings.Contains(out, longCmd[:40]) {
		t.Fatalf("summary missing truncated command, stdout=%s", out)
	}
	if !strings.Contains(out, "exit 0") && !strings.Contains(out, "exit=0") {
		t.Fatalf("summary missing exit code, stdout=%s", out)
	}
}

func TestApply_NoHooks_skips_hooks_but_runs_primary_op(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "xdg"))
	specPath := filepath.Join(dir, "genv.json")
	lockPath := filepath.Join(dir, "genv.lock.json")
	hookLog := filepath.Join(dir, "hook.log")
	installLog := filepath.Join(dir, "install.log")
	registerLifecycleHookAdapter(t, lifecycleHookAdapter{installMarker: installLog})

	spec := `{"schemaVersion":"6","packages":[{"id":"alpha","prefer":"test-hook-manager"}],"hooks":{"preApply":[{` + jsonHook(hookAppendFail(hookLog, "pre")) + `}],"postApply":[{` + jsonHook(hookAppendFail(hookLog, "post")) + `}]}}`
	if err := os.WriteFile(specPath, []byte(spec), 0o644); err != nil {
		t.Fatalf("write spec: %v", err)
	}

	code := run([]string{"apply", "--file", specPath, "--lock-file", lockPath, "--yes", "--no-hooks"})

	if code != exitOK {
		t.Fatalf("apply --no-hooks: expected exitOK (%d), got %d", exitOK, code)
	}
	if _, err := os.Stat(hookLog); !os.IsNotExist(err) {
		t.Fatalf("hook log stat = %v, want not exist", err)
	}
	got, err := os.ReadFile(installLog)
	if err != nil {
		t.Fatalf("read install log: %v", err)
	}
	if string(got) != "install" {
		t.Fatalf("install log = %q, want install", string(got))
	}
}

func TestRemove_NoHooks_skips_hooks_but_runs_primary_op(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "xdg"))
	specPath := filepath.Join(dir, "genv.json")
	lockPath := filepath.Join(dir, "genv.lock.json")
	hookLog := filepath.Join(dir, "hook.log")
	uninstallLog := filepath.Join(dir, "uninstall.log")
	registerLifecycleHookAdapter(t, lifecycleHookAdapter{uninstallMarker: uninstallLog})

	spec := `{"schemaVersion":"6","packages":[{"id":"alpha","prefer":"test-hook-manager"}],"hooks":{"preRemove":[{` + jsonHook(hookAppendFail(hookLog, "pre")) + `}],"postRemove":[{` + jsonHook(hookAppendFail(hookLog, "post")) + `}]}}`
	if err := os.WriteFile(specPath, []byte(spec), 0o644); err != nil {
		t.Fatalf("write spec: %v", err)
	}
	writeLock(t, lockPath, []genvfile.LockedPackage{{ID: "alpha", Manager: "test-hook-manager", PkgName: "alpha"}})

	code := run([]string{"remove", "--file", specPath, "--lock-file", lockPath, "--no-hooks", "alpha"})

	if code != exitOK {
		t.Fatalf("remove --no-hooks: expected exitOK (%d), got %d", exitOK, code)
	}
	if _, err := os.Stat(hookLog); !os.IsNotExist(err) {
		t.Fatalf("hook log stat = %v, want not exist", err)
	}
	got, err := os.ReadFile(uninstallLog)
	if err != nil {
		t.Fatalf("read uninstall log: %v", err)
	}
	if string(got) != "uninstall" {
		t.Fatalf("uninstall log = %q, want uninstall", string(got))
	}
}

func TestApply_HookTimeout_surfaces_actionable_error(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "xdg"))
	specPath := filepath.Join(dir, "genv.json")
	lockPath := filepath.Join(dir, "genv.lock.json")

	spec := `{"schemaVersion":"6","packages":[],"hooks":{"preApply":[{"command":"sleep 1"}]}}`
	if err := os.WriteFile(specPath, []byte(spec), 0o644); err != nil {
		t.Fatalf("write spec: %v", err)
	}

	var code int
	out := captureStderr(t, func() {
		code = run([]string{"apply", "--file", specPath, "--lock-file", lockPath, "--yes", "--hook-timeout", "1ms"})
	})

	if code != exitLogic {
		t.Fatalf("apply timed-out hook: expected exitLogic (%d), got %d", exitLogic, code)
	}
	if !strings.Contains(out, "pre-apply hook timed out after 1ms") || !strings.Contains(out, "context deadline exceeded") {
		t.Fatalf("apply timeout error missing expected text, got: %s", out)
	}
}

func TestAdd_HookTimeout_surfaces_actionable_error(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "xdg"))
	specPath := filepath.Join(dir, "genv.json")
	lockPath := filepath.Join(dir, "genv.lock.json")
	registerLifecycleHookAdapter(t, lifecycleHookAdapter{})

	spec := `{"schemaVersion":"6","packages":[],"hooks":{"preAdd":[{"command":"sleep 1"}]}}`
	if err := os.WriteFile(specPath, []byte(spec), 0o644); err != nil {
		t.Fatalf("write spec: %v", err)
	}

	var code int
	out := captureStderr(t, func() {
		code = run([]string{"add", "--file", specPath, "--lock-file", lockPath, "--prefer", "test-hook-manager", "--no-search", "--hook-timeout", "1ms", "alpha"})
	})

	if code != exitLogic {
		t.Fatalf("add timed-out hook: expected exitLogic (%d), got %d", exitLogic, code)
	}
	if !strings.Contains(out, "pre-add hook timed out after 1ms") || !strings.Contains(out, "context deadline exceeded") {
		t.Fatalf("add timeout error missing expected text, got: %s", out)
	}
}

func TestRemove_HookTimeout_surfaces_actionable_error(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "xdg"))
	specPath := filepath.Join(dir, "genv.json")
	lockPath := filepath.Join(dir, "genv.lock.json")
	registerLifecycleHookAdapter(t, lifecycleHookAdapter{})

	spec := `{"schemaVersion":"6","packages":[{"id":"alpha","prefer":"test-hook-manager"}],"hooks":{"preRemove":[{"command":"sleep 1"}]}}`
	if err := os.WriteFile(specPath, []byte(spec), 0o644); err != nil {
		t.Fatalf("write spec: %v", err)
	}
	writeLock(t, lockPath, []genvfile.LockedPackage{{ID: "alpha", Manager: "test-hook-manager", PkgName: "alpha"}})

	var code int
	out := captureStderr(t, func() {
		code = run([]string{"remove", "--file", specPath, "--lock-file", lockPath, "--hook-timeout", "1ms", "alpha"})
	})

	if code != exitLogic {
		t.Fatalf("remove timed-out hook: expected exitLogic (%d), got %d", exitLogic, code)
	}
	if !strings.Contains(out, "pre-remove hook timed out after 1ms") || !strings.Contains(out, "context deadline exceeded") {
		t.Fatalf("remove timeout error missing expected text, got: %s", out)
	}
}
