package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/ks1686/genv/internal/genvfile"
)

func writeEmptyV1Spec(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(`{"schemaVersion":"1","packages":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestEnvSetWritesSpec(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	path := filepath.Join(dir, "genv.json")
	writeEmptyV1Spec(t, path)

	if code := run([]string{"env", "set", "--file", path, "FOO", "bar"}); code != exitOK {
		t.Fatalf("env set: expected exitOK, got %d", code)
	}

	spec, err := genvfile.Read(path)
	if err != nil {
		t.Fatalf("read spec: %v", err)
	}
	if got := spec.Env["FOO"]; got.Value != "bar" || got.Sensitive {
		t.Errorf("Env[FOO] = %#v, want value bar and Sensitive false", got)
	}
}

func TestEnvListJSONRedactsSensitiveValue(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	path := filepath.Join(dir, "genv.json")
	writeEmptyV1Spec(t, path)

	if code := run([]string{"env", "set", "--file", path, "--sensitive", "SECRET", "x"}); code != exitOK {
		t.Fatalf("env set: expected exitOK, got %d", code)
	}

	var code int
	stdout := captureStdout(t, func() {
		code = run([]string{"env", "list", "--file", path, "--json"})
	})
	if code != exitOK {
		t.Fatalf("env list: expected exitOK, got %d", code)
	}
	if strings.Contains(stdout, `"x"`) {
		t.Errorf("sensitive value appeared in JSON: %s", stdout)
	}

	var result struct {
		Data struct {
			Entries []struct {
				Name      string `json:"name"`
				SpecValue string `json:"specValue"`
				Sensitive bool   `json:"sensitive"`
			} `json:"entries"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("decode env list JSON: %v\noutput: %s", err, stdout)
	}
	if len(result.Data.Entries) != 1 {
		t.Fatalf("got %d env entries, want 1", len(result.Data.Entries))
	}
	entry := result.Data.Entries[0]
	if entry.Name != "SECRET" || entry.SpecValue != "[redacted]" || !entry.Sensitive {
		t.Errorf("entry = %#v, want redacted SECRET entry", entry)
	}
}

func TestEnvUnsetRemovesVariableAndReportsMissing(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	path := filepath.Join(dir, "genv.json")
	writeEmptyV1Spec(t, path)

	if code := run([]string{"env", "set", "--file", path, "FOO", "bar"}); code != exitOK {
		t.Fatalf("env set: expected exitOK, got %d", code)
	}
	if code := run([]string{"env", "unset", "--file", path, "FOO"}); code != exitOK {
		t.Fatalf("env unset: expected exitOK, got %d", code)
	}

	spec, err := genvfile.Read(path)
	if err != nil {
		t.Fatalf("read spec: %v", err)
	}
	if _, exists := spec.Env["FOO"]; exists {
		t.Error("FOO remained in spec after env unset")
	}
	if code := run([]string{"env", "unset", "--file", path, "FOO"}); code != exitLogic {
		t.Errorf("unset missing variable: expected exitLogic (%d), got %d", exitLogic, code)
	}
}

func TestEnvCmdRejectsInvalidUsage(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "no subcommand", args: []string{"env"}},
		{name: "unknown subcommand", args: []string{"env", "export"}},
		{name: "set missing value", args: []string{"env", "set", "FOO"}},
		{name: "set invalid flag", args: []string{"env", "set", "--invalid"}},
		{name: "unset missing name", args: []string{"env", "unset"}},
		{name: "list invalid flag", args: []string{"env", "list", "--invalid"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if code := run(tc.args); code != exitUsage {
				t.Errorf("run(%q): expected exitUsage (%d), got %d", tc.args, exitUsage, code)
			}
		})
	}
}

func TestShellAliasSetWritesSpec(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	path := filepath.Join(dir, "genv.json")
	writeEmptyV1Spec(t, path)

	if code := run([]string{"shell", "alias", "set", "--file", path, "ll", "ls -la"}); code != exitOK {
		t.Fatalf("shell alias set: expected exitOK, got %d", code)
	}

	spec, err := genvfile.Read(path)
	if err != nil {
		t.Fatalf("read spec: %v", err)
	}
	if got := spec.Shell.Aliases["ll"]; got.Value != "ls -la" || got.Shell != "" {
		t.Errorf("Shell.Aliases[ll] = %#v, want unscoped ls -la", got)
	}
}

func TestShellAliasUnsetRemovesAliasAndReportsMissing(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	path := filepath.Join(dir, "genv.json")
	writeEmptyV1Spec(t, path)

	if code := run([]string{"shell", "alias", "set", "--file", path, "ll", "ls -la"}); code != exitOK {
		t.Fatalf("shell alias set: expected exitOK, got %d", code)
	}
	if code := run([]string{"shell", "alias", "unset", "--file", path, "ll"}); code != exitOK {
		t.Fatalf("shell alias unset: expected exitOK, got %d", code)
	}

	spec, err := genvfile.Read(path)
	if err != nil {
		t.Fatalf("read spec: %v", err)
	}
	if _, exists := spec.Shell.Aliases["ll"]; exists {
		t.Error("ll remained in spec after shell alias unset")
	}
	if code := run([]string{"shell", "alias", "unset", "--file", path, "ll"}); code != exitLogic {
		t.Errorf("unset missing alias: expected exitLogic (%d), got %d", exitLogic, code)
	}
}

func TestShellStatusReportsMatchingAndDrift(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	path := filepath.Join(dir, "genv.json")
	writeEmptyV1Spec(t, path)

	setArgs := []string{"shell", "alias", "set", "--file", path, "ll", "ls -la"}
	shellName := ""
	if runtime.GOOS == "windows" {
		shellName = "powershell"
		setArgs = []string{"shell", "alias", "set", "--file", path, "--shell", shellName, "ll", "ls -la"}
	}
	if code := run(setArgs); code != exitOK {
		t.Fatalf("shell alias set: expected exitOK, got %d", code)
	}
	lockPath := genvfile.LockPathFrom(path)
	writeLock(t, lockPath, nil)
	lock, err := genvfile.ReadLock(lockPath)
	if err != nil {
		t.Fatalf("read lock: %v", err)
	}
	lock.Shell = &genvfile.LockedShellConfig{
		Aliases: []genvfile.LockedShellAlias{{Name: "ll", Value: "ls -la", Shell: shellName}},
	}
	if err := genvfile.WriteLock(lockPath, lock); err != nil {
		t.Fatalf("write matching lock: %v", err)
	}

	if code := run([]string{"shell", "status", "--file", path}); code != exitOK {
		t.Fatalf("matching shell status: expected exitOK, got %d", code)
	}
	changeArgs := []string{"shell", "alias", "set", "--file", path, "ll", "ls -lh"}
	if shellName != "" {
		changeArgs = []string{"shell", "alias", "set", "--file", path, "--shell", shellName, "ll", "ls -lh"}
	}
	if code := run(changeArgs); code != exitOK {
		t.Fatalf("change shell alias: expected exitOK, got %d", code)
	}
	if code := run([]string{"shell", "status", "--file", path}); code != exitLogic {
		t.Errorf("drifted shell status: expected exitLogic (%d), got %d", exitLogic, code)
	}
}

func TestShellCmdRejectsInvalidUsage(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "no subcommand", args: []string{"shell"}},
		{name: "unknown subcommand", args: []string{"shell", "export"}},
		{name: "alias missing subcommand", args: []string{"shell", "alias"}},
		{name: "alias unknown subcommand", args: []string{"shell", "alias", "rename"}},
		{name: "set missing value", args: []string{"shell", "alias", "set", "ll"}},
		{name: "set invalid flag", args: []string{"shell", "alias", "set", "--invalid"}},
		{name: "unset missing name", args: []string{"shell", "alias", "unset"}},
		{name: "status invalid flag", args: []string{"shell", "status", "--invalid"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if code := run(tc.args); code != exitUsage {
				t.Errorf("run(%q): expected exitUsage (%d), got %d", tc.args, exitUsage, code)
			}
		})
	}
}
