package profilebackend

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ks1686/genv/internal/schema"
	"github.com/ks1686/genv/internal/testutil"
)

func TestWriteEnvPS1_ContentAndEscaping(t *testing.T) {
	path := filepath.Join(t.TempDir(), "env.ps1")
	vars := map[string]schema.EnvVar{
		"FOO": {Value: "bar"},
		"Q":   {Value: "it's"},
	}
	if err := WriteEnvPS1(path, vars); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got := string(data)
	if !strings.Contains(got, "$env:FOO = 'bar'") {
		t.Errorf("missing FOO line:\n%s", got)
	}
	if !strings.Contains(got, "$env:Q = 'it''s'") {
		t.Errorf("missing escaped quote:\n%s", got)
	}

	if err := WriteEnvPS1(path, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected fragment removed, err=%v", err)
	}
}

func TestWriteShellPS1_FiltersTargets(t *testing.T) {
	path := filepath.Join(t.TempDir(), "shell.ps1")
	cfg := &schema.ShellConfig{
		Aliases: map[string]schema.ShellAlias{
			"ll":  {Value: "Get-ChildItem", Shell: "powershell"},
			"gs":  {Value: "git status", Shell: "zsh"},
			"all": {Value: "echo all"},
		},
		Functions: map[string]schema.ShellFunction{
			"greet": {Body: "Write-Host hi", Shell: "powershell"},
		},
		Source: []string{`C:\tools\extra.ps1`},
	}
	if err := WriteShellPS1(path, cfg); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got := string(data)
	if !strings.Contains(got, "function ll { Get-ChildItem }") {
		t.Errorf("missing powershell alias:\n%s", got)
	}
	if !strings.Contains(got, "function greet {") || !strings.Contains(got, "Write-Host hi") {
		t.Errorf("missing powershell function:\n%s", got)
	}
	if strings.Contains(got, "git status") || strings.Contains(got, "echo all") {
		t.Errorf("unexpected non-powershell entries:\n%s", got)
	}
	if !strings.Contains(got, ". 'C:\\tools\\extra.ps1'") && !strings.Contains(got, `. 'C:\tools\extra.ps1'`) {
		t.Errorf("missing source:\n%s", got)
	}
}

func TestInjectProfileLine_Idempotent(t *testing.T) {
	home := t.TempDir()
	profile := filepath.Join(home, "Documents", "PowerShell", "Microsoft.PowerShell_profile.ps1")
	frag := filepath.Join(home, ".config", "genv", "env.ps1")
	if err := os.MkdirAll(filepath.Dir(frag), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(frag, []byte("# env\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := InjectProfileLine(profile, frag, "env"); err != nil {
		t.Fatal(err)
	}
	if err := InjectProfileLine(profile, frag, "env"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(profile)
	if err != nil {
		t.Fatal(err)
	}
	got := string(data)
	if strings.Count(got, "# BEGIN genv env") != 1 {
		t.Fatalf("expected one marked block, got:\n%s", got)
	}
	if !strings.Contains(got, ". "+psSingleQuote(frag)) {
		t.Fatalf("missing dotsource:\n%s", got)
	}
}

func TestPowerShellBackend_ApplyEnvUsesProfile(t *testing.T) {
	home := t.TempDir()
	testutil.SetHome(t, home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))

	eng := Engine{Bin: filepath.Join(home, "pwsh")}
	b := PowerShellBackend{Home: home, Engine: &eng}
	vars := map[string]schema.EnvVar{"FOO": {Value: "bar"}}
	if err := b.ApplyEnv(vars); err != nil {
		t.Fatal(err)
	}

	frag := filepath.Join(home, ".config", "genv", "env.ps1")
	if _, err := os.Stat(frag); err != nil {
		t.Fatalf("env.ps1 missing: %v", err)
	}
	profile := filepath.Join(home, "Documents", "PowerShell", "Microsoft.PowerShell_profile.ps1")
	data, err := os.ReadFile(profile)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), frag) {
		t.Fatalf("profile missing fragment path:\n%s", data)
	}
}
