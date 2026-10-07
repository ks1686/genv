package compose

import (
	"errors"
	"strings"
	"testing"

	"github.com/ks1686/genv/internal/schema"
)

func TestIdentityFromDisplay(t *testing.T) {
	cases := map[string]struct {
		in    string
		kind  string
		key   string
		valid bool
	}{
		"service":            {"service:searxng", "service", "searxng", true},
		"package":            {"package:ripgrep", "package", "ripgrep", true},
		"absolute file path": {"file:/Users/me/.zshrc", "file", "/Users/me/.zshrc", true},
		"windows path":       {"file:C:\\Users\\me\\.zshrc", "file", "C:\\Users\\me\\.zshrc", true},
		"no colon":           {"searxng", "", "", false},
		"empty kind":         {":searxng", "", "", false},
		"empty key":          {"service:", "", "", false},
		"empty":              {"", "", "", false},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			id, ok := IdentityFromDisplay(tc.in)
			if ok != tc.valid {
				t.Fatalf("IdentityFromDisplay(%q) ok = %v, want %v", tc.in, ok, tc.valid)
			}
			if !tc.valid {
				return
			}
			if id.Kind != tc.kind || id.Key != tc.key {
				t.Errorf("IdentityFromDisplay(%q) = %+v, want %s/%s", tc.in, id, tc.kind, tc.key)
			}
		})
	}
}

func TestIdentity_roundtrips_through_display(t *testing.T) {
	for _, id := range []Identity{
		serviceIdentity("searxng"),
		packageIdentity("ripgrep"),
		fileIdentity("/Users/me/.config/genv/env.sh"),
	} {
		parsed, ok := IdentityFromDisplay(id.String())
		if !ok {
			t.Fatalf("IdentityFromDisplay(%q) failed", id)
		}
		if parsed != id {
			t.Errorf("round trip = %+v, want %+v", parsed, id)
		}
	}
}

func TestIdentityString_without_key(t *testing.T) {
	if got := (Identity{Kind: KindHook}).String(); got != "hook" {
		t.Errorf("String() = %q, want %q", got, "hook")
	}
}

func TestCleanPath_normalizes_without_following_symlinks(t *testing.T) {
	cases := map[string]string{
		"a/./b":              "a/b",
		"a/b/../c":           "a/c",
		"a//b":               "a/b",
		"./a":                "a",
		"":                   "",
		"/abs/path":          "/abs/path",
		"C:\\Users\\me":      "C:\\Users\\me",
		"relative/../escape": "escape",
	}
	for in, want := range cases {
		if got := CleanPath(in); got != want {
			t.Errorf("CleanPath(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCleanPath_windows_style_stays_windows_style(t *testing.T) {
	// A Windows destination composed on a Unix planning host must not be
	// rewritten into POSIX form, or two Windows declarations would compare
	// unequal on a Linux CI runner.
	got := CleanPath("C:\\Users\\me\\.zshrc")
	if strings.Contains(got, "/") {
		t.Errorf("CleanPath() = %q, want backslash separators preserved", got)
	}
}

func TestFileIdentity_coalesces_equivalent_paths(t *testing.T) {
	if fileIdentity("/a/b/../c") != fileIdentity("/a/c") {
		t.Error("equivalent paths must share one identity")
	}
	if fileIdentity("a/b") == fileIdentity("a/c") {
		t.Error("different destinations must not share an identity")
	}
}

func TestHookIdentity_is_positional(t *testing.T) {
	a := hookIdentity("modules/a.json", "postApply", 0)
	b := hookIdentity("modules/b.json", "postApply", 0)
	c := hookIdentity("modules/a.json", "preApply", 0)
	if a == b || a == c {
		t.Error("hooks must be distinct by document, phase, and index")
	}
}

func TestProvenance_String_lists_owners(t *testing.T) {
	p := newProvenance()
	p.add(packageIdentity("git"), Origin{Document: RootDocument, Field: "targets.macos.packages", Target: "macos"})
	p.add(packageIdentity("git"), Origin{Document: "modules/base.json", Field: "targets.macos.packages", Module: "base", Target: "macos"})

	got := p.String()
	for _, want := range []string{"package:git", RootDocument, "modules/base.json", "(module base)"} {
		if !strings.Contains(got, want) {
			t.Errorf("String() missing %q: %s", want, got)
		}
	}
}

func TestProvenance_Modules_lists_module_owners(t *testing.T) {
	p := newProvenance()
	p.add(packageIdentity("git"), Origin{Document: RootDocument, Field: "targets.macos.packages", Target: "macos"})
	p.add(packageIdentity("jq"), Origin{Document: "modules/a.json", Field: "defaults.packages", Module: "a"})
	p.add(serviceIdentity("s"), Origin{Document: "modules/b.json", Field: "targets.macos.services.s", Module: "b"})

	got := strings.Join(p.Modules(), ",")
	if got != "a,b" {
		t.Errorf("Modules() = %q, want sorted module owners only", got)
	}
}

func TestProvenance_add_is_idempotent(t *testing.T) {
	p := newProvenance()
	origin := Origin{Document: "modules/a.json", Field: "defaults.packages", Module: "a"}
	p.add(packageIdentity("git"), origin)
	p.add(packageIdentity("git"), origin)
	if len(p.Owners(packageIdentity("git"))) != 1 {
		t.Errorf("duplicate origin should be recorded once, got %v", p.Owners(packageIdentity("git")))
	}
}

func TestProvenance_nil_is_safe(t *testing.T) {
	var p *Provenance
	if p.Owners(packageIdentity("git")) != nil {
		t.Error("Owners on nil provenance should be nil")
	}
	if p.IsModuleOwned(packageIdentity("git")) {
		t.Error("IsModuleOwned on nil provenance should be false")
	}
}

func TestFingerprint_empty_selection_and_nil_spec(t *testing.T) {
	if got := Fingerprint(nil, nil); got != "" {
		t.Errorf("Fingerprint(nil) = %q, want empty", got)
	}
}

func TestFingerprint_ignores_env_value_but_tracks_name(t *testing.T) {
	base := func(envName string) string {
		spec := &schema.GenvFile{
			SchemaVersion: schema.Version10,
			Env:           map[string]schema.EnvVar{envName: {Value: "x", Sensitive: true}},
		}
		return Fingerprint(spec, nil)
	}
	if base("SECRET") == base("OTHER") {
		t.Error("a different env name must change the fingerprint")
	}
}

func TestFingerprint_covers_files_services_and_hooks(t *testing.T) {
	build := func() *schema.GenvFile {
		return &schema.GenvFile{
			SchemaVersion: schema.Version10,
			Packages:      []schema.Package{{ID: "git", Prefer: "brew"}},
			Services:      map[string]schema.Service{"proxy": {Start: []string{"run"}}},
			Shell:         &schema.ShellConfig{Aliases: map[string]schema.ShellAlias{"ll": {Value: "ls -l"}}, Functions: map[string]schema.ShellFunction{"g": {Body: "git status"}}},
			Files: &schema.FilesConfig{
				Links:     []schema.FileLink{{Source: "s", Target: "/t", Mode: "link", Perm: "0644"}},
				Templates: []schema.FileTemplate{{Source: "tpl", Target: "/out", Perm: "0600"}},
				Dirs:      []schema.FileDir{{Target: "/d", Perm: "0755"}},
			},
			Hooks: &schema.HooksConfig{PostApply: []schema.Hook{{Command: "echo hi"}}},
		}
	}
	first := Fingerprint(build(), []string{"m"})
	if first == "" {
		t.Fatal("fingerprint should be set for a populated spec")
	}
	changed := build()
	changed.Files.Links[0].Perm = "0600"
	if Fingerprint(changed, []string{"m"}) == first {
		t.Error("changing a link permission must change the fingerprint")
	}
	changed2 := build()
	changed2.Hooks.PostApply[0].Command = "echo bye"
	if Fingerprint(changed2, []string{"m"}) == first {
		t.Error("changing a hook command must change the fingerprint")
	}
}

func TestFingerprint_order_independent_for_maps(t *testing.T) {
	a := &schema.GenvFile{
		SchemaVersion: schema.Version10,
		Env:           map[string]schema.EnvVar{"A": {Value: "1"}, "B": {Value: "2"}},
		Services:      map[string]schema.Service{"x": {Start: []string{"1"}}, "y": {Start: []string{"2"}}},
	}
	b := &schema.GenvFile{
		SchemaVersion: schema.Version10,
		Env:           map[string]schema.EnvVar{"B": {Value: "2"}, "A": {Value: "1"}},
		Services:      map[string]schema.Service{"y": {Start: []string{"2"}}, "x": {Start: []string{"1"}}},
	}
	if Fingerprint(a, nil) != Fingerprint(b, nil) {
		t.Error("map iteration order must not affect the fingerprint")
	}
}

func TestModuleSafePath_keeps_absolute_and_expanded_paths(t *testing.T) {
	cases := []string{"/usr/local/bin/tool", "~/bin/tool", "$HOME/bin/tool"}
	for _, in := range cases {
		got, err := moduleSafePath("modules/agent", in)
		if err != nil {
			t.Errorf("moduleSafePath(%q) error = %v, want passthrough", in, err)
			continue
		}
		if got != in {
			t.Errorf("moduleSafePath(%q) = %q, want unchanged (apply/export already report these)", in, got)
		}
	}
}

func TestModuleSafePath_empty_input(t *testing.T) {
	if got, err := moduleSafePath("modules", ""); err != nil || got != "" {
		t.Errorf("moduleSafePath(\"\") = %q, %v", got, err)
	}
}

func TestModuleSafePath_root_level_module(t *testing.T) {
	got, err := moduleSafePath(".", "agent/file.md")
	if err != nil {
		t.Fatalf("moduleSafePath() error: %v", err)
	}
	if got != "agent/file.md" {
		t.Errorf("moduleSafePath() = %q, want the path unchanged for a root-level module", got)
	}
}

func TestMergeEntries_tombstone_removes_inherited_key(t *testing.T) {
	editor := schema.EnvVar{Value: "nvim"}
	defaults := map[string]*schema.EnvVar{"EDITOR": &editor, "KEEP": {Value: "yes"}}
	overlay := map[string]*schema.EnvVar{"EDITOR": nil, "OTHER": {Value: "x"}}

	var out map[string]*schema.EnvVar
	mergeEntries(defaults, overlay, func() map[string]*schema.EnvVar {
		return map[string]*schema.EnvVar{}
	}, &out)

	if _, present := out["EDITOR"]; present {
		t.Error("tombstone should delete the inherited entry")
	}
	if out["KEEP"] == nil || out["OTHER"] == nil {
		t.Error("non-tombstoned entries must survive the overlay")
	}
}

func TestMergeEntries_ignores_default_tombstones(t *testing.T) {
	defaults := map[string]*schema.EnvVar{"GONE": nil, "KEPT": {Value: "1"}}
	var out map[string]*schema.EnvVar
	mergeEntries(defaults, nil, func() map[string]*schema.EnvVar {
		return map[string]*schema.EnvVar{}
	}, &out)
	if _, present := out["GONE"]; present {
		t.Error("a nil in defaults must not be materialized")
	}
	if out["KEPT"] == nil {
		t.Error("real defaults entries must be kept")
	}
}

func TestMergeEntries_empty_result_leaves_dst_nil(t *testing.T) {
	var out map[string]*schema.EnvVar
	mergeEntries(nil, map[string]*schema.EnvVar{"X": nil}, func() map[string]*schema.EnvVar {
		return map[string]*schema.EnvVar{}
	}, &out)
	if out != nil {
		t.Error("an empty union should leave the destination nil, not an empty map")
	}
}

func TestMergeShellBundles_tombstones_and_maps(t *testing.T) {
	alias := schema.ShellAlias{Value: "a"}
	fn := schema.ShellFunction{Body: "b"}
	defaults := &schema.TargetShellConfig{
		Aliases:   map[string]*schema.ShellAlias{"keep": &alias, "drop": {Value: "d"}},
		Functions: map[string]*schema.ShellFunction{"fn": &fn},
	}
	overlay := &schema.TargetShellConfig{
		Aliases:   map[string]*schema.ShellAlias{"drop": nil, "add": {Value: "n"}},
		Functions: map[string]*schema.ShellFunction{"fn": nil},
	}

	out := mergeShellBundles(defaults, overlay)
	if out.Aliases["keep"] == nil {
		t.Error("untouched alias must survive")
	}
	if _, present := out.Aliases["drop"]; present {
		t.Error("alias tombstone must delete the inherited alias")
	}
	if out.Aliases["add"] == nil {
		t.Error("overlay alias must be added")
	}
	if _, present := out.Functions["fn"]; present {
		t.Error("function tombstone must delete the inherited function")
	}
}

func TestMergeShellBundles_nil_inputs_return_nil(t *testing.T) {
	if mergeShellBundles(nil, nil) != nil {
		t.Error("no shell declarations should stay nil so omitempty keeps working")
	}
}

func TestHooksEmpty_and_phase_helpers_cover_every_phase(t *testing.T) {
	for _, phase := range hookPhaseNames {
		h := &schema.HooksConfig{}
		if !hooksEmpty(h) {
			t.Fatalf("empty hooks config should report empty")
		}
		setPhase(h, phase, []schema.Hook{{Command: "x"}})
		if hooksEmpty(h) {
			t.Errorf("phase %q was not recorded", phase)
		}
		if len(phaseSlice(h, phase)) != 1 {
			t.Errorf("phase %q should hold one hook", phase)
		}
		appendPhase(h, phase, []schema.Hook{{Command: "y"}})
		if len(phaseSlice(h, phase)) != 2 {
			t.Errorf("phase %q should append", phase)
		}
	}
	if len(phaseSlice(nil, "postApply")) != 0 {
		t.Error("nil hooks config should yield no phase entries")
	}
}

func TestPhaseSlice_unknown_phase_is_empty(t *testing.T) {
	if len(phaseSlice(&schema.HooksConfig{}, "nope")) != 0 {
		t.Error("unknown phase should be empty")
	}
}

func TestBundleSelections_dedups_and_tolerates_nil(t *testing.T) {
	if got := bundleSelections(nil, []string{"a"}); len(got) != 1 || got[0] != "a" {
		t.Errorf("nil bundle should pass selections through, got %v", got)
	}
	bundle := &schema.TargetBundle{UseModules: []string{"a", "b", "a"}}
	got := bundleSelections(bundle, []string{"b"})
	if strings.Join(got, ",") != "b,a" {
		t.Errorf("bundleSelections() = %v, want first-occurrence order without duplicates", got)
	}
}

func TestParseModuleErrorPaths(t *testing.T) {
	_, _, err := schema.ParseAndValidateModule([]byte(`{"schemaVersion":`))
	if err == nil {
		t.Fatal("expected a parse error for malformed JSON")
	}
	if !strings.Contains(err.Error(), "JSON syntax error") {
		t.Errorf("syntax error should be labeled, got %v", err)
	}

	_, errs, err := schema.ParseAndValidateModule([]byte(`{"schemaVersion":10}`))
	if err != nil {
		t.Fatalf("type mismatch should be a validation error, not a parse error: %v", err)
	}
	if len(errs) == 0 {
		t.Fatal("expected a type error for a numeric schemaVersion")
	}
}

func TestLoadDocument_invalid_json_reports_path(t *testing.T) {
	root := t.TempDir()
	writeModule(t, root, "modules/broken.json", `{"schemaVersion":`)
	_, err := LoadDocument(root, "modules/broken.json")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "modules/broken.json") {
		t.Errorf("error should name the module path: %v", err)
	}
}

func TestResolve_ignores_broken_module_nobody_selects(t *testing.T) {
	root := t.TempDir()
	writeModule(t, root, "modules/ok.json", `{"schemaVersion":"10","defaults":{}}`)
	f := &schema.GenvFile{
		SchemaVersion: schema.Version10,
		Modules:       map[string]string{"good": "modules/ok.json", "bad": "modules/missing.json"},
		Targets:       map[string]*schema.TargetBundle{"macos": {UseModules: []string{"good"}}},
	}
	// Composition reads only the selection closure, so an unrelated broken
	// registration must not break `genv status`. LoadAllRegistered (used by
	// `genv validate`) is what still reports it.
	c, err := Resolve(root, root, f, "macos", nil)
	if err != nil {
		t.Fatalf("Resolve() error: %v", err)
	}
	if len(c.SelectedModules) != 1 || c.SelectedModules[0] != "good" {
		t.Errorf("SelectedModules = %v, want only the selected module", c.SelectedModules)
	}
	if _, err := LoadAllRegistered(root, f.Modules); err == nil {
		t.Error("LoadAllRegistered should still report the broken registration")
	}
}

func TestResolve_loads_dependency_of_selected_module(t *testing.T) {
	root := t.TempDir()
	writeModule(t, root, "modules/app.json", `{"schemaVersion":"10","requiresModules":["dep"],"defaults":{"packages":[{"id":"app"}]}}`)
	writeModule(t, root, "modules/dep.json", `{"schemaVersion":"10","defaults":{"packages":[{"id":"dep"}]}}`)
	f := &schema.GenvFile{
		SchemaVersion: schema.Version10,
		Modules:       map[string]string{"app": "modules/app.json", "dep": "modules/dep.json"},
		Targets:       map[string]*schema.TargetBundle{"macos": {UseModules: []string{"app"}}},
	}
	c := mustResolve(t, root, f, "macos")
	if got := strings.Join(packageIDs(t, c), ","); got != "dep,app" {
		t.Errorf("packages = %q, want the dependency composed too", got)
	}
}

func TestResolve_selection_budget_rejects_oversize_total(t *testing.T) {
	root := t.TempDir()
	writeModule(t, root, "modules/a.json", `{"schemaVersion":"10","defaults":{"packages":[{"id":"a"}]}}`)
	old := maxTotalModuleBytes
	maxTotalModuleBytes = 4
	defer func() { maxTotalModuleBytes = old }()

	f := &schema.GenvFile{
		SchemaVersion: schema.Version10,
		Modules:       map[string]string{"a": "modules/a.json"},
		Targets:       map[string]*schema.TargetBundle{"macos": {UseModules: []string{"a"}}},
	}
	_, err := Resolve(root, root, f, "macos", nil)
	if err == nil {
		t.Fatal("expected aggregate budget error")
	}
	if !errors.Is(err, ErrLimit) {
		t.Errorf("error = %v, want ErrLimit", err)
	}
}

func TestResolve_carries_root_owned_blocks(t *testing.T) {
	root := t.TempDir()
	writeModule(t, root, "modules/a.json", `{"schemaVersion":"10","defaults":{"packages":[{"id":"a"}]}}`)
	spec(t, root, `{
	  "schemaVersion": "10",
	  "repo": { "url": "https://example.com/dotfiles", "ref": "main" },
	  "updates": { "enabled": true, "interval": "24h", "skipManagers": ["mas"] },
	  "adapters": { "gh-extension": { "list": "gh extension list", "install": "gh extension install {{id}}", "remove": "gh extension remove {{id}}" } },
	  "modules": { "a": "modules/a.json" },
	  "targets": { "macos": { "useModules": ["a"] } }
	}`)
	f, _, _ := schema.ParseAndValidate([]byte(mustRead(t, root+"/genv.json")))

	c := mustResolve(t, root, f, "macos")
	// Regression guard: dropping these is the class of bug that made v8
	// status/upgrade see an empty environment.
	if c.Effective.Repo == nil || c.Effective.Repo.URL != "https://example.com/dotfiles" || c.Effective.Repo.Ref != "main" {
		t.Errorf("repo not carried through: %+v", c.Effective.Repo)
	}
	if c.Effective.Updates == nil || !c.Effective.Updates.Enabled || c.Effective.Updates.Interval != "24h" {
		t.Errorf("updates not carried through: %+v", c.Effective.Updates)
	}
	if len(c.Effective.Updates.SkipManagers) != 1 || c.Effective.Updates.SkipManagers[0] != "mas" {
		t.Errorf("updates filters not carried through: %+v", c.Effective.Updates)
	}
	if c.Effective.Adapters["gh-extension"].List != "gh extension list" {
		t.Errorf("adapters not carried through: %+v", c.Effective.Adapters)
	}
	// And they must be copies, not shared state with the parsed spec.
	c.Effective.Updates.SkipManagers[0] = "mutated"
	if f.Updates.SkipManagers[0] != "mas" {
		t.Error("effective spec must not share slice state with the parsed spec")
	}
	c.Effective.Repo.URL = "mutated"
	if f.Repo.URL != "https://example.com/dotfiles" {
		t.Error("effective spec must not share repo state with the parsed spec")
	}
}

func TestResolve_defaults_useModules_selects_for_every_target(t *testing.T) {
	root := t.TempDir()
	writeModule(t, root, "modules/shared.json", `{"schemaVersion":"10","defaults":{"packages":[{"id":"shared"}]}}`)
	writeModule(t, root, "modules/mac.json", `{"schemaVersion":"10","defaults":{"packages":[{"id":"mac-only"}]}}`)
	spec(t, root, `{
	  "schemaVersion": "10",
	  "modules": { "shared": "modules/shared.json", "mac": "modules/mac.json" },
	  "defaults": { "useModules": ["shared"] },
	  "targets": {
	    "macos": { "useModules": ["mac"] },
	    "ubuntu": { "packages": [{ "id": "apt-pkg" }] }
	  }
	}`)
	f, _, _ := schema.ParseAndValidate([]byte(mustRead(t, root+"/genv.json")))

	mac := mustResolve(t, root, f, "macos")
	if got := strings.Join(packageIDs(t, mac), ","); got != "shared,mac-only" {
		t.Errorf("macos packages = %q, want defaults selection then target selection", got)
	}

	// A defaults-level selection applies to every target, including one that
	// declares no useModules of its own.
	// The root is the first contributor, so its own packages lead.
	ubuntu := mustResolve(t, root, f, "ubuntu")
	if got := strings.Join(packageIDs(t, ubuntu), ","); got != "apt-pkg,shared" {
		t.Errorf("ubuntu packages = %q, want the defaults-level selection applied", got)
	}
}

func TestResolve_link_and_dir_on_same_destination_conflict(t *testing.T) {
	root := t.TempDir()
	writeModule(t, root, "modules/a.json", `{"schemaVersion":"10","targets":{"macos":{"files":{"links":[{"source":"s","target":"/Users/me/x"}]}}}}`)
	writeModule(t, root, "modules/b.json", `{"schemaVersion":"10","targets":{"macos":{"files":{"dirs":[{"target":"/Users/me/x"}]}}}}`)
	spec(t, root, `{
	  "schemaVersion": "10",
	  "modules": { "a": "modules/a.json", "b": "modules/b.json" },
	  "targets": { "macos": { "useModules": ["a", "b"] } }
	}`)
	f, _, _ := schema.ParseAndValidate([]byte(mustRead(t, root+"/genv.json")))

	_, err := Resolve(root, root, f, "macos", nil)
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("error = %v, want ErrConflict for a link and a dir claiming one path", err)
	}
}

func TestResolve_identical_dirs_coalesce(t *testing.T) {
	root := t.TempDir()
	body := `{"schemaVersion":"10","targets":{"macos":{"files":{"dirs":[{"target":"/Users/me/shared","perm":"0755"}]}}}}`
	writeModule(t, root, "modules/a.json", body)
	writeModule(t, root, "modules/b.json", body)
	spec(t, root, `{
	  "schemaVersion": "10",
	  "modules": { "a": "modules/a.json", "b": "modules/b.json" },
	  "targets": { "macos": { "useModules": ["a", "b"] } }
	}`)
	f, _, _ := schema.ParseAndValidate([]byte(mustRead(t, root+"/genv.json")))

	c := mustResolve(t, root, f, "macos")
	if len(c.Effective.Files.Dirs) != 1 {
		t.Fatalf("identical dirs should coalesce, got %d", len(c.Effective.Files.Dirs))
	}
	if len(c.Provenance.Owners(dirIdentity("/Users/me/shared"))) != 2 {
		t.Error("both owners should be recorded")
	}
}

func TestResolve_effective_does_not_alias_nested_declarations(t *testing.T) {
	root := t.TempDir()
	writeModule(t, root, "modules/a.json", `{
	  "schemaVersion": "10",
	  "targets": { "macos": {
	    "services": {
	      "proxy": { "start": ["run"] },
	      "mlx": { "launchd": { "plist": "p.plist" } }
	    },
	    "packages": [{ "id": "jq", "prefer": "external", "external": {
	      "detect": { "command": ["jq", "--version"], "versionRegex": "jq-([0-9.]+)" },
	      "source": { "type": "githubRelease", "repository": "jqlang/jq" },
	      "platforms": [ { "os": ["darwin"], "arch": ["arm64"], "assetRegex": "jq-.*", "install": { "type": "direct", "destination": "/usr/local/bin/jq" } } ],
	      "verify": [ { "type": "githubDigest", "assetRegex": "jq-.*" } ]
	    } } ]
	  } }
	}`)
	spec(t, root, `{
	  "schemaVersion": "10",
	  "modules": { "a": "modules/a.json" },
	  "targets": { "macos": { "useModules": ["a"] } }
	}`)
	f, _, _ := schema.ParseAndValidate([]byte(mustRead(t, root+"/genv.json")))
	c := mustResolve(t, root, f, "macos")

	// Mutating the effective spec must not reach back into the module document.
	c.Effective.Services["proxy"].Start[0] = "mutated"
	c.Effective.Packages[0].External.Source.Repository = "mutated/repo"

	raw, errs, parseErr := schema.ParseAndValidateModule([]byte(mustRead(t, root+"/modules/a.json")))
	if parseErr != nil || len(errs) != 0 {
		t.Fatalf("re-reading the module failed: %v %v", parseErr, errs)
	}
	if raw.Targets["macos"].Services["proxy"].Start[0] != "run" {
		t.Error("effective spec aliased the module document's service argv")
	}
	if raw.Targets["macos"].Services["mlx"].Launchd.Plist != "p.plist" {
		t.Error("effective spec aliased the module document's launchd spec")
	}
	if raw.Targets["macos"].Packages[0].External.Source.Repository != "jqlang/jq" {
		t.Error("effective spec aliased the module document's external recipe")
	}
}

func TestResolve_module_without_selection_is_not_composed(t *testing.T) {
	root := t.TempDir()
	writeModule(t, root, "modules/unused.json", `{"schemaVersion":"10","defaults":{"packages":[{"id":"ghost"}]}}`)
	f := &schema.GenvFile{
		SchemaVersion: schema.Version10,
		Modules:       map[string]string{"unused": "modules/unused.json"},
		Targets:       map[string]*schema.TargetBundle{"macos": {}},
	}
	c := mustResolve(t, root, f, "macos")
	if got := strings.Join(packageIDs(t, c), ","); got != "" {
		t.Errorf("packages = %q, want an unselected module to contribute nothing", got)
	}
	if len(c.SelectedModules) != 0 {
		t.Errorf("SelectedModules = %v, want empty", c.SelectedModules)
	}
}

func TestResolve_targets_nil_is_reported(t *testing.T) {
	root := t.TempDir()
	f := &schema.GenvFile{
		SchemaVersion: schema.Version10,
		Modules:       map[string]string{"m": "modules/m.json"},
		Targets:       map[string]*schema.TargetBundle{"macos": nil},
	}
	_, err := Resolve(root, root, f, "macos", nil)
	if err == nil {
		t.Fatal("a nil target must be reported, matching MergeTarget behavior")
	}
}

func TestResolve_nil_spec_is_reported(t *testing.T) {
	if _, err := Resolve(t.TempDir(), "", nil, "macos", nil); err == nil {
		t.Fatal("expected an error for a nil spec")
	}
}

func TestResolve_module_doc_nil_is_tolerated(t *testing.T) {
	if got := moduleBundle(nil, "macos"); got == nil {
		t.Error("moduleBundle(nil) should return an empty bundle, not nil")
	}
}

func TestResolve_declared_destinations_reported_in_provenance(t *testing.T) {
	root := t.TempDir()
	writeModule(t, root, "modules/a.json", `{
	  "schemaVersion": "10",
	  "targets": { "macos": {
	    "files": {
	      "links": [ { "source": "s", "target": "/Users/me/a" } ],
	      "templates": [ { "source": "t", "target": "/Users/me/b" } ],
	      "dirs": [ { "target": "/Users/me/d" } ]
	    },
	    "env": { "EDITOR": { "value": "nvim" } },
	    "shell": { "aliases": { "ll": { "value": "ls -l" } } }
	  } }
	}`)
	spec(t, root, `{
	  "schemaVersion": "10",
	  "modules": { "a": "modules/a.json" },
	  "targets": { "macos": { "useModules": ["a"] } }
	}`)
	f, _, _ := schema.ParseAndValidate([]byte(mustRead(t, root+"/genv.json")))

	c := mustResolve(t, root, f, "macos")
	want := []string{
		"alias:ll",
		"dir:/Users/me/d",
		"env:EDITOR",
		"file:/Users/me/a",
		"file:/Users/me/b",
	}
	got := []string{}
	for _, id := range c.Provenance.Identities() {
		got = append(got, id.String())
	}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("Identities() = %v, want %v", got, want)
	}
}
