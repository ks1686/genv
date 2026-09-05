package schema

import (
	"strings"
	"testing"
)

func TestMergeTarget_TargetWinsAndTombstone(t *testing.T) {
	f := &GenvFile{
		SchemaVersion: Version8,
		Defaults: &TargetBundle{
			Env: map[string]*EnvVar{
				"EDITOR": {Value: "nvim"},
				"LANG":   {Value: "en_US.UTF-8"},
			},
			Shell: &TargetShellConfig{Aliases: map[string]*ShellAlias{
				"ll": {Value: "ls -la"},
			}},
		},
		Targets: map[string]*TargetBundle{
			"macos": {
				Packages: []Package{{ID: "git", Prefer: "brew"}},
				Env: map[string]*EnvVar{
					"EDITOR":                nil,
					"HOMEBREW_NO_ANALYTICS": {Value: "1"},
				},
				Shell: &TargetShellConfig{Aliases: map[string]*ShellAlias{
					"ll": nil,
					"gs": {Value: "git status"},
				}},
			},
		},
	}

	got, err := MergeTarget(f, "macos")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := got.Env["EDITOR"]; ok {
		t.Fatal("EDITOR should be tombstoned away")
	}
	if got.Env["LANG"].Value != "en_US.UTF-8" {
		t.Fatalf("LANG=%q", got.Env["LANG"].Value)
	}
	if got.Env["HOMEBREW_NO_ANALYTICS"].Value != "1" {
		t.Fatal("missing target env")
	}
	if len(got.Packages) != 1 || got.Packages[0].Prefer != "brew" {
		t.Fatalf("packages=%v", got.Packages)
	}
	if got.Shell == nil {
		t.Fatal("missing shell config")
	}
	if _, ok := got.Shell.Aliases["ll"]; ok {
		t.Fatal("ll alias should be tombstoned away")
	}
	if got.Shell.Aliases["gs"].Value != "git status" {
		t.Fatalf("missing target shell alias: %+v", got.Shell.Aliases)
	}
	if got.Targets != nil || got.Defaults != nil {
		t.Fatal("effective doc must be flat")
	}
}

func TestMergeTarget_MissingTarget(t *testing.T) {
	f := &GenvFile{SchemaVersion: Version8, Targets: map[string]*TargetBundle{"arch": {}}}
	if _, err := MergeTarget(f, "ubuntu"); err == nil {
		t.Fatal("expected error")
	}
}

func TestMergeTarget_DeepCopiesAndReplacesArrays(t *testing.T) {
	f := &GenvFile{
		SchemaVersion: Version8,
		Repo:          &Repo{URL: "https://example.com/repo", Ref: "main"},
		Updates:       &UpdatesConfig{Enabled: true, Interval: "24h", OnlyManagers: []string{"brew"}},
		Adapters:      map[string]AdapterDef{"gh-extension": {List: "gh extension list", Install: "gh extension install {{id}}", Remove: "gh extension remove {{id}}"}},
		Defaults: &TargetBundle{
			Packages: []Package{{ID: "default-pkg", Prefer: "brew", Managers: map[string]string{"brew": "default-pkg"}}},
			Env:      map[string]*EnvVar{"KEEP": {Value: "1"}},
			Services: map[string]*Service{"svc": {Start: []string{"default"}}},
			Files: &FilesConfig{
				Links: []FileLink{{Source: "default", Target: "~/default"}},
			},
			Hooks: &HooksConfig{
				PostApply: []Hook{{Command: "default"}},
			},
		},
		Targets: map[string]*TargetBundle{
			"macos": {
				Packages: []Package{{ID: "target-pkg", Prefer: "brew"}},
				Services: map[string]*Service{
					"svc": nil,
					"new": {Start: []string{"target"}},
				},
				Files: &FilesConfig{
					Links: []FileLink{{Source: "target", Target: "~/target"}},
				},
			},
			"arch": {
				Hooks: &HooksConfig{
					PostApply: []Hook{{Command: "arch"}},
				},
			},
		},
	}

	got, err := MergeTarget(f, "macos")
	if err != nil {
		t.Fatal(err)
	}
	if got.SchemaVersion != Version8 {
		t.Fatalf("schemaVersion=%q", got.SchemaVersion)
	}
	if got.Repo == f.Repo || got.Repo.URL != f.Repo.URL || got.Updates == f.Updates || got.Updates.Interval != f.Updates.Interval {
		t.Fatalf("repo/updates not deep-copied: got repo=%p src=%p updates=%p src=%p", got.Repo, f.Repo, got.Updates, f.Updates)
	}
	if len(got.Adapters) != 1 || got.Adapters["gh-extension"].List != "gh extension list" {
		t.Fatalf("adapters should copy onto the flat spec: %+v", got.Adapters)
	}
	got.Adapters["gh-extension"] = AdapterDef{List: "mutated"}
	if f.Adapters["gh-extension"].List != "gh extension list" {
		t.Fatal("MergeTarget mutated source adapters")
	}
	if len(got.Packages) != 1 || got.Packages[0].ID != "target-pkg" {
		t.Fatalf("target packages should replace defaults: %+v", got.Packages)
	}
	if _, ok := got.Services["svc"]; ok {
		t.Fatalf("service tombstone should delete svc: %+v", got.Services)
	}
	if got.Services["new"].Start[0] != "target" {
		t.Fatalf("missing target service: %+v", got.Services)
	}
	if got.Files.Links[0].Source != "target" {
		t.Fatalf("target files should replace defaults: %+v", got.Files)
	}
	if got.Hooks.PostApply[0].Command != "default" {
		t.Fatalf("omitted target hooks should keep defaults: %+v", got.Hooks)
	}

	got.Env["KEEP"] = EnvVar{Value: "mutated"}
	got.Packages[0].Managers = map[string]string{"brew": "mutated"}
	got.Files.Links[0].Source = "mutated"
	got.Hooks.PostApply[0].Command = "mutated"
	got.Services["new"] = Service{Start: []string{"mutated"}}
	if f.Defaults.Env["KEEP"].Value != "1" ||
		f.Defaults.Packages[0].Managers["brew"] != "default-pkg" ||
		f.Targets["macos"].Files.Links[0].Source != "target" ||
		f.Defaults.Hooks.PostApply[0].Command != "default" ||
		f.Targets["macos"].Services["new"].Start[0] != "target" {
		t.Fatal("MergeTarget mutated or aliased input data")
	}
}

func TestMergeTarget_OmittedArrayFieldsKeepDefaults(t *testing.T) {
	f := &GenvFile{
		SchemaVersion: Version8,
		Defaults: &TargetBundle{
			Packages: []Package{{ID: "git", Prefer: "pacman"}},
			Files: &FilesConfig{
				Dirs: []FileDir{{Target: "~/.config/genv"}},
			},
			Hooks: &HooksConfig{
				PostApply: []Hook{{Command: "echo default"}},
			},
		},
		Targets: map[string]*TargetBundle{
			"linux": {
				Env: map[string]*EnvVar{"TARGET": {Value: "1"}},
			},
		},
	}

	got, err := MergeTarget(f, "linux")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Packages) != 1 || got.Packages[0].ID != "git" {
		t.Fatalf("omitted target packages should keep defaults: %+v", got.Packages)
	}
	if got.Files == nil || len(got.Files.Dirs) != 1 || got.Files.Dirs[0].Target != "~/.config/genv" {
		t.Fatalf("omitted target files should keep defaults: %+v", got.Files)
	}
	if got.Hooks == nil || len(got.Hooks.PostApply) != 1 || got.Hooks.PostApply[0].Command != "echo default" {
		t.Fatalf("omitted target hooks should keep defaults: %+v", got.Hooks)
	}
}

func TestMergeTarget_JSONShellTombstones(t *testing.T) {
	raw := []byte(`{
		"schemaVersion": "8",
		"defaults": {
			"shell": {
				"aliases": {
					"ll": {"value": "ls -la"},
					"gs": {"value": "git status"}
				},
				"functions": {
					"hello": {"body": "echo hello"}
				},
				"source": ["/etc/profile"]
			}
		},
		"targets": {
			"macos": {
				"shell": {
					"aliases": {
						"ll": null,
						"gst": {"value": "git status --short"}
					},
					"functions": {
						"hello": null
					},
					"source": ["~/.zprofile"]
				}
			}
		}
	}`)
	f, errs, err := ParseAndValidate(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(errs) > 0 {
		t.Fatalf("validation: %v", errs)
	}

	got, err := MergeTarget(f, "macos")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := got.Shell.Aliases["ll"]; ok {
		t.Fatal("ll alias should be tombstoned")
	}
	if got.Shell.Aliases["gst"].Value != "git status --short" {
		t.Fatalf("missing target alias: %+v", got.Shell.Aliases)
	}
	if _, ok := got.Shell.Functions["hello"]; ok {
		t.Fatal("hello function should be tombstoned")
	}
	if strings.Join(got.Shell.Source, ",") != "~/.zprofile" {
		t.Fatalf("target shell source should replace defaults: %+v", got.Shell.Source)
	}
}
