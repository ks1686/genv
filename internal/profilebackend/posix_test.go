package profilebackend

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ks1686/genv/internal/schema"
	"github.com/ks1686/genv/internal/testutil"
)

func TestPOSIXBackend_StateDirSkipsRCInject(t *testing.T) {
	home := t.TempDir()
	testutil.SetHome(t, home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("SHELL", "/bin/zsh")

	state := t.TempDir()
	b := POSIXBackend{Dir: state}
	if err := b.ApplyEnv(map[string]schema.EnvVar{"FOO": {Value: "bar"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(state, "env.sh")); err != nil {
		t.Fatalf("expected fragment in state dir: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".config", "genv", "env.sh")); err == nil {
		t.Fatal("wrote default env.sh despite StateDir")
	}
	if _, err := os.Stat(filepath.Join(home, ".zshrc")); err == nil {
		t.Fatal("injected rc despite non-default state dir")
	}
}

func TestPOSIXBackend_ApplyEnvAndShell(t *testing.T) {
	home := t.TempDir()
	testutil.SetHome(t, home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("SHELL", "/bin/zsh")

	b := POSIXBackend{}
	if b.Name() != "posix" {
		t.Fatalf("Name = %q", b.Name())
	}
	if err := b.ApplyEnv(map[string]schema.EnvVar{"FOO": {Value: "bar"}}); err != nil {
		t.Fatal(err)
	}
	frag := filepath.Join(home, ".config", "genv", "env.sh")
	data, err := os.ReadFile(frag)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "export FOO=") {
		t.Fatalf("env.sh missing FOO:\n%s", data)
	}

	cfg := &schema.ShellConfig{
		Aliases: map[string]schema.ShellAlias{"ll": {Value: "ls -la"}},
	}
	if err := b.ApplyShell(cfg); err != nil {
		t.Fatal(err)
	}
	shellFrag := filepath.Join(home, ".config", "genv", "shell.sh")
	data, err = os.ReadFile(shellFrag)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "alias ll=") {
		t.Fatalf("shell.sh missing alias:\n%s", data)
	}
}

func TestPowerShellBackend_ApplyShell(t *testing.T) {
	home := t.TempDir()
	testutil.SetHome(t, home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))

	eng := Engine{Bin: filepath.Join(home, "powershell.exe")}
	b := PowerShellBackend{Home: home, Engine: &eng}
	cfg := &schema.ShellConfig{
		Aliases: map[string]schema.ShellAlias{
			"ll": {Value: "Get-ChildItem", Shell: "powershell"},
		},
		Functions: map[string]schema.ShellFunction{
			"greet": {Body: "Write-Host hi", Shell: "powershell"},
		},
	}
	if err := b.ApplyShell(cfg); err != nil {
		t.Fatal(err)
	}
	frag := filepath.Join(home, ".config", "genv", "shell.ps1")
	data, err := os.ReadFile(frag)
	if err != nil {
		t.Fatal(err)
	}
	got := string(data)
	if !strings.Contains(got, "function ll {") || !strings.Contains(got, "function greet") {
		t.Fatalf("shell.ps1 missing entries:\n%s", got)
	}
	profile := filepath.Join(home, "Documents", "WindowsPowerShell", "Microsoft.PowerShell_profile.ps1")
	pdata, err := os.ReadFile(profile)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(pdata), frag) {
		t.Fatalf("profile missing fragment:\n%s", pdata)
	}

	// Empty / non-PS content removes fragment and skips inject.
	if err := b.ApplyShell(&schema.ShellConfig{}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(frag); !os.IsNotExist(err) {
		t.Fatal("expected empty shell.ps1 removed")
	}
}

func TestSetLookPathForTest_NilRestoresDefault(t *testing.T) {
	restore := SetLookPathForTest(func(string) (string, error) { return "/x", nil })
	restore()
	restore = SetLookPathForTest(nil)
	t.Cleanup(restore)
	// Just ensure it does not panic; DetectEngine may or may not find a binary.
	_, _ = DetectEngine()
}

func TestWriteShellPS1_EmptyRemoves(t *testing.T) {
	path := filepath.Join(t.TempDir(), "shell.ps1")
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := WriteShellPS1(path, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("expected removal")
	}
}

func TestMissingEngineWarning_WithEngine(t *testing.T) {
	restore := SetLookPathForTest(func(file string) (string, error) {
		if file == "pwsh" {
			return "/pwsh", nil
		}
		return "", os.ErrNotExist
	})
	t.Cleanup(restore)
	if got := MissingEngineWarning("windows"); got != "" {
		t.Fatalf("want empty warning, got %q", got)
	}
}
