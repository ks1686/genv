package schema

import "testing"

func TestDropInapplicable_WindowsDropsZshAndHomebrew(t *testing.T) {
	in := &GenvFile{
		Env: map[string]EnvVar{
			"EDITOR":                {Value: "nvim"},
			"HOMEBREW_NO_ENV_HINTS": {Value: "1"},
		},
		Shell: &ShellConfig{
			Aliases: map[string]ShellAlias{
				"ll": {Value: "ls -lh", Shell: "zsh"},
				"vi": {Value: "nvim"},
			},
		},
	}
	got := DropInapplicable(in, "windows")
	if _, ok := got.Env["HOMEBREW_NO_ENV_HINTS"]; ok {
		t.Fatal("HOMEBREW_NO_ENV_HINTS leaked onto windows")
	}
	if got.Env["EDITOR"].Value != "nvim" {
		t.Fatal("EDITOR should remain")
	}
	if got.Shell != nil {
		if _, ok := got.Shell.Aliases["ll"]; ok {
			t.Fatal("zsh alias leaked onto windows")
		}
		if _, ok := got.Shell.Aliases["vi"]; ok {
			t.Fatal("unscoped POSIX alias leaked onto windows")
		}
	}
}

func TestDropInapplicable_DarwinKeepsHomebrewAndZsh(t *testing.T) {
	in := &GenvFile{
		Env:   map[string]EnvVar{"HOMEBREW_NO_ENV_HINTS": {Value: "1"}},
		Shell: &ShellConfig{Aliases: map[string]ShellAlias{"ll": {Value: "ls -lh", Shell: "zsh"}}},
	}
	got := DropInapplicable(in, "darwin")
	if _, ok := got.Env["HOMEBREW_NO_ENV_HINTS"]; !ok {
		t.Fatal("expected HOMEBREW on darwin")
	}
	if _, ok := got.Shell.Aliases["ll"]; !ok {
		t.Fatal("expected zsh alias on darwin")
	}
}

func TestDropInapplicable_Nil(t *testing.T) {
	if DropInapplicable(nil, "windows") != nil {
		t.Fatal("nil in should stay nil")
	}
}

func TestDropInapplicable_KeepsPowerShellAliasOnWindows(t *testing.T) {
	in := &GenvFile{
		Shell: &ShellConfig{Aliases: map[string]ShellAlias{
			"ll": {Value: "Get-ChildItem", Shell: "powershell"},
		}},
	}
	got := DropInapplicable(in, "windows")
	if _, ok := got.Shell.Aliases["ll"]; !ok {
		t.Fatal("powershell alias should remain on windows")
	}
}
