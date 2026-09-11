package external

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/ks1686/genv/internal/schema"
)

func TestExpandInstallTemplateRejectsUnknownPlaceholder(t *testing.T) {
	if _, err := ExpandInstallTemplate("{version}-{unknown}", TemplateValues{Version: "1.2.3"}); err == nil {
		t.Fatal("unknown placeholder accepted")
	}
}

func TestRunInstallerScriptUsesExplicitArgvAndEnv(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "log")
	t.Setenv("SCRIPT_LOG", logPath)

	var (
		interpreter string
		script      string
		hostOS      = "linux"
	)
	if runtime.GOOS == "windows" {
		// Windows CI has pwsh; bash is not guaranteed. Exercise the supported
		// PowerShell interpreter path instead of a fake bash shim.
		if _, err := exec.LookPath("pwsh"); err != nil {
			if _, err = exec.LookPath("powershell"); err != nil {
				t.Skip("pwsh/powershell unavailable")
			}
			interpreter = "powershell"
		} else {
			interpreter = "pwsh"
		}
		hostOS = "windows"
		script = filepath.Join(dir, "installer.ps1")
		ps := "$line = '{0}|{1}|{2}' -f $args[0], $args[1], $env:TEST_VALUE\n" +
			"Set-Content -Path $env:SCRIPT_LOG -Value $line -NoNewline\n"
		if err := os.WriteFile(script, []byte(ps), 0o600); err != nil {
			t.Fatal(err)
		}
	} else {
		interpreter = "bash"
		fakeBash := filepath.Join(dir, "bash")
		if err := os.WriteFile(fakeBash, []byte("#!/bin/sh\nprintf '%s|%s|%s' \"$2\" \"$3\" \"$TEST_VALUE\" > \"$SCRIPT_LOG\"\n"), 0o755); err != nil {
			t.Fatal(err)
		}
		t.Setenv("PATH", dir)
		script = filepath.Join(dir, "installer")
		if err := os.WriteFile(script, []byte("unused"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	var stderr bytes.Buffer
	err := RunInstallerScript(context.Background(), script, schema.ExternalInstall{
		Interpreter: interpreter,
		Args:        []string{"--version", "{version}"},
		Env:         map[string]string{"TEST_VALUE": "{os}"},
	}, TemplateValues{Version: "1.2.3", OS: hostOS}, nil, &stderr)
	if err != nil {
		t.Fatalf("RunInstallerScript() error: %v (%s)", err, stderr.String())
	}
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	wantSuffix := "--version|1.2.3|" + hostOS
	if got := string(data); !strings.HasSuffix(got, wantSuffix) {
		t.Fatalf("argv/env = %q, want suffix %q", got, wantSuffix)
	}
}

func TestRunUninstallExecutesExactArgv(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "removed")
	var bin string
	if runtime.GOOS == "windows" {
		bin = filepath.Join(dir, "uninstaller.cmd")
		if err := os.WriteFile(bin, []byte("@echo off\r\ntype nul > \"%~1\"\r\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	} else {
		bin = filepath.Join(dir, "uninstaller")
		if err := os.WriteFile(bin, []byte("#!/bin/sh\ntouch \"$1\"\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := RunUninstall(context.Background(), []string{bin, marker}, nil, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("uninstall argv was not executed: %v", err)
	}
}

func TestRunUninstallRequiresAction(t *testing.T) {
	if err := RunUninstall(context.Background(), nil, nil, nil); err == nil {
		t.Fatal("empty uninstall accepted")
	}
}

func TestResolveInterpreterRejectsUnknown(t *testing.T) {
	if _, _, err := resolveInterpreter("python"); err == nil {
		t.Fatal("unknown interpreter accepted")
	}
}

func TestEnsureScriptExtensionStagesPowerShell(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, ".genv-download-1")
	if err := os.WriteFile(src, []byte("Write-Host hi"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := ensureScriptExtension(src, "pwsh")
	if err != nil {
		t.Fatal(err)
	}
	if got != src+".ps1" {
		t.Fatalf("path = %q", got)
	}
	if _, err := os.Stat(got); err != nil {
		t.Fatal(err)
	}
	same, err := ensureScriptExtension(got, "powershell")
	if err != nil || same != got {
		t.Fatalf("idempotent = %q %v", same, err)
	}
	unchanged, err := ensureScriptExtension(src+".bak", "bash")
	if err != nil || unchanged != src+".bak" {
		t.Fatalf("bash path = %q %v", unchanged, err)
	}
}
