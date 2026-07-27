package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestApply_FileMismatchDoesNotAbortPackages(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "xdg"))
	t.Setenv("HOME", dir)
	specPath := filepath.Join(dir, "genv.json")
	lockPath := filepath.Join(dir, "genv.lock.json")
	sourcePath := filepath.Join(dir, "source.txt")
	targetPath := filepath.Join(dir, "target.txt")
	installLog := filepath.Join(dir, "install.log")

	writeTestFile(t, sourcePath, "desired\n")
	writeTestFile(t, targetPath, "blocking-real-file\n")
	registerLifecycleHookAdapter(t, lifecycleHookAdapter{installMarker: installLog})

	writeTestFile(t, specPath, `{
		"schemaVersion":"6",
		"packages":[{"id":"alpha","prefer":"test-hook-manager"}],
		"files":{"links":[{"source":"`+sourcePath+`","target":"`+targetPath+`","mode":"link"}]}
	}`)

	var code int
	var stdout, stderr string
	stdout = captureStdout(t, func() {
		stderr = captureStderr(t, func() {
			code = run([]string{"apply", "--file", specPath, "--lock-file", lockPath, "--yes", "--no-hooks"})
		})
	})

	if code != exitLogic {
		t.Fatalf("apply with mismatch: expected exitLogic (%d), got %d\nstdout=%s\nstderr=%s", exitLogic, code, stdout, stderr)
	}
	combined := stdout + stderr
	if !strings.Contains(combined, "mismatch") {
		t.Fatalf("expected mismatch in output; stdout=%s stderr=%s", stdout, stderr)
	}
	if !strings.Contains(combined, targetPath) && !strings.Contains(combined, filepath.Base(targetPath)) {
		t.Fatalf("expected mismatch path in output; stdout=%s stderr=%s", stdout, stderr)
	}
	got, err := os.ReadFile(installLog)
	if err != nil {
		t.Fatalf("package install should still run despite file mismatch; read install log: %v\nstdout=%s stderr=%s", err, stdout, stderr)
	}
	if string(got) != "install" {
		t.Fatalf("install log = %q, want install", got)
	}
	fi, err := os.Lstat(targetPath)
	if err != nil {
		t.Fatalf("lstat target: %v", err)
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		t.Fatal("target became a symlink without --force")
	}
}

func TestApply_DryRunTextShowsFilePaths(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "xdg"))
	t.Setenv("HOME", dir)
	specPath := filepath.Join(dir, "genv.json")
	lockPath := filepath.Join(dir, "genv.lock.json")
	sourcePath := filepath.Join(dir, "source.txt")
	targetPath := filepath.Join(dir, "target.txt")

	writeTestFile(t, sourcePath, "desired\n")
	writeTestFile(t, targetPath, "blocking\n")
	writeTestFile(t, specPath, `{
		"schemaVersion":"5",
		"files":{"links":[{"source":"`+sourcePath+`","target":"`+targetPath+`","mode":"link"}]}
	}`)

	var code int
	stdout := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			code = run([]string{"apply", "--file", specPath, "--lock-file", lockPath, "--dry-run", "--yes"})
		})
	})
	if code != exitOK && code != exitLogic {
		t.Fatalf("dry-run: unexpected exit %d\n%s", code, stdout)
	}
	if !strings.Contains(stdout, targetPath) {
		t.Fatalf("dry-run text should name file path; got: %s", stdout)
	}
	if !strings.Contains(stdout, "mismatch") {
		t.Fatalf("dry-run text should include mismatch kind; got: %s", stdout)
	}
}

func TestApply_ForceBackupFlagBacksUpWithoutPerEntryBackup(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "xdg"))
	t.Setenv("HOME", dir)
	specPath := filepath.Join(dir, "genv.json")
	lockPath := filepath.Join(dir, "genv.lock.json")
	sourcePath := filepath.Join(dir, "source.txt")
	targetPath := filepath.Join(dir, "target.txt")

	writeTestFile(t, sourcePath, "desired\n")
	writeTestFile(t, targetPath, "old-content\n")
	writeTestFile(t, specPath, `{
		"schemaVersion":"5",
		"files":{"links":[{"source":"`+sourcePath+`","target":"`+targetPath+`","mode":"link"}]}
	}`)

	code := run([]string{"apply", "--file", specPath, "--lock-file", lockPath, "--force", "--backup", "--yes", "--no-hooks"})
	if code != exitOK {
		t.Fatalf("apply --force --backup: expected exitOK, got %d", code)
	}
	fi, err := os.Lstat(targetPath)
	if err != nil {
		t.Fatalf("lstat: %v", err)
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		t.Fatal("expected symlink after --force")
	}
	matches, err := filepath.Glob(targetPath + ".backup.*")
	if err != nil {
		t.Fatalf("glob backup: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected one backup file, got %v", matches)
	}
}
