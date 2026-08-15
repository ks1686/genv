package testutil

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

// InstallFakeBinary writes a POSIX shell script named name onto PATH.
// On Windows it also writes a .cmd shim so exec.LookPath finds the fake
// before later PATHEXT entries such as winget.exe.
func InstallFakeBinary(t *testing.T, name, body string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		if _, err := exec.LookPath("bash"); err != nil {
			t.Skip("InstallFakeBinary requires bash on Windows")
		}
	}
	dir := t.TempDir()
	script := "#!/bin/sh\n" + body + "\n"
	shPath := filepath.Join(dir, name)
	if runtime.GOOS == "windows" {
		shPath = filepath.Join(dir, name+".sh")
	}
	if err := os.WriteFile(shPath, []byte(script), 0o755); err != nil {
		t.Fatalf("InstallFakeBinary(%q): WriteFile: %v", name, err)
	}
	if runtime.GOOS == "windows" {
		// Invoke bash with a quoted script path. Callers whose argv contains
		// cmd metacharacters (e.g. apk version -l <) should skip on Windows.
		shim := "@echo off\r\nbash \"" + shPath + "\" %*\r\n"
		if err := os.WriteFile(filepath.Join(dir, name+".cmd"), []byte(shim), 0o755); err != nil {
			t.Fatalf("InstallFakeBinary(%q): WriteFile cmd: %v", name, err)
		}
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}
