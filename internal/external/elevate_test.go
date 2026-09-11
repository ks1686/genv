package external

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestElevationHintIsExplicitForSystemScope(t *testing.T) {
	if ElevationHint("user") != "" || ElevationHint("") != "" {
		t.Fatal("user scope must not request elevation")
	}
	if ElevationHint("system") == "" {
		t.Fatal("system scope must describe elevation")
	}
}

func TestWrapSystemScopeErrorDoesNotRetryPermissionFailure(t *testing.T) {
	err := wrapSystemScopeError("system", "/usr/local/bin/tool", os.ErrPermission)
	if err == nil || !errors.Is(err, ErrElevationRequired) || !errors.Is(err, os.ErrPermission) {
		t.Fatalf("error = %v", err)
	}
	if wrapSystemScopeError("user", "/usr/local/bin/tool", os.ErrPermission) != os.ErrPermission {
		t.Fatal("user-scope permission errors must not be rewritten as elevation")
	}
}

func TestInstallDirectWithMode_unattendedRefusesElevation(t *testing.T) {
	origReq := destinationRequiresElevation
	origRun := runElevated
	t.Cleanup(func() {
		destinationRequiresElevation = origReq
		runElevated = origRun
	})
	destinationRequiresElevation = func(string) bool { return true }
	runElevated = func(io.Reader, io.Writer, []string) error {
		t.Fatal("unattended install must not spawn sudo")
		return nil
	}

	source := filepath.Join(t.TempDir(), "src")
	if err := os.WriteFile(source, []byte("bin"), 0o644); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(t.TempDir(), "tool")
	_, _, err := installDirectWithMode(source, dest, 0o755, installPolicy{Scope: "system", Mode: ExecutionUnattended})
	if !errors.Is(err, ErrUnattendedElevation) {
		t.Fatalf("error = %v, want ErrUnattendedElevation", err)
	}
}

func TestInstallDirectWithMode_interactiveElevatesInsteadOfUnprivilegedWrite(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix sudo elevation path")
	}
	origReq := destinationRequiresElevation
	origRun := runElevated
	t.Cleanup(func() {
		destinationRequiresElevation = origReq
		runElevated = origRun
	})
	destinationRequiresElevation = func(string) bool { return true }

	source := filepath.Join(t.TempDir(), "src")
	if err := os.WriteFile(source, []byte("bin"), 0o644); err != nil {
		t.Fatal(err)
	}
	destDir := t.TempDir()
	dest := filepath.Join(destDir, "tool")
	var argvLog [][]string
	runElevated = func(_ io.Reader, _ io.Writer, argv []string) error {
		argvLog = append(argvLog, append([]string(nil), argv...))
		switch {
		case len(argv) >= 3 && argv[1] == "mkdir":
			return os.MkdirAll(argv[len(argv)-1], 0o755)
		case len(argv) >= 5 && argv[1] == "install":
			return os.WriteFile(argv[len(argv)-1], []byte("bin"), 0o755)
		case len(argv) >= 4 && argv[1] == "mv":
			return os.Rename(argv[2], argv[3])
		case len(argv) >= 3 && argv[1] == "rm":
			_ = os.Remove(argv[len(argv)-1])
			return nil
		default:
			return nil
		}
	}

	restore, finish, err := installDirectWithMode(source, dest, 0o755, installPolicy{Scope: "system", Mode: ExecutionAssumeYes})
	if err != nil {
		t.Fatalf("installDirectWithMode() error: %v", err)
	}
	if restore == nil || finish == nil {
		t.Fatal("expected restore and finish")
	}
	if len(argvLog) == 0 {
		t.Fatal("expected elevated commands")
	}
	if filepath.Base(argvLog[0][0]) != "sudo" {
		t.Fatalf("first elevated argv = %v, want sudo", argvLog[0])
	}
	for _, argv := range argvLog {
		if len(argv) > 1 && argv[1] == "-n" {
			t.Fatalf("interactive elevation used sudo -n: %v", argv)
		}
	}
	if _, err := os.Stat(dest); err != nil {
		t.Fatalf("destination missing after elevated install: %v", err)
	}
	if err := finish(); err != nil {
		t.Fatal(err)
	}
}

func TestInstallDirectWithMode_writableSystemScopeStaysUnprivileged(t *testing.T) {
	origRun := runElevated
	t.Cleanup(func() { runElevated = origRun })
	runElevated = func(io.Reader, io.Writer, []string) error {
		t.Fatal("writable destination must not elevate")
		return nil
	}
	source := filepath.Join(t.TempDir(), "src")
	if err := os.WriteFile(source, []byte("bin"), 0o644); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(t.TempDir(), "tool")
	restore, finish, err := installDirectWithMode(source, dest, 0o755, installPolicy{Scope: "system"})
	if err != nil {
		t.Fatalf("installDirectWithMode() error: %v", err)
	}
	defer restore()
	if err := finish(); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(dest)
	if err != nil || string(got) != "bin" {
		t.Fatalf("got %q err %v", got, err)
	}
}

func TestInstallDirectWithMode_unwritableDoesNotLeaveStagingFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix permission bits")
	}
	destDir := t.TempDir()
	dest := filepath.Join(destDir, "tool")
	if err := os.Chmod(destDir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(destDir, 0o755) })
	source := filepath.Join(t.TempDir(), "src")
	if err := os.WriteFile(source, []byte("bin"), 0o644); err != nil {
		t.Fatal(err)
	}
	origRun := runElevated
	t.Cleanup(func() { runElevated = origRun })
	runElevated = func(io.Reader, io.Writer, []string) error {
		return errors.New("sudo unavailable in test")
	}
	_, _, err := installDirectWithMode(source, dest, 0o755, installPolicy{Scope: "system", Mode: ExecutionAssumeYes})
	if err == nil {
		t.Fatal("expected elevation failure")
	}
	entries, readErr := os.ReadDir(destDir)
	if readErr != nil {
		t.Fatal(readErr)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".genv-install-") {
			t.Fatalf("unprivileged staging file %s was created under the system destination", entry.Name())
		}
	}
}
