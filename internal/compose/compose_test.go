package compose

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ks1686/genv/internal/schema"
)

// spec writes a root genv.json in root and returns root.
func spec(t *testing.T, root, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, "genv.json"), []byte(body), 0o600); err != nil {
		t.Fatalf("write spec: %v", err)
	}
}

func mustResolve(t *testing.T, root string, f *schema.GenvFile, target string, extra ...string) *Composition {
	t.Helper()
	sel := append([]string{}, extra...)
	c, err := Resolve(root, root, f, target, sel)
	if err != nil {
		t.Fatalf("Resolve() error: %v", err)
	}
	return c
}

func packageIDs(t *testing.T, c *Composition) []string {
	t.Helper()
	out := []string{}
	for _, p := range c.Effective.Packages {
		out = append(out, p.ID)
	}
	return out
}

func TestSelect_orders_dependencies_first(t *testing.T) {
	docs := map[string]*Module{
		"app":    {Name: "app", Doc: &schema.ModuleDoc{RequiresModules: []string{"base"}}},
		"base":   {Name: "base", Doc: &schema.ModuleDoc{}},
		"unused": {Name: "unused", Doc: &schema.ModuleDoc{}},
	}
	got, err := Select(map[string]string{"app": "a.json", "base": "b.json", "unused": "u.json"}, docs, []string{"app"})
	if err != nil {
		t.Fatalf("Select() error: %v", err)
	}
	want := []string{"base", "app"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("Select() = %v, want %v", got, want)
	}
}

func TestSelect_dedups_dependencies(t *testing.T) {
	docs := map[string]*Module{
		"a":      {Name: "a", Doc: &schema.ModuleDoc{RequiresModules: []string{"shared"}}},
		"b":      {Name: "b", Doc: &schema.ModuleDoc{RequiresModules: []string{"shared"}}},
		"shared": {Name: "shared", Doc: &schema.ModuleDoc{}},
	}
	registered := map[string]string{"a": "a.json", "b": "b.json", "shared": "s.json"}
	got, err := Select(registered, docs, []string{"a", "b"})
	if err != nil {
		t.Fatalf("Select() error: %v", err)
	}
	if len(got) != 3 {
		t.Errorf("Select() = %v, want each module once", got)
	}
	// Dependency-first ordering: the shared dependency is emitted the first time
	// it is reached, so it precedes both dependents.
	if strings.Join(got, ",") != "shared,a,b" {
		t.Errorf("Select() = %v, want shared first then declared order", got)
	}
}

func TestSelect_detects_cycle(t *testing.T) {
	docs := map[string]*Module{
		"a": {Name: "a", Doc: &schema.ModuleDoc{RequiresModules: []string{"b"}}},
		"b": {Name: "b", Doc: &schema.ModuleDoc{RequiresModules: []string{"a"}}},
	}
	_, err := Select(map[string]string{"a": "a.json", "b": "b.json"}, docs, []string{"a"})
	if err == nil {
		t.Fatal("expected cycle error")
	}
	if !errors.Is(err, ErrCycle) {
		t.Errorf("error = %v, want ErrCycle", err)
	}
	var cyc *CycleError
	if !errors.As(err, &cyc) {
		t.Fatalf("error = %v, want *CycleError", err)
	}
	if cyc.Error() != "module dependency cycle: a -> b -> a" {
		t.Errorf("cycle error = %q, want a closed path without a repeated node", cyc.Error())
	}
}

func TestSelect_reports_missing_module(t *testing.T) {
	_, err := Select(map[string]string{"a": "a.json"}, map[string]*Module{}, []string{"ghost"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, ErrMissingModule) {
		t.Errorf("error = %v, want ErrMissingModule", err)
	}
	if !strings.Contains(err.Error(), "ghost") {
		t.Errorf("error should name the module: %v", err)
	}
}

func TestSelect_enforces_depth_limit(t *testing.T) {
	old := maxModuleDepth
	maxModuleDepth = 2
	defer func() { maxModuleDepth = old }()

	docs := map[string]*Module{
		"a": {Name: "a", Doc: &schema.ModuleDoc{RequiresModules: []string{"b"}}},
		"b": {Name: "b", Doc: &schema.ModuleDoc{RequiresModules: []string{"c"}}},
		"c": {Name: "c", Doc: &schema.ModuleDoc{}},
	}
	registered := map[string]string{"a": "a.json", "b": "b.json", "c": "c.json"}
	_, err := Select(registered, docs, []string{"a"})
	if err == nil {
		t.Fatal("expected depth limit error")
	}
	if !errors.Is(err, ErrLimit) {
		t.Errorf("error = %v, want ErrLimit", err)
	}
}

func TestResolve_composes_root_and_two_modules(t *testing.T) {
	root := t.TempDir()
	writeModule(t, root, "modules/base.json", `{
	  "schemaVersion": "10",
	  "defaults": { "packages": [{ "id": "git" }], "env": { "EDITOR": { "value": "nvim" } } },
	  "targets": { "macos": { "packages": [{ "id": "mas-cli", "prefer": "mas" }] } }
	}`)
	writeModule(t, root, "modules/search.json", `{
	  "schemaVersion": "10",
	  "requiresModules": ["base"],
	  "targets": { "macos": { "packages": [{ "id": "ripgrep" }] } }
	}`)
	spec(t, root, `{
	  "schemaVersion": "10",
	  "modules": { "base": "modules/base.json", "search": "modules/search.json" },
	  "targets": { "macos": { "useModules": ["search"], "packages": [{ "id": "jq" }] } }
	}`)

	f, errs, parseErr := schema.ParseAndValidate([]byte(mustRead(t, filepath.Join(root, "genv.json"))))
	if parseErr != nil || len(errs) > 0 {
		t.Fatalf("spec invalid: parseErr=%v errs=%v", parseErr, errs)
	}

	c := mustResolve(t, root, f, "macos")

	ids := strings.Join(packageIDs(t, c), ",")
	// Contributors run root first, then modules in dependency order. Within one
	// contributor the existing overlay rules apply, so base's targets.macos array
	// replaces its defaults array (mas-cli), and the root's own jq comes first.
	if ids != "jq,mas-cli,ripgrep" {
		t.Errorf("packages = %v, want root first then dependency-ordered modules", ids)
	}
	if _, ok := c.Effective.Env["EDITOR"]; !ok {
		t.Error("module defaults.env.EDITOR should reach the effective spec")
	}
	if len(c.SelectedModules) != 2 || c.SelectedModules[0] != "base" || c.SelectedModules[1] != "search" {
		t.Errorf("SelectedModules = %v, want [base search]", c.SelectedModules)
	}
}

func TestResolve_module_absent_target_contributes_defaults_only(t *testing.T) {
	root := t.TempDir()
	writeModule(t, root, "modules/lin.json", `{
	  "schemaVersion": "10",
	  "defaults": { "packages": [{ "id": "jq" }] },
	  "targets": { "macos": { "packages": [{ "id": "mas-cli" }] } }
	}`)
	spec(t, root, `{
	  "schemaVersion": "10",
	  "modules": { "lin": "modules/lin.json" },
	  "targets": { "ubuntu": { "useModules": ["lin"] } }
	}`)
	f, _, _ := schema.ParseAndValidate([]byte(mustRead(t, filepath.Join(root, "genv.json"))))

	c := mustResolve(t, root, f, "ubuntu")
	if got := strings.Join(packageIDs(t, c), ","); got != "jq" {
		t.Errorf("packages = %q, want only the module's defaults", got)
	}
}

func TestResolve_conflicting_services_error(t *testing.T) {
	root := t.TempDir()
	writeModule(t, root, "modules/a.json", `{
	  "schemaVersion": "10",
	  "targets": { "macos": { "services": { "searxng": { "start": ["/bin/sh", "-c", "a"] } } } }
	}`)
	writeModule(t, root, "modules/b.json", `{
	  "schemaVersion": "10",
	  "targets": { "macos": { "services": { "searxng": { "start": ["/bin/sh", "-c", "b"] } } } }
	}`)
	spec(t, root, `{
	  "schemaVersion": "10",
	  "modules": { "a": "modules/a.json", "b": "modules/b.json" },
	  "targets": { "macos": { "useModules": ["a", "b"] } }
	}`)
	f, _, _ := schema.ParseAndValidate([]byte(mustRead(t, filepath.Join(root, "genv.json"))))

	_, err := Resolve(root, root, f, "macos", nil)
	if err == nil {
		t.Fatal("expected conflict error")
	}
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("error = %v, want ErrConflict", err)
	}
	var conflict *ConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("error should carry ConflictError: %v", err)
	}
	if conflict.Identity.Kind != KindService || conflict.Identity.Key != "searxng" {
		t.Errorf("Identity = %v, want service:searxng", conflict.Identity)
	}
	if len(conflict.Origins) != 2 {
		t.Errorf("Origins = %v, want both modules named", conflict.Origins)
	}
	msg := err.Error()
	for _, want := range []string{"modules/a.json", "modules/b.json"} {
		if !strings.Contains(msg, want) {
			t.Errorf("conflict message missing %s: %s", want, msg)
		}
	}
}

func TestResolve_identical_declarations_coalesce_with_both_owners(t *testing.T) {
	root := t.TempDir()
	body := `{"schemaVersion":"10","targets":{"macos":{"services":{"proxy":{"start":["/usr/local/bin/peaproxy"]}}}}}`
	writeModule(t, root, "modules/a.json", body)
	writeModule(t, root, "modules/b.json", body)
	spec(t, root, `{
	  "schemaVersion": "10",
	  "modules": { "a": "modules/a.json", "b": "modules/b.json" },
	  "targets": { "macos": { "useModules": ["a", "b"] } }
	}`)
	f, _, _ := schema.ParseAndValidate([]byte(mustRead(t, filepath.Join(root, "genv.json"))))

	c := mustResolve(t, root, f, "macos")
	svc, ok := c.Effective.Services["proxy"]
	if !ok {
		t.Fatal("shared service missing from effective spec")
	}
	if len(svc.Start) != 1 || svc.Start[0] != "/usr/local/bin/peaproxy" {
		t.Errorf("Start = %v", svc.Start)
	}
	owners := c.Provenance.Owners(serviceIdentity("proxy"))
	if len(owners) != 2 {
		t.Fatalf("owners = %v, want both modules", owners)
	}
	if !c.Provenance.IsModuleOwned(serviceIdentity("proxy")) {
		t.Error("IsModuleOwned should be true for a module-declared service")
	}
}

func TestResolve_root_and_module_conflict_is_reported(t *testing.T) {
	root := t.TempDir()
	writeModule(t, root, "modules/a.json", `{
	  "schemaVersion": "10",
	  "targets": { "macos": { "services": { "proxy": { "start": ["module-start"] } } } }
	}`)
	spec(t, root, `{
	  "schemaVersion": "10",
	  "modules": { "a": "modules/a.json" },
	  "targets": { "macos": { "useModules": ["a"], "services": { "proxy": { "start": ["root-start"] } } } }
	}`)
	f, _, _ := schema.ParseAndValidate([]byte(mustRead(t, filepath.Join(root, "genv.json"))))

	_, err := Resolve(root, root, f, "macos", nil)
	if err == nil {
		t.Fatal("expected conflict: the root is a contributor, not an override")
	}
	if !errors.Is(err, ErrConflict) {
		t.Errorf("error = %v, want ErrConflict", err)
	}
}

func TestResolve_root_tombstone_does_not_delete_module_resource(t *testing.T) {
	root := t.TempDir()
	writeModule(t, root, "modules/a.json", `{
	  "schemaVersion": "10",
	  "defaults": { "env": { "EDITOR": { "value": "nvim" } } }
	}`)
	spec(t, root, `{
	  "schemaVersion": "10",
	  "modules": { "a": "modules/a.json" },
	  "targets": { "macos": { "useModules": ["a"], "env": { "EDITOR": null } } }
	}`)
	f, _, _ := schema.ParseAndValidate([]byte(mustRead(t, filepath.Join(root, "genv.json"))))

	c := mustResolve(t, root, f, "macos")
	if _, ok := c.Effective.Env["EDITOR"]; !ok {
		t.Fatal("module-declared env must survive a root tombstone")
	}
	if len(c.Provenance.Owners(envIdentity("EDITOR"))) != 1 {
		t.Errorf("owners = %v, want the module alone", c.Provenance.Owners(envIdentity("EDITOR")))
	}
}

func TestResolve_root_tombstone_removes_root_declared_resource(t *testing.T) {
	root := t.TempDir()
	spec(t, root, `{
	  "schemaVersion": "10",
	  "defaults": { "env": { "EDITOR": { "value": "vim" } } },
	  "targets": { "macos": { "env": { "EDITOR": null } } }
	}`)
	f, _, _ := schema.ParseAndValidate([]byte(mustRead(t, filepath.Join(root, "genv.json"))))

	c := mustResolve(t, root, f, "macos")
	if _, present := c.Effective.Env["EDITOR"]; present {
		t.Error("a root tombstone must still delete the root's own env entry")
	}
}

func TestResolve_hook_order_root_first_then_modules(t *testing.T) {
	root := t.TempDir()
	writeModule(t, root, "modules/base.json", `{
	  "schemaVersion": "10",
	  "defaults": { "hooks": { "postApply": [{ "command": "base-hook" }] } }
	}`)
	writeModule(t, root, "modules/app.json", `{
	  "schemaVersion": "10",
	  "requiresModules": ["base"],
	  "defaults": { "hooks": { "postApply": [{ "command": "app-hook" }] } }
	}`)
	spec(t, root, `{
	  "schemaVersion": "10",
	  "modules": { "base": "modules/base.json", "app": "modules/app.json" },
	  "defaults": { "hooks": { "postApply": [{ "command": "root-hook" }] } },
	  "targets": { "macos": { "useModules": ["app"] } }
	}`)
	f, _, _ := schema.ParseAndValidate([]byte(mustRead(t, filepath.Join(root, "genv.json"))))

	c := mustResolve(t, root, f, "macos")
	got := []string{}
	for _, h := range c.Effective.Hooks.PostApply {
		got = append(got, h.Command)
	}
	if strings.Join(got, "|") != "root-hook|base-hook|app-hook" {
		t.Errorf("hook order = %v, want root first then dependency order", got)
	}
	// Hooks are positional: each document's hooks stay distinct resources.
	if len(c.Provenance.Owners(hookIdentity("genv.json", "postApply", 0))) != 1 {
		t.Error("root hook should have exactly one owner")
	}
	if len(c.Provenance.Owners(hookIdentity("modules/base.json", "postApply", 0))) != 1 {
		t.Error("base module hook should have exactly one owner")
	}
}

func TestResolve_normalizes_module_asset_paths_to_root(t *testing.T) {
	root := t.TempDir()
	writeModule(t, root, "modules/agent/mods.json", `{
	  "schemaVersion": "10",
	  "targets": { "macos": {
	    "files": { "links": [ { "source": "harnesses/cursor.md", "target": "/Users/me/.cursor/rules.md" } ] },
	    "hooks": { "postApply": [{ "file": "scripts/sync.sh" }] },
	    "services": { "mlx": { "launchd": { "plist": "launchd/mlx.plist" } } }
	  } }
	}`)
	spec(t, root, `{
	  "schemaVersion": "10",
	  "modules": { "agent": "modules/agent/mods.json" },
	  "targets": { "macos": { "useModules": ["agent"] } }
	}`)
	f, _, _ := schema.ParseAndValidate([]byte(mustRead(t, filepath.Join(root, "genv.json"))))

	c := mustResolve(t, root, f, "macos")
	link := c.Effective.Files.Links[0]
	rel := "modules/agent/harnesses/cursor.md"
	if link.Source != rel {
		t.Errorf("link source = %q, want %q resolved against the module document", link.Source, rel)
	}
	if c.Effective.Hooks.PostApply[0].File != "modules/agent/scripts/sync.sh" {
		t.Errorf("hook file = %q", c.Effective.Hooks.PostApply[0].File)
	}
	if c.Effective.Services["mlx"].Launchd.Plist != "modules/agent/launchd/mlx.plist" {
		t.Errorf("plist = %q", c.Effective.Services["mlx"].Launchd.Plist)
	}
	// Origins still point at the module document for ownership and explain.
	origin := c.Provenance.Owners(fileIdentity("/Users/me/.cursor/rules.md"))[0]
	if origin.Module != "agent" || origin.Document != "modules/agent/mods.json" {
		t.Errorf("origin = %+v, want the module document", origin)
	}
}

func TestResolve_rejects_module_asset_escape(t *testing.T) {
	root := t.TempDir()
	writeModule(t, root, "modules/m.json", `{
	  "schemaVersion": "10",
	  "targets": { "macos": {
	    "files": { "links": [ { "source": "../../outside.md", "target": "/Users/me/.x" } ] }
	  } }
	}`)
	spec(t, root, `{
	  "schemaVersion": "10",
	  "modules": { "m": "modules/m.json" },
	  "targets": { "macos": { "useModules": ["m"] } }
	}`)
	f, _, _ := schema.ParseAndValidate([]byte(mustRead(t, filepath.Join(root, "genv.json"))))

	_, err := Resolve(root, root, f, "macos", nil)
	if err == nil {
		t.Fatal("expected error for module asset escaping the root")
	}
	if !errors.Is(err, ErrPathEscape) {
		t.Errorf("error = %v, want ErrPathEscape", err)
	}
}

func TestResolve_file_identity_normalizes_relative_and_absolute(t *testing.T) {
	root := t.TempDir()
	body := `{"schemaVersion":"10","targets":{"macos":{"files":{"links":[{"source":"s","target":"links/../links/x"}]}}}}`
	writeModule(t, root, "modules/a.json", body)
	writeModule(t, root, "modules/b.json", body)
	spec(t, root, `{
	  "schemaVersion": "10",
	  "modules": { "a": "modules/a.json", "b": "modules/b.json" },
	  "targets": { "macos": { "useModules": ["a", "b"] } }
	}`)
	f, _, _ := schema.ParseAndValidate([]byte(mustRead(t, filepath.Join(root, "genv.json"))))

	c := mustResolve(t, root, f, "macos")
	if len(c.Effective.Files.Links) != 1 {
		t.Fatalf("identical normalized links should coalesce, got %d", len(c.Effective.Files.Links))
	}
}

func TestResolve_cross_kind_file_collision_is_reported(t *testing.T) {
	root := t.TempDir()
	writeModule(t, root, "modules/a.json", `{
	  "schemaVersion": "10",
	  "targets": { "macos": {
	    "files": { "links": [ { "source": "s", "target": "/Users/me/shared" } ] }
	  } }
	}`)
	writeModule(t, root, "modules/b.json", `{
	  "schemaVersion": "10",
	  "targets": { "macos": {
	    "files": { "templates": [ { "source": "t", "target": "/Users/me/shared" } ] }
	  } }
	}`)
	spec(t, root, `{
	  "schemaVersion": "10",
	  "modules": { "a": "modules/a.json", "b": "modules/b.json" },
	  "targets": { "macos": { "useModules": ["a", "b"] } }
	}`)
	f, _, _ := schema.ParseAndValidate([]byte(mustRead(t, filepath.Join(root, "genv.json"))))

	_, err := Resolve(root, root, f, "macos", nil)
	if err == nil {
		t.Fatal("expected collision error: a link and a template cannot own one destination")
	}
	if !errors.Is(err, ErrConflict) {
		t.Errorf("error = %v, want ErrConflict", err)
	}
}

func TestResolve_package_conflict_on_same_id_different_prefer(t *testing.T) {
	root := t.TempDir()
	writeModule(t, root, "modules/a.json", `{
	  "schemaVersion": "10",
	  "targets": { "macos": { "packages": [{ "id": "git", "prefer": "brew" }] } }
	}`)
	writeModule(t, root, "modules/b.json", `{
	  "schemaVersion": "10",
	  "targets": { "macos": { "packages": [{ "id": "git", "prefer": "mas" }] } }
	}`)
	spec(t, root, `{
	  "schemaVersion": "10",
	  "modules": { "a": "modules/a.json", "b": "modules/b.json" },
	  "targets": { "macos": { "useModules": ["a", "b"] } }
	}`)
	f, _, _ := schema.ParseAndValidate([]byte(mustRead(t, filepath.Join(root, "genv.json"))))

	_, err := Resolve(root, root, f, "macos", nil)
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("error = %v, want ErrConflict", err)
	}
}

func TestResolve_duplicates_identical_packages(t *testing.T) {
	root := t.TempDir()
	body := `{"schemaVersion":"10","targets":{"macos":{"packages":[{"id":"git","prefer":"brew"}]}}}`
	writeModule(t, root, "modules/a.json", body)
	writeModule(t, root, "modules/b.json", body)
	spec(t, root, `{
	  "schemaVersion": "10",
	  "modules": { "a": "modules/a.json", "b": "modules/b.json" },
	  "targets": { "macos": { "useModules": ["a", "b"] } }
	}`)
	f, _, _ := schema.ParseAndValidate([]byte(mustRead(t, filepath.Join(root, "genv.json"))))

	c := mustResolve(t, root, f, "macos")
	if got := strings.Join(packageIDs(t, c), ","); got != "git" {
		t.Errorf("packages = %q, want a single coalesced entry", got)
	}
	if len(c.Provenance.Owners(packageIdentity("git"))) != 2 {
		t.Errorf("owners = %v, want both modules", c.Provenance.Owners(packageIdentity("git")))
	}
}

func TestResolve_v8_spec_returns_flattened_spec(t *testing.T) {
	root := t.TempDir()
	spec(t, root, `{
	  "schemaVersion": "8",
	  "defaults": { "packages": [{ "id": "git" }] },
	  "targets": { "macos": { "packages": [{ "id": "jq" }] } }
	}`)
	f, _, _ := schema.ParseAndValidate([]byte(mustRead(t, filepath.Join(root, "genv.json"))))

	c := mustResolve(t, root, f, "macos")
	// v8 array semantics are replace-not-append: the target's package array wins
	// and defaults.packages is not concatenated. Composition must not change it.
	if got := strings.Join(packageIDs(t, c), ","); got != "jq" {
		t.Errorf("packages = %q, want the plain v8 merge (target array replaces defaults)", got)
	}
	if len(c.SelectedModules) != 0 {
		t.Errorf("SelectedModules = %v, want empty for v8", c.SelectedModules)
	}
}

func TestResolve_explicit_selection_extends_bucket_selection(t *testing.T) {
	root := t.TempDir()
	writeModule(t, root, "modules/a.json", `{"schemaVersion":"10","defaults":{"packages":[{"id":"a"}]}}`)
	writeModule(t, root, "modules/b.json", `{"schemaVersion":"10","defaults":{"packages":[{"id":"b"}]}}`)
	spec(t, root, `{
	  "schemaVersion": "10",
	  "modules": { "a": "modules/a.json", "b": "modules/b.json" },
	  "targets": { "macos": { "useModules": ["a"] } }
	}`)
	f, _, _ := schema.ParseAndValidate([]byte(mustRead(t, filepath.Join(root, "genv.json"))))

	c := mustResolve(t, root, f, "macos", "b")
	if got := strings.Join(packageIDs(t, c), ","); got != "a,b" {
		t.Errorf("packages = %q, want bucket selection then explicit selection", got)
	}
}

func TestResolve_env_redeclare_conflict_is_reported(t *testing.T) {
	root := t.TempDir()
	writeModule(t, root, "modules/a.json", `{
	  "schemaVersion": "10",
	  "defaults": { "env": { "EDITOR": { "value": "nvim" } } }
	}`)
	writeModule(t, root, "modules/b.json", `{
	  "schemaVersion": "10",
	  "defaults": { "env": { "EDITOR": { "value": "vim" } } }
	}`)
	spec(t, root, `{
	  "schemaVersion": "10",
	  "modules": { "a": "modules/a.json", "b": "modules/b.json" },
	  "targets": { "macos": { "useModules": ["a", "b"] } }
	}`)
	f, _, _ := schema.ParseAndValidate([]byte(mustRead(t, filepath.Join(root, "genv.json"))))

	_, err := Resolve(root, root, f, "macos", nil)
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("error = %v, want ErrConflict", err)
	}
	if strings.Contains(err.Error(), "nvim") || strings.Contains(err.Error(), "vim\"") {
		t.Errorf("conflict message must not print env values: %v", err)
	}
}

func TestFingerprint_excludes_env_values(t *testing.T) {
	root := t.TempDir()
	spec(t, root, `{
	  "schemaVersion": "10",
	  "defaults": { "env": { "SECRET_ONE": { "value": "alpha", "sensitive": true } } },
	  "targets": { "macos": {} }
	}`)
	f, _, _ := schema.ParseAndValidate([]byte(mustRead(t, filepath.Join(root, "genv.json"))))
	first := mustResolve(t, root, f, "macos").Fingerprint

	spec(t, root, `{
	  "schemaVersion": "10",
	  "defaults": { "env": { "SECRET_ONE": { "value": "beta", "sensitive": true } } },
	  "targets": { "macos": {} }
	}`)
	f2, _, _ := schema.ParseAndValidate([]byte(mustRead(t, filepath.Join(root, "genv.json"))))
	second := mustResolve(t, root, f2, "macos").Fingerprint

	if first != second {
		t.Error("fingerprint must not change when only an env value changes")
	}
}

// The fingerprint goes into the lock file and into output, so it must never
// carry a value of any kind. Only the name and the sensitive flag are hashed.
func TestFingerprint_excludes_non_sensitive_env_values_too(t *testing.T) {
	root := t.TempDir()
	spec(t, root, `{
	  "schemaVersion": "10",
	  "defaults": { "env": { "EDITOR": { "value": "nvim" } } },
	  "targets": { "macos": {} }
	}`)
	f, _, _ := schema.ParseAndValidate([]byte(mustRead(t, filepath.Join(root, "genv.json"))))
	first := mustResolve(t, root, f, "macos").Fingerprint

	spec(t, root, `{
	  "schemaVersion": "10",
	  "defaults": { "env": { "EDITOR": { "value": "emacs" } } },
	  "targets": { "macos": {} }
	}`)
	f2, _, _ := schema.ParseAndValidate([]byte(mustRead(t, filepath.Join(root, "genv.json"))))
	second := mustResolve(t, root, f2, "macos").Fingerprint

	if first != second {
		t.Error("fingerprint must not change when only an env value changes")
	}
	// The value must not appear anywhere in the fingerprint string either.
	if strings.Contains(first, "nvim") || strings.Contains(first, "emacs") {
		t.Errorf("fingerprint leaks an env value: %s", first)
	}
}

func TestFingerprint_changes_with_structure(t *testing.T) {
	root := t.TempDir()
	spec(t, root, `{"schemaVersion":"10","targets":{"macos":{"packages":[{"id":"git"}]}}}`)
	f, _, _ := schema.ParseAndValidate([]byte(mustRead(t, filepath.Join(root, "genv.json"))))
	first := mustResolve(t, root, f, "macos").Fingerprint

	spec(t, root, `{"schemaVersion":"10","targets":{"macos":{"packages":[{"id":"git"},{"id":"jq"}]}}}`)
	f2, _, _ := schema.ParseAndValidate([]byte(mustRead(t, filepath.Join(root, "genv.json"))))
	second := mustResolve(t, root, f2, "macos").Fingerprint

	if first == second {
		t.Error("fingerprint must change when the resource set changes")
	}
}

func TestFingerprint_changes_with_selection(t *testing.T) {
	root := t.TempDir()
	writeModule(t, root, "modules/a.json", `{"schemaVersion":"10","defaults":{"packages":[{"id":"a"}]}}`)
	spec(t, root, `{
	  "schemaVersion": "10",
	  "modules": { "a": "modules/a.json" },
	  "targets": { "macos": {} }
	}`)
	f, _, _ := schema.ParseAndValidate([]byte(mustRead(t, filepath.Join(root, "genv.json"))))
	base := mustResolve(t, root, f, "macos").Fingerprint
	withModule := mustResolve(t, root, f, "macos", "a").Fingerprint

	if base == withModule {
		t.Error("fingerprint must change when module selection changes")
	}
}

func TestFingerprint_is_stable_across_runs(t *testing.T) {
	root := t.TempDir()
	writeModule(t, root, "modules/a.json", `{"schemaVersion":"10","defaults":{"packages":[{"id":"a"}]}}`)
	spec(t, root, `{
	  "schemaVersion": "10",
	  "modules": { "a": "modules/a.json" },
	  "targets": { "macos": { "useModules": ["a"] } }
	}`)
	f, _, _ := schema.ParseAndValidate([]byte(mustRead(t, filepath.Join(root, "genv.json"))))
	first := mustResolve(t, root, f, "macos").Fingerprint
	second := mustResolve(t, root, f, "macos").Fingerprint
	if first != second {
		t.Error("fingerprint must be deterministic for the same inputs")
	}
}

func TestFingerprint_ignores_contributor_order(t *testing.T) {
	rootA := t.TempDir()
	rootB := t.TempDir()
	bodyA := `{"schemaVersion":"10","defaults":{"packages":[{"id":"a"}]}}`
	bodyB := `{"schemaVersion":"10","defaults":{"packages":[{"id":"b"}]}}`
	for _, root := range []string{rootA, rootB} {
		writeModule(t, root, "modules/a.json", bodyA)
		writeModule(t, root, "modules/b.json", bodyB)
	}
	rootSpecJSON := `{
	  "schemaVersion": "10",
	  "modules": { "a": "modules/a.json", "b": "modules/b.json" },
	  "targets": { "macos": { "useModules": ["a", "b"] } }
	}`
	spec(t, rootA, rootSpecJSON)
	spec(t, rootB, rootSpecJSON)

	fa, _, _ := schema.ParseAndValidate([]byte(rootSpecJSON))
	fb, _, _ := schema.ParseAndValidate([]byte(rootSpecJSON))
	if mustResolve(t, rootA, fa, "macos").Fingerprint != mustResolve(t, rootB, fb, "macos").Fingerprint {
		t.Error("fingerprint must not depend on filesystem paths or map order")
	}
}

func TestProvenance_identities_sorted(t *testing.T) {
	root := t.TempDir()
	writeModule(t, root, "modules/a.json", `{
	  "schemaVersion": "10",
	  "targets": { "macos": {
	    "packages": [{ "id": "git" }],
	    "services": { "proxy": { "start": ["run"] } },
	    "env": { "EDITOR": { "value": "nvim" } }
	  } }
	}`)
	spec(t, root, `{
	  "schemaVersion": "10",
	  "modules": { "a": "modules/a.json" },
	  "targets": { "macos": { "useModules": ["a"] } }
	}`)
	f, _, _ := schema.ParseAndValidate([]byte(mustRead(t, filepath.Join(root, "genv.json"))))

	c := mustResolve(t, root, f, "macos")
	ids := []string{}
	for _, id := range c.Provenance.Identities() {
		ids = append(ids, id.String())
	}
	want := "env:EDITOR,package:git,service:proxy"
	if strings.Join(ids, ",") != want {
		t.Errorf("Identities() = %v, want %s", ids, want)
	}
}

func TestResolve_reports_missing_target(t *testing.T) {
	root := t.TempDir()
	spec(t, root, `{"schemaVersion":"10","targets":{"macos":{}}}`)
	f, _, _ := schema.ParseAndValidate([]byte(mustRead(t, filepath.Join(root, "genv.json"))))

	if _, err := Resolve(root, root, f, "ubuntu", nil); err == nil {
		t.Fatal("expected error: v10 keeps the existing no-fallback target rule")
	}
}

func TestResolve_module_requires_unknown_module(t *testing.T) {
	root := t.TempDir()
	writeModule(t, root, "modules/a.json", `{"schemaVersion":"10","requiresModules":["ghost"],"defaults":{}}`)
	spec(t, root, `{
	  "schemaVersion": "10",
	  "modules": { "a": "modules/a.json" },
	  "targets": { "macos": { "useModules": ["a"] } }
	}`)
	f, _, _ := schema.ParseAndValidate([]byte(mustRead(t, filepath.Join(root, "genv.json"))))

	_, err := Resolve(root, root, f, "macos", nil)
	if !errors.Is(err, ErrMissingModule) {
		t.Fatalf("error = %v, want ErrMissingModule", err)
	}
	if !strings.Contains(err.Error(), "ghost") {
		t.Errorf("error should name the missing dependency: %v", err)
	}
}

func TestResolve_selected_modules_sorted_stable(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"a", "b", "c"} {
		writeModule(t, root, "modules/"+name+".json",
			`{"schemaVersion":"10","defaults":{"packages":[{"id":"`+name+`"}]}}`)
	}
	spec(t, root, `{
	  "schemaVersion": "10",
	  "modules": { "a": "modules/a.json", "b": "modules/b.json", "c": "modules/c.json" },
	  "targets": { "macos": { "useModules": ["c", "a"] } }
	}`)
	f, _, _ := schema.ParseAndValidate([]byte(mustRead(t, filepath.Join(root, "genv.json"))))

	c := mustResolve(t, root, f, "macos", "b")
	want := []string{"c", "a", "b"}
	if strings.Join(c.SelectedModules, ",") != strings.Join(want, ",") {
		t.Errorf("SelectedModules = %v, want declared order %v", c.SelectedModules, want)
	}
	if got := strings.Join(packageIDs(t, c), ","); got != "c,a,b" {
		t.Errorf("packages = %q, want contribution order %v", got, want)
	}
}

func TestComposition_effective_is_not_shared_with_spec(t *testing.T) {
	root := t.TempDir()
	writeModule(t, root, "modules/a.json", `{"schemaVersion":"10","defaults":{"packages":[{"id":"a"}]}}`)
	spec(t, root, `{
	  "schemaVersion": "10",
	  "modules": { "a": "modules/a.json" },
	  "targets": { "macos": { "useModules": ["a"], "packages": [{ "id": "root-pkg" }] } }
	}`)
	f, _, _ := schema.ParseAndValidate([]byte(mustRead(t, filepath.Join(root, "genv.json"))))
	before := len(f.Targets["macos"].Packages)

	c := mustResolve(t, root, f, "macos")
	if len(f.Targets["macos"].Packages) != before {
		t.Error("Resolve must not mutate the raw spec")
	}
	if len(f.Targets["macos"].Packages) == len(c.Effective.Packages) {
		t.Error("effective spec should be a distinct, merged value")
	}
}

func mustRead(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

var _ = json.Marshal

// parseSpec parses a spec file and fails the test on validation errors.
func parseSpec(t *testing.T, path string) *schema.GenvFile {
	t.Helper()
	f, errs, err := schema.ParseAndValidate([]byte(mustRead(t, path)))
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	if len(errs) > 0 {
		t.Fatalf("validate %s: %v", path, errs)
	}
	return f
}

// --- regressions from review -------------------------------------------------
//
// Each test below pins behavior that the v8 merge already had, or that the v10
// feature documents. A module-free spec must not compose differently from the
// same spec read as v8.

func TestResolve_module_free_spec_reports_root_ownership(t *testing.T) {
	root := t.TempDir()
	spec(t, root, `{
	  "schemaVersion": "8",
	  "defaults": { "packages": [{ "id": "git" }] },
	  "targets": { "macos": {
	    "packages": [{ "id": "jq" }],
	    "services": { "searxng": { "start": ["searxng"] } }
	  } }
	}`)
	f, _, _ := schema.ParseAndValidate([]byte(mustRead(t, filepath.Join(root, "genv.json"))))

	c := mustResolve(t, root, f, "macos")
	for _, tc := range []struct {
		id    Identity
		field string
	}{
		{Identity{Kind: KindPackage, Key: "jq"}, "targets.macos.packages[jq]"},
		{Identity{Kind: KindService, Key: "searxng"}, "targets.macos.services.searxng"},
	} {
		owners := c.Provenance.Owners(tc.id)
		if len(owners) == 0 {
			t.Errorf("%s has no owner; genv explain would report it as unknown", tc.id)
			continue
		}
		if owners[0].Document != RootDocument || owners[0].Module != "" {
			t.Errorf("%s owner = %+v, want the root document", tc.id, owners[0])
		}
		if owners[0].Field != tc.field {
			t.Errorf("%s field = %q, want %q", tc.id, owners[0].Field, tc.field)
		}
	}
}

func TestResolve_module_free_defaults_are_attributed_to_defaults(t *testing.T) {
	// Ownership names the block that declares the resource, so `genv explain`
	// sends the user to the file they have to edit.
	root := t.TempDir()
	spec(t, root, `{
	  "schemaVersion": "8",
	  "defaults": {
	    "env": { "EDITOR": { "value": "nvim" } },
	    "services": { "cache": { "start": ["redis-server"] } }
	  },
	  "targets": { "macos": { "packages": [{ "id": "jq" }] } }
	}`)
	f, _, _ := schema.ParseAndValidate([]byte(mustRead(t, filepath.Join(root, "genv.json"))))

	c := mustResolve(t, root, f, "macos")
	for _, tc := range []struct {
		id    Identity
		field string
	}{
		{Identity{Kind: KindEnv, Key: "EDITOR"}, "defaults.env.EDITOR"},
		{Identity{Kind: KindService, Key: "cache"}, "defaults.services.cache"},
		{Identity{Kind: KindPackage, Key: "jq"}, "targets.macos.packages[jq]"},
	} {
		owners := c.Provenance.Owners(tc.id)
		if len(owners) != 1 {
			t.Errorf("%s owners = %+v, want exactly one", tc.id, owners)
			continue
		}
		if owners[0].Field != tc.field {
			t.Errorf("%s field = %q, want %q", tc.id, owners[0].Field, tc.field)
		}
	}
}

func TestResolve_keeps_shell_source_from_every_contributor(t *testing.T) {
	root := t.TempDir()
	writeModule(t, root, "modules/base.json", `{
	  "schemaVersion": "10",
	  "defaults": { "shell": { "source": ["modules/base.sh"] } }
	}`)
	spec(t, root, `{
	  "schemaVersion": "10",
	  "modules": { "base": "modules/base.json" },
	  "defaults": { "shell": { "source": ["root.sh"] } },
	  "targets": { "macos": { "useModules": ["base"] } }
	}`)
	f, _, _ := schema.ParseAndValidate([]byte(mustRead(t, filepath.Join(root, "genv.json"))))

	c := mustResolve(t, root, f, "macos")
	got := c.Effective.Shell.Source
	if len(got) != 2 {
		t.Fatalf("shell.source = %v, want both contributors' source files (root.sh, modules/base.sh)", got)
	}
}

func TestResolve_shell_source_survives_without_aliases_or_functions(t *testing.T) {
	// A contributor that declares only shell.source still has to reach the
	// effective spec: returning nil would silently drop every sourced file.
	root := t.TempDir()
	spec(t, root, `{
	  "schemaVersion": "8",
	  "defaults": { "shell": { "source": ["lib/helpers.sh"] } },
	  "targets": { "macos": {} }
	}`)
	f, _, _ := schema.ParseAndValidate([]byte(mustRead(t, filepath.Join(root, "genv.json"))))

	c := mustResolve(t, root, f, "macos")
	if c.Effective.Shell == nil || len(c.Effective.Shell.Source) != 1 {
		t.Fatalf("effective shell = %+v, want the sourced file", c.Effective.Shell)
	}
}

func TestResolve_empty_target_package_array_clears_defaults(t *testing.T) {
	// v8 semantics: a non-nil array in a target replaces defaults, even when it
	// is empty. Composition must agree, or the same spec means two things.
	root := t.TempDir()
	spec(t, root, `{
	  "schemaVersion": "8",
	  "defaults": { "packages": [{ "id": "git" }, { "id": "jq" }] },
	  "targets": { "macos": { "packages": [] } }
	}`)
	f, _, _ := schema.ParseAndValidate([]byte(mustRead(t, filepath.Join(root, "genv.json"))))

	c := mustResolve(t, root, f, "macos")
	if got := packageIDs(t, c); len(got) != 0 {
		t.Errorf("packages = %v, want empty: an empty target array replaces defaults", got)
	}
}

func TestResolve_empty_target_file_array_clears_defaults(t *testing.T) {
	root := t.TempDir()
	spec(t, root, `{
	  "schemaVersion": "8",
	  "defaults": { "files": { "links": [{ "source": "a", "target": "b" }] } },
	  "targets": { "macos": { "files": { "links": [] } } }
	}`)
	f, _, _ := schema.ParseAndValidate([]byte(mustRead(t, filepath.Join(root, "genv.json"))))

	c := mustResolve(t, root, f, "macos")
	if c.Effective.Files != nil && len(c.Effective.Files.Links) != 0 {
		t.Errorf("files.links = %+v, want empty: an empty target array replaces defaults", c.Effective.Files.Links)
	}
}

func TestResolve_empty_target_hook_phase_clears_defaults(t *testing.T) {
	root := t.TempDir()
	spec(t, root, `{
	  "schemaVersion": "8",
	  "defaults": { "hooks": { "postApply": [{ "name": "hello", "command": "echo hi" }] } },
	  "targets": { "macos": { "hooks": { "postApply": [] } } }
	}`)
	f, _, _ := schema.ParseAndValidate([]byte(mustRead(t, filepath.Join(root, "genv.json"))))

	c := mustResolve(t, root, f, "macos")
	if n := len(phaseSlice(c.Effective.Hooks, "postApply")); n != 0 {
		t.Errorf("postApply hooks = %d, want 0: an empty target phase replaces defaults", n)
	}
}

// The cases below run through the module path (schemaVersion 10 with a module
// selected), which is where composition has its own defaults->overlay merge
// step. That step has to agree with schema.MergeTarget, or the same spec means
// one thing when it uses modules and another when it does not.

func TestResolve_module_path_empty_target_package_array_clears_root_defaults(t *testing.T) {
	root := t.TempDir()
	writeModule(t, root, "modules/tool.json", `{
	  "schemaVersion": "10",
	  "defaults": { "packages": [{ "id": "curl" }] }
	}`)
	spec(t, root, `{
	  "schemaVersion": "10",
	  "modules": { "tool": "modules/tool.json" },
	  "defaults": { "packages": [{ "id": "git" }, { "id": "jq" }] },
	  "targets": { "macos": { "useModules": ["tool"], "packages": [] } }
	}`)
	f, _, _ := schema.ParseAndValidate([]byte(mustRead(t, filepath.Join(root, "genv.json"))))

	c := mustResolve(t, root, f, "macos")
	for _, id := range packageIDs(t, c) {
		if id == "git" || id == "jq" {
			t.Errorf("packages = %v, want the root's empty target array to clear its own defaults; only the module's curl should remain", packageIDs(t, c))
			break
		}
	}
}

func TestResolve_module_path_empty_target_hook_phase_clears_root_defaults(t *testing.T) {
	root := t.TempDir()
	writeModule(t, root, "modules/tool.json", `{"schemaVersion":"10","defaults":{}}`)
	spec(t, root, `{
	  "schemaVersion": "10",
	  "modules": { "tool": "modules/tool.json" },
	  "defaults": { "hooks": { "postApply": [{ "name": "hello", "command": "echo hi" }] } },
	  "targets": { "macos": { "useModules": ["tool"], "hooks": { "postApply": [] } } }
	}`)
	f, _, _ := schema.ParseAndValidate([]byte(mustRead(t, filepath.Join(root, "genv.json"))))

	c := mustResolve(t, root, f, "macos")
	if n := len(phaseSlice(c.Effective.Hooks, "postApply")); n != 0 {
		t.Errorf("postApply hooks = %d, want 0", n)
	}
}

func TestResolve_module_path_hooks_from_two_contributors_both_run(t *testing.T) {
	// Replacing is within one document. Across contributors hooks accumulate,
	// otherwise a module's post-apply hook could silently vanish.
	root := t.TempDir()
	writeModule(t, root, "modules/tool.json", `{
	  "schemaVersion": "10",
	  "defaults": { "hooks": { "postApply": [{ "name": "mod", "command": "echo mod" }] } }
	}`)
	spec(t, root, `{
	  "schemaVersion": "10",
	  "modules": { "tool": "modules/tool.json" },
	  "defaults": { "hooks": { "postApply": [{ "name": "root", "command": "echo root" }] } },
	  "targets": { "macos": { "useModules": ["tool"] } }
	}`)
	f, _, _ := schema.ParseAndValidate([]byte(mustRead(t, filepath.Join(root, "genv.json"))))

	c := mustResolve(t, root, f, "macos")
	if n := len(phaseSlice(c.Effective.Hooks, "postApply")); n != 2 {
		names := []string{}
		for _, h := range phaseSlice(c.Effective.Hooks, "postApply") {
			names = append(names, h.Name)
		}
		t.Errorf("postApply hooks = %v, want both contributors' hooks", names)
	}
}

func TestResolve_module_path_empty_target_file_array_clears_root_defaults(t *testing.T) {
	root := t.TempDir()
	writeModule(t, root, "modules/tool.json", `{"schemaVersion":"10","defaults":{}}`)
	spec(t, root, `{
	  "schemaVersion": "10",
	  "modules": { "tool": "modules/tool.json" },
	  "defaults": { "files": { "links": [{ "source": "a", "target": "b" }] } },
	  "targets": { "macos": { "useModules": ["tool"], "files": { "links": [] } } }
	}`)
	f, _, _ := schema.ParseAndValidate([]byte(mustRead(t, filepath.Join(root, "genv.json"))))

	c := mustResolve(t, root, f, "macos")
	if c.Effective.Files != nil && len(c.Effective.Files.Links) != 0 {
		t.Errorf("files.links = %+v, want empty", c.Effective.Files.Links)
	}
}

func TestResolve_module_path_shell_source_replaces_root_array(t *testing.T) {
	root := t.TempDir()
	writeModule(t, root, "modules/tool.json", `{
	  "schemaVersion": "10",
	  "defaults": { "shell": { "source": ["modules/tool.sh"] } }
	}`)
	spec(t, root, `{
	  "schemaVersion": "10",
	  "modules": { "tool": "modules/tool.json" },
	  "defaults": { "shell": { "source": ["root.sh"] } },
	  "targets": { "macos": { "useModules": ["tool"], "shell": { "source": [] } } }
	}`)
	f, _, _ := schema.ParseAndValidate([]byte(mustRead(t, filepath.Join(root, "genv.json"))))

	c := mustResolve(t, root, f, "macos")
	for _, s := range c.Effective.Shell.Source {
		if s == "root.sh" {
			t.Fatalf("shell.source = %v, want the empty target array to clear the root's source", c.Effective.Shell.Source)
		}
	}
}
