package host

import (
	"testing"

	"github.com/ks1686/genv/internal/schema"
)

func TestFilterForHost_NilInput(t *testing.T) {
	if FilterForHost(nil, "any") != nil {
		t.Fatal("FilterForHost(nil, host) should return nil")
	}
}

func TestFilterForHost_DoesNotMutateInput(t *testing.T) {
	original := &schema.GenvFile{
		Packages: []schema.Package{
			{ID: "p1", Host: schema.HostPredicate{"macos"}},
			{ID: "p2", Host: schema.HostPredicate{"arch"}},
		},
	}

	filtered := FilterForHost(original, "arch")
	if filtered == original {
		t.Fatal("FilterForHost returned the input pointer, expected a copy")
	}
	if len(original.Packages) != 2 {
		t.Fatalf("input spec was mutated: got %d packages, want 2", len(original.Packages))
	}
}

func TestFilterForHost_Packages(t *testing.T) {
	f := &schema.GenvFile{
		Packages: []schema.Package{
			{ID: "mac", Host: schema.HostPredicate{"macos"}},
			{ID: "arch", Host: schema.HostPredicate{"arch"}},
			{ID: "universal"},
		},
	}

	got := FilterForHost(f, "arch")

	if len(got.Packages) != 2 {
		t.Fatalf("got %d packages, want 2", len(got.Packages))
	}
	if got.Packages[0].ID != "arch" || got.Packages[1].ID != "universal" {
		t.Fatalf("got packages %v, want [arch universal]", got.Packages)
	}
}

func TestFilterForHost_Services(t *testing.T) {
	f := &schema.GenvFile{
		Services: map[string]schema.Service{
			"mac":  {Host: schema.HostPredicate{"macos"}},
			"arch": {Host: schema.HostPredicate{"arch"}},
			"all":  {},
		},
	}

	got := FilterForHost(f, "arch")

	if len(got.Services) != 2 {
		t.Fatalf("got %d services, want 2", len(got.Services))
	}
	if _, ok := got.Services["mac"]; ok {
		t.Fatal("mac service should have been filtered out")
	}
	if _, ok := got.Services["arch"]; !ok {
		t.Fatal("arch service should have been kept")
	}
	if _, ok := got.Services["all"]; !ok {
		t.Fatal("all service should have been kept")
	}
}

func TestFilterForHost_Files(t *testing.T) {
	f := &schema.GenvFile{
		Files: &schema.FilesConfig{
			Links: []schema.FileLink{
				{Source: "mac", Host: schema.HostPredicate{"macos"}},
				{Source: "arch", Host: schema.HostPredicate{"arch"}},
				{Source: "all"},
			},
			Templates: []schema.FileTemplate{
				{Source: "tmpl-mac", Host: schema.HostPredicate{"macos"}},
				{Source: "tmpl-arch", Host: schema.HostPredicate{"arch"}},
			},
			Dirs: []schema.FileDir{
				{Target: "dir-mac", Host: schema.HostPredicate{"macos"}},
				{Target: "dir-arch", Host: schema.HostPredicate{"arch"}},
			},
		},
	}

	got := FilterForHost(f, "arch")

	if len(got.Files.Links) != 2 {
		t.Fatalf("got %d links, want 2", len(got.Files.Links))
	}
	if len(got.Files.Templates) != 1 || got.Files.Templates[0].Source != "tmpl-arch" {
		t.Fatalf("got templates %v, want [tmpl-arch]", got.Files.Templates)
	}
	if len(got.Files.Dirs) != 1 || got.Files.Dirs[0].Target != "dir-arch" {
		t.Fatalf("got dirs %v, want [dir-arch]", got.Files.Dirs)
	}
}

func TestFilterForHost_Hooks(t *testing.T) {
	f := &schema.GenvFile{
		Hooks: &schema.HooksConfig{
			PreUpgrade:  []schema.Hook{{Command: "pre-mac", Host: schema.HostPredicate{"macos"}}, {Command: "pre-arch", Host: schema.HostPredicate{"arch"}}},
			PostApply:   []schema.Hook{{Command: "post-all"}},
			PostUpgrade: []schema.Hook{{Command: "post-mac", Host: schema.HostPredicate{"macos"}}},
		},
	}

	got := FilterForHost(f, "arch")

	if len(got.Hooks.PreUpgrade) != 1 || got.Hooks.PreUpgrade[0].Command != "pre-arch" {
		t.Fatalf("got preUpgrade %v, want [pre-arch]", got.Hooks.PreUpgrade)
	}
	if len(got.Hooks.PostApply) != 1 {
		t.Fatalf("got postApply %v, want [post-all]", got.Hooks.PostApply)
	}
	if len(got.Hooks.PostUpgrade) != 0 {
		t.Fatalf("got postUpgrade %v, want []", got.Hooks.PostUpgrade)
	}
}

func TestFilterForHost_PreservesEnvAndShell(t *testing.T) {
	f := &schema.GenvFile{
		Env: map[string]schema.EnvVar{
			"FOO": {Value: "bar"},
		},
		Shell: &schema.ShellConfig{
			Aliases:   map[string]schema.ShellAlias{"a": {Value: "b"}},
			Functions: map[string]schema.ShellFunction{"f": {Body: "body"}},
			Source:    []string{"/path/to/source"},
		},
	}

	got := FilterForHost(f, "arch")

	if len(got.Env) != 1 || got.Env["FOO"].Value != "bar" {
		t.Fatalf("env was not preserved: %v", got.Env)
	}
	if got.Shell == nil {
		t.Fatal("shell block was dropped")
	}
	if len(got.Shell.Aliases) != 1 || got.Shell.Aliases["a"].Value != "b" {
		t.Fatalf("aliases were not preserved: %v", got.Shell.Aliases)
	}
	if len(got.Shell.Functions) != 1 || got.Shell.Functions["f"].Body != "body" {
		t.Fatalf("functions were not preserved: %v", got.Shell.Functions)
	}
	if len(got.Shell.Source) != 1 || got.Shell.Source[0] != "/path/to/source" {
		t.Fatalf("source was not preserved: %v", got.Shell.Source)
	}
}

func TestFilterForHost_UnknownHostExcludesHostSpecificRecords(t *testing.T) {
	f := &schema.GenvFile{
		Packages: []schema.Package{
			{ID: "specific", Host: schema.HostPredicate{"macos"}},
			{ID: "universal"},
		},
	}

	got := FilterForHost(f, "")

	if len(got.Packages) != 1 || got.Packages[0].ID != "universal" {
		t.Fatalf("got %v, want [universal]", got.Packages)
	}
}
