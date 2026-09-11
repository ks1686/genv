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

// WriteStdoutTool writes a runnable tool at path that prints line to stdout.
// On Windows the path gains a .cmd suffix when it has no extension, because
// extensionless shell scripts are not executable there. Returns the path to
// pass to exec / Detect.Command.
func WriteStdoutTool(t *testing.T, path, line string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		if filepath.Ext(path) == "" {
			path += ".cmd"
		}
		body := "@echo off\r\necho " + line + "\r\n"
		if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
			t.Fatalf("WriteStdoutTool(%q): %v", path, err)
		}
		return path
	}
	body := "#!/bin/sh\necho '" + line + "'\n"
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatalf("WriteStdoutTool(%q): %v", path, err)
	}
	return path
}

// DirectToolArtifact returns payload bytes and a destination-path suffix for a
// direct-install fake tool that prints line when executed on the host OS.
// Suffix is ".cmd" on Windows and "" elsewhere.
func DirectToolArtifact(line string) (payload []byte, destSuffix string) {
	if runtime.GOOS == "windows" {
		return []byte("@echo off\r\necho " + line + "\r\n"), ".cmd"
	}
	return []byte("#!/bin/sh\necho '" + line + "'\n"), ""
}
