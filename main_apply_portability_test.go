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

func TestApply_MissingLockMetadataRefused(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "xdg"))
	specPath := filepath.Join(dir, "genv.json")
	lockPath := filepath.Join(dir, "genv.lock.json")

	writeTestFile(t, specPath, `{"schemaVersion":"8","targets":{"arch":{"packages":[{"id":"git","prefer":"mas"}]}}}`)
	writeTestFile(t, lockPath, `{"schemaVersion":"1","packages":[{"id":"x","manager":"mas","pkgName":"1"}]}`)

	var code int
	errOut := captureStderr(t, func() {
		code = run([]string{"apply", "--file", specPath, "--lock-file", lockPath, "--target", "arch", "--yes", "--dry-run"})
	})

	if code != exitLogic {
		t.Fatalf("apply missing lock metadata: expected exitLogic (%d), got %d; stderr=%s", exitLogic, code, errOut)
	}
	if !strings.Contains(errOut, "foreign lock") || !strings.Contains(errOut, "target/goos") {
		t.Fatalf("missing metadata message, got: %s", errOut)
	}
}

func TestUpgrade_ForeignLockRefused(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "xdg"))
	specPath := filepath.Join(dir, "genv.json")
	lockPath := filepath.Join(dir, "genv.lock.json")

	writeTestFile(t, specPath, `{"schemaVersion":"8","targets":{"arch":{"packages":[{"id":"git","prefer":"pacman"}]}}}`)
	writeTestFile(t, lockPath, `{"schemaVersion":"8","target":"macos","goos":"darwin","packages":[{"id":"git","manager":"brew","pkgName":"git"}]}`)

	var code int
	errOut := captureStderr(t, func() {
		code = run([]string{"upgrade", "--file", specPath, "--lock-file", lockPath, "--target", "arch", "--dry-run"})
	})

	if code != exitLogic {
		t.Fatalf("upgrade foreign lock: expected exitLogic (%d), got %d; stderr=%s", exitLogic, code, errOut)
	}
	if !strings.Contains(errOut, "foreign lock") {
		t.Fatalf("upgrade foreign lock message missing, got: %s", errOut)
	}
	if strings.Contains(errOut, "--force-new-lock") {
		t.Fatalf("upgrade should not suggest --force-new-lock, got: %s", errOut)
	}
}

func TestRemove_UninstallFailureLeavesLock(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "xdg"))
	specPath := filepath.Join(dir, "genv.json")
	lockPath := filepath.Join(dir, "genv.lock.json")
	registerLifecycleHookAdapter(t, lifecycleHookAdapter{failUninstall: true})

	writeTestFile(t, specPath, `{"schemaVersion":"7","packages":[{"id":"alpha","prefer":"test-hook-manager"}]}`)
	writeLock(t, lockPath, []genvfile.LockedPackage{{ID: "alpha", Manager: "test-hook-manager", PkgName: "alpha"}})

	var code int
	errOut := captureStderr(t, func() {
		code = run([]string{"remove", "--file", specPath, "--lock-file", lockPath, "--no-hooks", "alpha"})
	})
	if code != exitLogic {
		t.Fatalf("remove uninstall failure: expected exitLogic (%d), got %d; stderr=%s", exitLogic, code, errOut)
	}

	lf, err := genvfile.ReadLock(lockPath)
	if err != nil {
		t.Fatalf("read lock: %v", err)
	}
	if len(lf.Packages) != 1 || lf.Packages[0].ID != "alpha" {
		t.Fatalf("lock packages = %+v, want alpha retained", lf.Packages)
	}

	f, err := genvfile.Read(specPath)
	if err != nil {
		t.Fatalf("read spec: %v", err)
	}
	if len(f.Packages) != 1 || f.Packages[0].ID != "alpha" {
		t.Fatalf("spec packages = %+v, want alpha retained after uninstall failure", f.Packages)
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
