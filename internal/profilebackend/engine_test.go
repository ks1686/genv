package profilebackend

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ks1686/genv/internal/testutil"
)

func TestDetectEngine_PreferPwsh(t *testing.T) {
	dir := t.TempDir()
	pwsh := filepath.Join(dir, "pwsh")
	ps := filepath.Join(dir, "powershell")
	if err := os.WriteFile(pwsh, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ps, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	prev := lookPath
	t.Cleanup(func() { lookPath = prev })
	lookPath = func(file string) (string, error) {
		switch file {
		case "pwsh":
			return pwsh, nil
		case "powershell", "powershell.exe":
			return ps, nil
		default:
			return "", os.ErrNotExist
		}
	}

	eng, ok := DetectEngine()
	if !ok {
		t.Fatal("expected engine")
	}
	if eng.Bin != pwsh {
		t.Errorf("Bin = %q, want %q", eng.Bin, pwsh)
	}
	if !eng.IsPwsh() {
		t.Error("expected IsPwsh true")
	}
}

func TestDetectEngine_FallbackPowerShell(t *testing.T) {
	dir := t.TempDir()
	ps := filepath.Join(dir, "powershell.exe")
	if err := os.WriteFile(ps, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	prev := lookPath
	t.Cleanup(func() { lookPath = prev })
	lookPath = func(file string) (string, error) {
		if file == "powershell.exe" {
			return ps, nil
		}
		return "", os.ErrNotExist
	}

	eng, ok := DetectEngine()
	if !ok {
		t.Fatal("expected engine")
	}
	if eng.Bin != ps {
		t.Errorf("Bin = %q, want %q", eng.Bin, ps)
	}
	if eng.IsPwsh() {
		t.Error("expected IsPwsh false for powershell.exe")
	}
}

func TestDetectEngine_Neither(t *testing.T) {
	prev := lookPath
	t.Cleanup(func() { lookPath = prev })
	lookPath = func(string) (string, error) { return "", os.ErrNotExist }

	if _, ok := DetectEngine(); ok {
		t.Fatal("expected no engine")
	}
}

func TestSelectBackends_NonWindows(t *testing.T) {
	backends := SelectBackends("darwin")
	if len(backends) != 1 || backends[0].Name() != "posix" {
		t.Fatalf("got %#v", backends)
	}
}

func TestSelectBackendsIn_SetsDir(t *testing.T) {
	dir := t.TempDir()
	backends := SelectBackendsIn("darwin", dir)
	if len(backends) != 1 {
		t.Fatalf("got %#v", backends)
	}
	posix, ok := backends[0].(POSIXBackend)
	if !ok {
		t.Fatalf("got %T", backends[0])
	}
	if posix.Dir != dir {
		t.Fatalf("Dir = %q, want %q", posix.Dir, dir)
	}
	if SelectBackendsIn("darwin", "")[0].(POSIXBackend).Dir != "" {
		t.Fatal("empty state dir should leave Dir unset")
	}
}

func TestSelectBackends_WindowsWithEngine(t *testing.T) {
	prev := lookPath
	t.Cleanup(func() { lookPath = prev })
	lookPath = func(file string) (string, error) {
		if file == "pwsh" {
			return "/fake/pwsh", nil
		}
		return "", os.ErrNotExist
	}
	t.Setenv("SHELL", "")
	home := t.TempDir()
	testutil.SetHome(t, home)
	// No POSIX rc → PowerShell only.
	backends := SelectBackends("windows")
	if len(backends) != 1 || backends[0].Name() != "powershell" {
		t.Fatalf("got names %v", backendNames(backends))
	}

	// With bashrc → both.
	if err := os.WriteFile(filepath.Join(home, ".bashrc"), []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	backends = SelectBackends("windows")
	names := backendNames(backends)
	if len(names) != 2 || names[0] != "powershell" || names[1] != "posix" {
		t.Fatalf("got %v, want [powershell posix]", names)
	}
}

func TestSelectBackends_WindowsNoEngine(t *testing.T) {
	prev := lookPath
	t.Cleanup(func() { lookPath = prev })
	lookPath = func(string) (string, error) { return "", os.ErrNotExist }

	home := t.TempDir()
	testutil.SetHome(t, home)
	t.Setenv("SHELL", "")
	backends := SelectBackends("windows")
	if len(backends) != 0 {
		t.Fatalf("expected empty, got %v", backendNames(backends))
	}

	t.Setenv("SHELL", "/usr/bin/bash")
	backends = SelectBackends("windows")
	if len(backends) != 1 || backends[0].Name() != "posix" {
		t.Fatalf("got %v", backendNames(backends))
	}

	warn := MissingEngineWarning("windows")
	if warn == "" {
		t.Fatal("expected missing-engine warning")
	}
	if MissingEngineWarning("darwin") != "" {
		t.Fatal("expected no warning on darwin")
	}
}

func backendNames(backends []Backend) []string {
	names := make([]string, len(backends))
	for i, b := range backends {
		names[i] = b.Name()
	}
	return names
}
