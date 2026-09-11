package external

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
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
	interpreter := filepath.Join(dir, "bash")
	if err := os.WriteFile(interpreter, []byte("#!/bin/sh\nprintf '%s|%s|%s' \"$2\" \"$3\" \"$TEST_VALUE\" > \"$SCRIPT_LOG\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	t.Setenv("SCRIPT_LOG", logPath)
	script := filepath.Join(dir, "installer")
	if err := os.WriteFile(script, []byte("unused"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	err := RunInstallerScript(context.Background(), script, schema.ExternalInstall{
		Interpreter: "bash",
		Args:        []string{"--version", "{version}"},
		Env:         map[string]string{"TEST_VALUE": "{os}"},
	}, TemplateValues{Version: "1.2.3", OS: "linux"}, nil, &stderr)
	if err != nil {
		t.Fatalf("RunInstallerScript() error: %v (%s)", err, stderr.String())
	}
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(data); !strings.HasSuffix(got, "--version|1.2.3|linux") {
		t.Fatalf("argv/env = %q", got)
	}
}

func TestRunUninstallExecutesExactArgv(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "removed")
	bin := filepath.Join(dir, "uninstaller")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\ntouch \"$1\"\n"), 0o755); err != nil {
		t.Fatal(err)
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
