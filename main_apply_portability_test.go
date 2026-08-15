package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ks1686/genv/internal/genvfile"
)

func TestApply_ForeignLockRefused(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "xdg"))
	specPath := filepath.Join(dir, "genv.json")
	lockPath := filepath.Join(dir, "genv.lock.json")

	writeTestFile(t, specPath, `{"schemaVersion":"8","targets":{"arch":{"packages":[{"id":"git","prefer":"mas"}]}}}`)
	writeTestFile(t, lockPath, `{"schemaVersion":"1","target":"macos","goos":"darwin","packages":[{"id":"x","manager":"mas","pkgName":"1"}]}`)

	var code int
	errOut := captureStderr(t, func() {
		code = run([]string{"apply", "--file", specPath, "--lock-file", lockPath, "--target", "arch", "--yes", "--dry-run"})
	})

	if code != exitLogic {
		t.Fatalf("apply foreign lock: expected exitLogic (%d), got %d; stderr=%s", exitLogic, code, errOut)
	}
	if !strings.Contains(errOut, "foreign lock") || !strings.Contains(errOut, "--force-new-lock") {
		t.Fatalf("foreign lock message missing reason/guidance, got: %s", errOut)
	}
}

func TestApply_ForceNewLockAllowsDryRunWithoutForeignUninstall(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "xdg"))
	specPath := filepath.Join(dir, "genv.json")
	lockPath := filepath.Join(dir, "genv.lock.json")
	installLog := filepath.Join(dir, "install.log")
	registerLifecycleHookAdapter(t, lifecycleHookAdapter{installMarker: installLog})

	writeTestFile(t, specPath, `{"schemaVersion":"8","targets":{"arch":{"packages":[{"id":"git","prefer":"test-hook-manager"}]}}}`)
	writeTestFile(t, lockPath, `{"schemaVersion":"1","target":"macos","goos":"darwin","packages":[{"id":"x","manager":"mas","pkgName":"1"}]}`)

	var code int
	out := captureStdout(t, func() {
		code = run([]string{"apply", "--file", specPath, "--lock-file", lockPath, "--target", "arch", "--yes", "--dry-run", "--force-new-lock"})
	})

	if code != exitOK {
		t.Fatalf("apply --force-new-lock dry-run: expected exitOK (%d), got %d; stdout=%s", exitOK, code, out)
	}
	if strings.Contains(out, "mas") || strings.Contains(out, "x") {
		t.Fatalf("force-new-lock dry-run should not plan foreign uninstall, got: %s", out)
	}
	if _, err := os.Stat(lockPath); err != nil {
		t.Fatalf("original lock must remain during force-new-lock dry-run, stat err=%v", err)
	}
	matches, err := filepath.Glob(lockPath + ".bak-*")
	if err != nil {
		t.Fatalf("glob backup: %v", err)
	}
	if len(matches) != 0 {
		t.Fatalf("dry-run must not rename lock, backups=%v", matches)
	}
}

func TestApply_V8SuccessfulWriteStampsTargetMetadata(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "xdg"))
	specPath := filepath.Join(dir, "genv.json")
	lockPath := filepath.Join(dir, "genv.lock.json")
	installLog := filepath.Join(dir, "install.log")
	registerLifecycleHookAdapter(t, lifecycleHookAdapter{installMarker: installLog})

	writeTestFile(t, specPath, `{"schemaVersion":"8","targets":{"arch":{"packages":[{"id":"alpha","prefer":"test-hook-manager"}]}}}`)

	code := run([]string{"apply", "--file", specPath, "--lock-file", lockPath, "--target", "arch", "--yes", "--no-hooks"})
	if code != exitOK {
		t.Fatalf("apply v8: expected exitOK (%d), got %d", exitOK, code)
	}
	lf, err := genvfile.ReadLock(lockPath)
	if err != nil {
		t.Fatalf("read lock: %v", err)
	}
	if lf.Target != "arch" || lf.GOOS == "" {
		t.Fatalf("lock target metadata = target %q goos %q; want arch and non-empty goos", lf.Target, lf.GOOS)
	}
}

func TestApply_LegacySuccessfulWriteDoesNotStampTargetMetadata(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "xdg"))
	specPath := filepath.Join(dir, "genv.json")
	lockPath := filepath.Join(dir, "genv.lock.json")
	installLog := filepath.Join(dir, "install.log")
	registerLifecycleHookAdapter(t, lifecycleHookAdapter{installMarker: installLog})

	writeTestFile(t, specPath, `{"schemaVersion":"7","packages":[{"id":"alpha","prefer":"test-hook-manager"}]}`)

	code := run([]string{"apply", "--file", specPath, "--lock-file", lockPath, "--target", "arch", "--yes", "--no-hooks"})
	if code != exitOK {
		t.Fatalf("apply v7: expected exitOK (%d), got %d", exitOK, code)
	}
	lf, err := genvfile.ReadLock(lockPath)
	if err != nil {
		t.Fatalf("read lock: %v", err)
	}
	if lf.Target != "" || lf.GOOS != "" {
		t.Fatalf("legacy lock target metadata = target %q goos %q; want both empty", lf.Target, lf.GOOS)
	}
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
