package commands

import (
	"errors"
	"testing"

	"github.com/ks1686/genv/internal/schema"
)

func newV8File(targetID string, bundle *schema.TargetBundle) *schema.GenvFile {
	return &schema.GenvFile{
		SchemaVersion: schema.Version8,
		Targets: map[string]*schema.TargetBundle{
			targetID: bundle,
		},
	}
}

func TestActiveBundleRequiresExistingKnownTarget(t *testing.T) {
	f := newV8File("arch", &schema.TargetBundle{})

	if _, err := ActiveBundle(f, "arch"); err != nil {
		t.Fatalf("ActiveBundle existing target: %v", err)
	}
	if _, err := ActiveBundle(f, "ubuntu"); err == nil {
		t.Fatal("expected error for missing known target")
	}
	if _, err := ActiveBundle(f, "solaris"); err == nil {
		t.Fatal("expected error for unknown target")
	}
}

func TestActiveBundleInitializesNilExistingTarget(t *testing.T) {
	f := newV8File("arch", nil)

	bundle, err := ActiveBundle(f, "arch")
	if err != nil {
		t.Fatalf("ActiveBundle nil existing target: %v", err)
	}
	if bundle == nil || f.Targets["arch"] == nil {
		t.Fatalf("expected nil target value to be initialized: %+v", f.Targets)
	}
}

func TestAdd_V8WritesActiveTargetPackages(t *testing.T) {
	f := newV8File("arch", &schema.TargetBundle{})

	if err := Add(f, "git", "2.*", "pacman", nil, "arch"); err != nil {
		t.Fatalf("Add v8: %v", err)
	}

	if len(f.Packages) != 0 {
		t.Fatalf("top-level packages mutated in v8: %+v", f.Packages)
	}
	got := f.Targets["arch"].Packages
	if len(got) != 1 || got[0].ID != "git" || got[0].Version != "2.*" || got[0].Prefer != "pacman" {
		t.Fatalf("unexpected target packages: %+v", got)
	}
}

func TestAdd_V8SeedsNilTargetPackagesFromDefaults(t *testing.T) {
	f := newV8File("arch", &schema.TargetBundle{})
	f.Defaults = &schema.TargetBundle{
		Packages: []schema.Package{{ID: "git", Managers: map[string]string{"pacman": "git"}}},
	}

	if err := Add(f, "jq", "", "", nil, "arch"); err != nil {
		t.Fatalf("Add v8 with defaults: %v", err)
	}

	got := f.Targets["arch"].Packages
	if len(got) != 2 || got[0].ID != "git" || got[1].ID != "jq" {
		t.Fatalf("expected default and added packages, got %+v", got)
	}
	got[0].Managers["pacman"] = "changed"
	if f.Defaults.Packages[0].Managers["pacman"] != "git" {
		t.Fatalf("default package managers were not deep-copied: %+v", f.Defaults.Packages[0].Managers)
	}
}

func TestAdd_V8MissingTargetFails(t *testing.T) {
	f := newV8File("arch", &schema.TargetBundle{})

	err := Add(f, "git", "", "", nil, "ubuntu")
	if err == nil {
		t.Fatal("expected missing target error")
	}
	if len(f.Targets["arch"].Packages) != 0 || f.Targets["ubuntu"] != nil {
		t.Fatalf("missing target should not create or mutate bundles: %+v", f.Targets)
	}
}

func TestRemove_V8RemovesFromActiveTargetPackages(t *testing.T) {
	f := newV8File("arch", &schema.TargetBundle{
		Packages: []schema.Package{{ID: "git"}, {ID: "neovim"}},
	})

	if err := Remove(f, "git", "arch"); err != nil {
		t.Fatalf("Remove v8: %v", err)
	}

	got := f.Targets["arch"].Packages
	if len(got) != 1 || got[0].ID != "neovim" {
		t.Fatalf("unexpected target packages after remove: %+v", got)
	}
}

func TestEnvSetUnset_V8MutatesActiveTargetEnv(t *testing.T) {
	f := newV8File("arch", &schema.TargetBundle{})

	if err := EnvSet(f, "EDITOR", "nvim", false, "arch"); err != nil {
		t.Fatalf("EnvSet v8: %v", err)
	}
	if f.Env != nil {
		t.Fatalf("top-level env mutated in v8: %+v", f.Env)
	}
	if got := f.Targets["arch"].Env["EDITOR"]; got == nil || got.Value != "nvim" {
		t.Fatalf("target env not set: %+v", f.Targets["arch"].Env)
	}

	if err := EnvUnset(f, "EDITOR", "arch"); err != nil {
		t.Fatalf("EnvUnset v8: %v", err)
	}
	if _, ok := f.Targets["arch"].Env["EDITOR"]; ok {
		t.Fatalf("target env entry not removed: %+v", f.Targets["arch"].Env)
	}
}

func TestEnvUnset_V8TombstonesDefault(t *testing.T) {
	editor := schema.EnvVar{Value: "vim"}
	f := newV8File("arch", &schema.TargetBundle{})
	f.Defaults = &schema.TargetBundle{Env: map[string]*schema.EnvVar{"EDITOR": &editor}}

	if err := EnvUnset(f, "EDITOR", "arch"); err != nil {
		t.Fatalf("EnvUnset inherited v8: %v", err)
	}
	if got, ok := f.Targets["arch"].Env["EDITOR"]; !ok || got != nil {
		t.Fatalf("expected EDITOR tombstone, got %+v", f.Targets["arch"].Env)
	}
}

func TestShellAliasSetUnset_V8UsesTargetShellPointerMap(t *testing.T) {
	f := newV8File("arch", &schema.TargetBundle{})

	if err := ShellAliasSet(f, "ll", "ls -la", "bash", "arch"); err != nil {
		t.Fatalf("ShellAliasSet v8: %v", err)
	}
	if f.Shell != nil {
		t.Fatalf("top-level shell mutated in v8: %+v", f.Shell)
	}
	got := f.Targets["arch"].Shell.Aliases["ll"]
	if got == nil || got.Value != "ls -la" || got.Shell != "bash" {
		t.Fatalf("target alias not set: %+v", f.Targets["arch"].Shell.Aliases)
	}

	if err := ShellAliasUnset(f, "ll", "arch"); err != nil {
		t.Fatalf("ShellAliasUnset v8: %v", err)
	}
	if _, ok := f.Targets["arch"].Shell.Aliases["ll"]; ok {
		t.Fatalf("target alias entry not removed: %+v", f.Targets["arch"].Shell.Aliases)
	}
}

func TestShellAliasUnset_V8TombstonesDefault(t *testing.T) {
	alias := schema.ShellAlias{Value: "ls -la"}
	f := newV8File("arch", &schema.TargetBundle{})
	f.Defaults = &schema.TargetBundle{Shell: &schema.TargetShellConfig{
		Aliases: map[string]*schema.ShellAlias{"ll": &alias},
	}}

	if err := ShellAliasUnset(f, "ll", "arch"); err != nil {
		t.Fatalf("ShellAliasUnset inherited v8: %v", err)
	}
	if got, ok := f.Targets["arch"].Shell.Aliases["ll"]; !ok || got != nil {
		t.Fatalf("expected ll tombstone, got %+v", f.Targets["arch"].Shell.Aliases)
	}
}

func TestServiceAddRemove_V8MutatesActiveTargetServices(t *testing.T) {
	f := newV8File("arch", &schema.TargetBundle{})

	if err := ServiceAdd(f, "worker", []string{"worker"}, nil, nil, nil, "", "arch"); err != nil {
		t.Fatalf("ServiceAdd v8: %v", err)
	}
	if f.Services != nil {
		t.Fatalf("top-level services mutated in v8: %+v", f.Services)
	}
	if got := f.Targets["arch"].Services["worker"]; got == nil || got.Start[0] != "worker" {
		t.Fatalf("target service not set: %+v", f.Targets["arch"].Services)
	}

	if err := ServiceRemove(f, "worker", "arch"); err != nil {
		t.Fatalf("ServiceRemove v8: %v", err)
	}
	if _, ok := f.Targets["arch"].Services["worker"]; ok {
		t.Fatalf("target service entry not removed: %+v", f.Targets["arch"].Services)
	}
}

func TestServiceRemove_V8TombstonesDefault(t *testing.T) {
	service := schema.Service{Start: []string{"worker"}}
	f := newV8File("arch", &schema.TargetBundle{})
	f.Defaults = &schema.TargetBundle{Services: map[string]*schema.Service{"worker": &service}}

	if err := ServiceRemove(f, "worker", "arch"); err != nil {
		t.Fatalf("ServiceRemove inherited v8: %v", err)
	}
	if got, ok := f.Targets["arch"].Services["worker"]; !ok || got != nil {
		t.Fatalf("expected worker tombstone, got %+v", f.Targets["arch"].Services)
	}
}

func TestRemove_V8NotTrackedInActiveTarget(t *testing.T) {
	f := newV8File("arch", &schema.TargetBundle{Packages: []schema.Package{{ID: "git"}}})

	err := Remove(f, "git", "ubuntu")
	if err == nil {
		t.Fatal("expected missing target error")
	}
	if errors.Is(err, ErrNotTracked) {
		t.Fatalf("missing target should be reported before package tracking: %v", err)
	}
}
