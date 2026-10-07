package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ks1686/genv/internal/compose"
	"github.com/ks1686/genv/internal/genvfile"
	"github.com/ks1686/genv/internal/schema"
)

// composeFixture writes a root spec plus module documents in dir and returns dir.
func composeFixture(t *testing.T, rootSpec string, modules map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for rel, body := range modules {
		full := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "genv.json"), []byte(rootSpec), 0o600); err != nil {
		t.Fatalf("write spec: %v", err)
	}
	return dir
}

// composeWriteFixture writes a root spec plus module documents into dir.
func composeWriteFixture(t *testing.T, dir, rootSpec string, modules map[string]string) {
	t.Helper()
	for rel, body := range modules {
		full := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "genv.json"), []byte(rootSpec), 0o600); err != nil {
		t.Fatalf("write spec: %v", err)
	}
}

const twoModuleSpec = `{
  "schemaVersion": "10",
  "modules": { "base": "modules/base.json", "tools": "modules/tools.json" },
  "targets": { "macos": { "useModules": ["tools"], "packages": [{ "id": "root-pkg" }] } }
}`

func twoModuleFixture(t *testing.T) string {
	return composeFixture(t, twoModuleSpec, map[string]string{
		"modules/base.json":  `{"schemaVersion":"10","defaults":{"packages":[{"id":"jq"}],"env":{"EDITOR":{"value":"nvim"}}}}`,
		"modules/tools.json": `{"schemaVersion":"10","requiresModules":["base"],"defaults":{"packages":[{"id":"ripgrep"}]}}`,
	})
}

func TestMaterializeSpecForCommand_v10_composes_modules(t *testing.T) {
	dir := twoModuleFixture(t)
	specPath := filepath.Join(dir, "genv.json")
	f, err := genvfile.Read(specPath)
	if err != nil {
		t.Fatalf("read spec: %v", err)
	}

	effective, targetID, code := materializeSpecForCommand("status", specPath, f, "", "macos")
	if code != exitOK {
		t.Fatalf("materializeSpecForCommand code = %d, want 0", code)
	}
	if targetID != "macos" {
		t.Errorf("target = %q, want macos", targetID)
	}
	ids := map[string]bool{}
	for _, p := range effective.Packages {
		ids[p.ID] = true
	}
	for _, want := range []string{"root-pkg", "jq", "ripgrep"} {
		if !ids[want] {
			t.Errorf("effective spec missing %q (got %v)", want, ids)
		}
	}
	if _, ok := effective.Env["EDITOR"]; !ok {
		t.Error("module defaults.env should reach the effective spec")
	}
}

func TestMaterializeSpecForCommand_v8_unchanged(t *testing.T) {
	dir := composeFixture(t, `{
	  "schemaVersion": "8",
	  "defaults": { "packages": [{ "id": "git" }], "env": { "EDITOR": { "value": "vim" } } },
	  "targets": { "macos": { "packages": [{ "id": "jq" }] } }
	}`, nil)
	specPath := filepath.Join(dir, "genv.json")
	f, err := genvfile.Read(specPath)
	if err != nil {
		t.Fatalf("read spec: %v", err)
	}
	effective, targetID, code := materializeSpecForCommand("status", specPath, f, "", "macos")
	if code != exitOK {
		t.Fatalf("code = %d", code)
	}
	if targetID != "macos" {
		t.Errorf("target = %q", targetID)
	}
	// v8 array semantics: the target array replaces the defaults array.
	if len(effective.Packages) != 1 || effective.Packages[0].ID != "jq" {
		t.Errorf("packages = %+v, want the plain v8 merge", effective.Packages)
	}
	if effective.Env["EDITOR"].Value != "vim" {
		t.Errorf("env = %+v, want the defaults entry", effective.Env)
	}
}

func TestMaterializeSpecForCommand_reports_module_failure(t *testing.T) {
	dir := composeFixture(t, `{
	  "schemaVersion": "10",
	  "modules": { "a": "modules/a.json", "b": "modules/b.json" },
	  "targets": { "macos": { "useModules": ["a", "b"] } }
	}`, map[string]string{
		"modules/a.json": `{"schemaVersion":"10","targets":{"macos":{"services":{"s":{"start":["a"]}}}}}`,
		"modules/b.json": `{"schemaVersion":"10","targets":{"macos":{"services":{"s":{"start":["b"]}}}}}`,
	})
	specPath := filepath.Join(dir, "genv.json")
	f, err := genvfile.Read(specPath)
	if err != nil {
		t.Fatalf("read spec: %v", err)
	}
	_, _, code := materializeSpecForCommand("status", specPath, f, "", "macos")
	if code == exitOK {
		t.Fatal("expected a non-zero exit for a composition conflict")
	}
}

func TestCompositionFor_reports_provenance(t *testing.T) {
	dir := twoModuleFixture(t)
	specPath := filepath.Join(dir, "genv.json")
	f, err := genvfile.Read(specPath)
	if err != nil {
		t.Fatalf("read spec: %v", err)
	}
	c, err := compositionFor(specPath, f, "", "macos", "")
	if err != nil {
		t.Fatalf("compositionFor: %v", err)
	}
	owners := c.Provenance.Owners(compose.Identity{Kind: compose.KindPackage, Key: "jq"})
	if len(owners) != 1 || owners[0].Module != "base" {
		t.Errorf("jq owners = %+v, want the base module", owners)
	}
	if c.Provenance.IsModuleOwned(compose.Identity{Kind: compose.KindService, Key: "nope"}) {
		t.Error("an identity no module declares must not be module-owned")
	}
	if len(c.SelectedModules) != 2 {
		t.Errorf("SelectedModules = %v, want base and tools", c.SelectedModules)
	}
}

func TestValidateComposition_reports_unused_broken_module(t *testing.T) {
	dir := composeFixture(t, `{
	  "schemaVersion": "10",
	  "modules": { "good": "modules/good.json", "bad": "modules/missing.json" },
	  "targets": { "macos": { "useModules": ["good"] } }
	}`, map[string]string{
		"modules/good.json": `{"schemaVersion":"10","defaults":{"packages":[{"id":"jq"}]}}`,
	})
	specPath := filepath.Join(dir, "genv.json")
	f, err := genvfile.Read(specPath)
	if err != nil {
		t.Fatalf("read spec: %v", err)
	}
	// Reconciliation ignores an unselected broken module...
	if _, cErr := compositionFor(specPath, f, "", "macos", ""); cErr != nil {
		t.Errorf("compositionFor should ignore an unselected broken module: %v", cErr)
	}
	// ...but validate must report it.
	issues := validateComposition(specPath, f, "", "")
	if len(issues) == 0 {
		t.Fatal("validateComposition should report the broken registration")
	}
	if !strings.Contains(issues[0].Error(), "bad") {
		t.Errorf("issue should name the module: %v", issues[0])
	}
}

func TestValidateComposition_noop_without_modules(t *testing.T) {
	dir := composeFixture(t, `{"schemaVersion":"8","targets":{"macos":{}}}`, nil)
	specPath := filepath.Join(dir, "genv.json")
	f, err := genvfile.Read(specPath)
	if err != nil {
		t.Fatalf("read spec: %v", err)
	}
	if issues := validateComposition(specPath, f, "", ""); len(issues) != 0 {
		t.Errorf("validateComposition on a module-less spec = %v, want none", issues)
	}
}

func TestValidateComposition_reports_cycle(t *testing.T) {
	dir := composeFixture(t, `{
	  "schemaVersion": "10",
	  "modules": { "a": "modules/a.json", "b": "modules/b.json" },
	  "targets": { "macos": { "useModules": ["a"] } }
	}`, map[string]string{
		"modules/a.json": `{"schemaVersion":"10","requiresModules":["b"],"defaults":{}}`,
		"modules/b.json": `{"schemaVersion":"10","requiresModules":["a"],"defaults":{}}`,
	})
	specPath := filepath.Join(dir, "genv.json")
	f, err := genvfile.Read(specPath)
	if err != nil {
		t.Fatalf("read spec: %v", err)
	}
	issues := validateComposition(specPath, f, "", "")
	if len(issues) == 0 {
		t.Fatal("expected a cycle report")
	}
	if !strings.Contains(issues[0].Error(), "cycle") {
		t.Errorf("issue should mention the cycle: %v", issues[0])
	}
}

func TestStampCompositionLock_records_selection(t *testing.T) {
	dir := twoModuleFixture(t)
	specPath := filepath.Join(dir, "genv.json")
	f, _ := genvfile.Read(specPath)
	c, err := compositionFor(specPath, f, "", "macos", "")
	if err != nil {
		t.Fatalf("compositionFor: %v", err)
	}
	lf := &genvfile.LockFile{SchemaVersion: schema.Version8}
	stampCompositionLock(lf, c)
	if strings.Join(lf.Modules, ",") != "base,tools" {
		t.Errorf("lock modules = %v, want the composed order", lf.Modules)
	}
	if lf.Fingerprint == "" {
		t.Error("lock fingerprint should be recorded")
	}

	// Drift notice: a different selection or structure is reported, not refused.
	other := &compose.Composition{SelectedModules: []string{"base"}, Fingerprint: c.Fingerprint}
	notice := compositionDriftNotice(lf, other)
	if notice == "" {
		t.Error("expected a drift notice after the selection changed")
	}
	if !strings.Contains(notice, "module selection changed") {
		t.Errorf("notice = %q, want it to mention the selection change", notice)
	}
}

func TestCompositionDriftNotice_silent_when_unchanged(t *testing.T) {
	dir := twoModuleFixture(t)
	specPath := filepath.Join(dir, "genv.json")
	f, _ := genvfile.Read(specPath)
	c, _ := compositionFor(specPath, f, "", "macos", "")
	lf := &genvfile.LockFile{}
	stampCompositionLock(lf, c)
	if got := compositionDriftNotice(lf, c); got != "" {
		t.Errorf("drift notice = %q, want empty", got)
	}
	if got := compositionDriftNotice(nil, c); got != "" {
		t.Errorf("drift notice with nil lock = %q, want empty", got)
	}
	if got := compositionDriftNotice(lf, nil); got != "" {
		t.Errorf("drift notice with nil composition = %q, want empty", got)
	}
	// An old lock with no composition metadata never produces a notice.
	if got := compositionDriftNotice(&genvfile.LockFile{}, c); got != "" {
		t.Errorf("drift notice for a legacy lock = %q, want empty", got)
	}
}

func TestComposeSourceRoot(t *testing.T) {
	if got := composeSourceRoot("/a/b/genv.json", ""); got != "/a/b" {
		t.Errorf("composeSourceRoot = %q, want the spec directory", got)
	}
	if got := composeSourceRoot("/a/b/genv.json", "/elsewhere"); got != "/elsewhere" {
		t.Errorf("composeSourceRoot = %q, want the explicit override", got)
	}
	if got := composeSourceRoot("", ""); got != "" {
		t.Errorf("composeSourceRoot with no inputs = %q, want empty", got)
	}
}

func TestStatusCmd_v10_reports_module_packages(t *testing.T) {
	dir := twoModuleFixture(t)
	specPath := filepath.Join(dir, "genv.json")
	lockPath := filepath.Join(dir, "genv.lock.json")

	out := captureStdout(t, func() {
		code := statusCmd([]string{"--file", specPath, "--lock-file", lockPath, "--target", "macos", "--offline", "--json"})
		if code == exitOK {
			t.Fatalf("status should exit %d when packages are not applied yet", exitLogic)
		}
	})
	if !strings.Contains(out, "jq") || !strings.Contains(out, "ripgrep") {
		t.Errorf("status output should list module packages, got: %s", out)
	}
}

func TestApplyCmd_v10_dry_run_plans_module_packages(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "xdg"))
	specPath := filepath.Join(dir, "genv.json")
	lockPath := filepath.Join(dir, "genv.lock.json")
	registerLifecycleHookAdapter(t, lifecycleHookAdapter{installMarker: filepath.Join(dir, "install.log")})
	composeWriteFixture(t, dir, `{
	  "schemaVersion": "10",
	  "modules": { "base": "modules/base.json", "tools": "modules/tools.json" },
	  "targets": { "arch": { "useModules": ["tools"], "packages": [{ "id": "root-pkg", "prefer": "test-hook-manager" }] } }
	}`, map[string]string{
		"modules/base.json":  `{"schemaVersion":"10","defaults":{"packages":[{"id":"jq","prefer":"test-hook-manager"}]}}`,
		"modules/tools.json": `{"schemaVersion":"10","requiresModules":["base"],"defaults":{"packages":[{"id":"ripgrep","prefer":"test-hook-manager"}]}}`,
	})

	out := captureStdout(t, func() {
		code := run([]string{"apply", "--file", specPath, "--lock-file", lockPath, "--target", "arch", "--dry-run", "--json"})
		if code != exitOK {
			t.Fatalf("apply --dry-run --json exit = %d, want 0", code)
		}
	})

	var env struct {
		Data struct {
			Install []struct {
				ID string `json:"id"`
			} `json:"toInstall"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("parse envelope: %v\n%s", err, out)
	}
	got := map[string]bool{}
	for _, p := range env.Data.Install {
		got[p.ID] = true
	}
	for _, want := range []string{"jq", "ripgrep", "root-pkg"} {
		if !got[want] {
			t.Errorf("apply plan missing %q (got %v)", want, got)
		}
	}
}

func TestApplyCmd_v10_records_module_selection_in_lock(t *testing.T) {
	dir := twoModuleFixture(t)
	specPath := filepath.Join(dir, "genv.json")
	lockPath := filepath.Join(dir, "genv.lock.json")

	captureStdout(t, func() {
		// --skip-packages keeps this test hermetic: no package manager runs.
		if code := applyCmd([]string{"--file", specPath, "--lock-file", lockPath, "--target", "macos", "--skip-packages", "--yes"}); code != exitOK {
			t.Fatalf("apply exit = %d, want 0", code)
		}
	})

	lf, err := genvfile.ReadLock(lockPath)
	if err != nil {
		t.Fatalf("read lock: %v", err)
	}
	if strings.Join(lf.Modules, ",") != "base,tools" {
		t.Errorf("lock modules = %v, want base,tools", lf.Modules)
	}
	if lf.Fingerprint == "" {
		t.Error("lock should record the composition fingerprint")
	}
}

func TestApplyCmd_v10_missing_module_exits_validation(t *testing.T) {
	dir := composeFixture(t, `{
	  "schemaVersion": "10",
	  "modules": { "ghost": "modules/ghost.json" },
	  "targets": { "macos": { "useModules": ["ghost"] } }
	}`, nil)
	specPath := filepath.Join(dir, "genv.json")
	lockPath := filepath.Join(dir, "genv.lock.json")

	captureStdout(t, func() {
		if code := applyCmd([]string{"--file", specPath, "--lock-file", lockPath, "--target", "macos", "--dry-run"}); code != exitValidation {
			t.Fatalf("exit = %d, want %d for a missing module", code, exitValidation)
		}
	})
}

func TestApplyCmd_v10_conflict_exits_before_mutating(t *testing.T) {
	dir := composeFixture(t, `{
	  "schemaVersion": "10",
	  "modules": { "a": "modules/a.json", "b": "modules/b.json" },
	  "targets": { "macos": { "useModules": ["a", "b"] } }
	}`, map[string]string{
		"modules/a.json": `{"schemaVersion":"10","targets":{"macos":{"env":{"EDITOR":{"value":"nvim"}}}}}`,
		"modules/b.json": `{"schemaVersion":"10","targets":{"macos":{"env":{"EDITOR":{"value":"vim"}}}}}`,
	})
	specPath := filepath.Join(dir, "genv.json")
	lockPath := filepath.Join(dir, "genv.lock.json")

	captureStdout(t, func() {
		if code := applyCmd([]string{"--file", specPath, "--lock-file", lockPath, "--target", "macos", "--yes"}); code != exitValidation {
			t.Fatalf("exit = %d, want %d for a conflict", code, exitValidation)
		}
	})
	if _, err := os.Stat(lockPath); err == nil {
		t.Error("a conflicting composition must not write a lock")
	}
	if _, err := os.Stat(filepath.Join(dir, "env.sh")); err == nil {
		t.Error("a conflicting composition must not write env fragments")
	}
}

// --- ownership guards (Task 5) -------------------------------------------
//
// A module's declarations are read-only from the CLI. Each guard must refuse
// before any subprocess runs and before the spec changes.

func ownedFixture(t *testing.T) (dir, specPath, lockPath string) {
	t.Helper()
	dir = t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "xdg"))
	specPath = filepath.Join(dir, "genv.json")
	lockPath = filepath.Join(dir, "genv.lock.json")
	composeWriteFixture(t, dir, `{
	  "schemaVersion": "10",
	  "modules": { "base": "modules/base.json" },
	  "targets": { "arch": { "useModules": ["base"], "packages": [{ "id": "root-pkg" }] } }
	}`, map[string]string{
		"modules/base.json": `{"schemaVersion":"10","defaults":{"packages":[{"id":"jq"}],"env":{"EDITOR":{"value":"nvim"}}}}`,
	})
	return dir, specPath, lockPath
}

func TestAddCmd_refuses_module_owned_package(t *testing.T) {
	_, specPath, lockPath := ownedFixture(t)
	marker := filepath.Join(t.TempDir(), "install.log")
	registerLifecycleHookAdapter(t, lifecycleHookAdapter{installMarker: marker})

	captureStdout(t, func() {
		code := run([]string{"add", "jq", "--file", specPath, "--lock-file", lockPath, "--target", "arch", "--prefer", "test-hook-manager", "--no-search", "--no-hooks"})
		if code != exitLogic {
			t.Fatalf("add of a module-owned package = %d, want %d", code, exitLogic)
		}
	})
	if _, err := os.Stat(marker); err == nil {
		t.Error("refused add must not run the install command")
	}
	spec, err := os.ReadFile(specPath)
	if err != nil {
		t.Fatalf("read spec: %v", err)
	}
	// The root spec must still contain only its own declaration: jq lives in the
	// module, and a refused add must not add a root-level copy.
	if !strings.Contains(string(spec), `"root-pkg"`) || strings.Contains(string(spec), "jq") {
		t.Errorf("refused add must leave the root spec untouched:\n%s", spec)
	}
}

func TestAddCmd_allows_unowned_package(t *testing.T) {
	_, specPath, lockPath := ownedFixture(t)
	marker := filepath.Join(t.TempDir(), "install.log")
	registerLifecycleHookAdapter(t, lifecycleHookAdapter{installMarker: marker})

	captureStdout(t, func() {
		code := run([]string{"add", "wget", "--file", specPath, "--lock-file", lockPath, "--target", "arch", "--prefer", "test-hook-manager", "--no-search", "--no-hooks"})
		if code != exitOK {
			t.Fatalf("add of an unowned package = %d, want %d", code, exitOK)
		}
	})
	spec, err := os.ReadFile(specPath)
	if err != nil {
		t.Fatalf("read spec: %v", err)
	}
	if !strings.Contains(string(spec), "wget") {
		t.Errorf("add should persist an unowned package:\n%s", spec)
	}
}

func TestRemoveCmd_refuses_module_owned_package(t *testing.T) {
	_, specPath, lockPath := ownedFixture(t)
	captureStdout(t, func() {
		if code := run([]string{"remove", "jq", "--file", specPath, "--lock-file", lockPath, "--target", "arch"}); code != exitLogic {
			t.Fatalf("remove of a module-owned package = %d, want %d", code, exitLogic)
		}
	})
}

func TestDisownCmd_refuses_module_owned_package(t *testing.T) {
	_, specPath, lockPath := ownedFixture(t)
	captureStdout(t, func() {
		if code := run([]string{"disown", "jq", "--file", specPath, "--lock-file", lockPath, "--target", "arch"}); code != exitLogic {
			t.Fatalf("disown of a module-owned package = %d, want %d", code, exitLogic)
		}
	})
}

func TestEnvSetCmd_refuses_module_owned_var(t *testing.T) {
	_, specPath, _ := ownedFixture(t)
	captureStdout(t, func() {
		if code := run([]string{"env", "set", "--file", specPath, "--target", "arch", "EDITOR", "vim"}); code != exitLogic {
			t.Fatalf("env set of a module-owned var = %d, want %d", code, exitLogic)
		}
	})
}

func TestEnvSetCmd_allows_unowned_var(t *testing.T) {
	_, specPath, _ := ownedFixture(t)
	captureStdout(t, func() {
		if code := run([]string{"env", "set", "--file", specPath, "--target", "arch", "PAGER", "less"}); code != exitOK {
			t.Fatalf("env set of an unowned var = %d, want %d", code, exitOK)
		}
	})
}

func TestServiceCmd_refuses_module_owned_service(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "xdg"))
	specPath := filepath.Join(dir, "genv.json")
	composeWriteFixture(t, dir, `{
	  "schemaVersion": "10",
	  "modules": { "svc": "modules/svc.json" },
	  "targets": { "arch": { "useModules": ["svc"] } }
	}`, map[string]string{
		"modules/svc.json": `{"schemaVersion":"10","targets":{"arch":{"services":{"proxy":{"start":["run","svc"]}}}}}`,
	})

	captureStdout(t, func() {
		code := run([]string{"service", "remove", "proxy", "--file", specPath, "--target", "arch"})
		if code != exitLogic {
			t.Fatalf("service remove of a module-owned service = %d, want %d", code, exitLogic)
		}
	})
}

func TestGuard_names_multiple_owning_modules(t *testing.T) {
	dir := t.TempDir()
	specPath := filepath.Join(dir, "genv.json")
	composeWriteFixture(t, dir, `{
	  "schemaVersion": "10",
	  "modules": { "one": "modules/one.json", "two": "modules/two.json" },
	  "targets": { "arch": { "useModules": ["one", "two"] } }
	}`, map[string]string{
		// Identical declarations coalesce and keep both owners, which is the
		// case where a guard must name more than one module.
		"modules/one.json": `{"schemaVersion":"10","defaults":{"env":{"SHARED":{"value":"a"}}}}`,
		"modules/two.json": `{"schemaVersion":"10","defaults":{"env":{"SHARED":{"value":"a"}}}}`,
	})
	f, err := genvfile.Read(specPath)
	if err != nil {
		t.Fatalf("read spec: %v", err)
	}
	c, code := materializeComposition("test", specPath, f, "", "arch", dir)
	if code != exitOK {
		t.Fatalf("compose: %d", code)
	}
	if got := moduleOwnerGuard("env set", c, compose.KindEnv, "SHARED"); got != exitLogic {
		t.Fatalf("guard = %d, want %d", got, exitLogic)
	}
	// Root-owned and unknown keys stay mutable.
	if got := moduleOwnerGuard("env set", c, compose.KindEnv, "MINE"); got != exitOK {
		t.Errorf("guard on an unowned var = %d, want %d", got, exitOK)
	}
	if got := moduleOwnerGuard("env set", c, compose.KindEnv, ""); got != exitOK {
		t.Errorf("guard on an empty key = %d, want %d", got, exitOK)
	}
	if got := moduleOwnerGuard("env set", nil, compose.KindEnv, "SHARED"); got != exitOK {
		t.Errorf("guard with no composition = %d, want %d", got, exitOK)
	}
}

func TestGuardShortCircuits_specs_without_modules(t *testing.T) {
	dir := composeFixture(t, `{"schemaVersion":"8","targets":{"macos":{"packages":[{"id":"jq"}]}}}`, nil)
	specPath := filepath.Join(dir, "genv.json")
	f, err := genvfile.Read(specPath)
	if err != nil {
		t.Fatalf("read spec: %v", err)
	}
	// moduleOwnerGuardFor must not even compose when nothing can be module-owned.
	if got := moduleOwnerGuardFor("add", specPath, f, "", "macos", "", compose.KindPackage, "jq"); got != exitOK {
		t.Errorf("guard on a module-less spec = %d, want %d", got, exitOK)
	}
	if got := moduleOwnerGuardFor("add", specPath, nil, "", "macos", "", compose.KindPackage, "jq"); got != exitOK {
		t.Errorf("guard with a nil spec = %d, want %d", got, exitOK)
	}
}

func TestFilesAdopt_refuses_module_owned_link(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "xdg"))
	specPath := filepath.Join(dir, "genv.json")
	lockPath := filepath.Join(dir, "genv.lock.json")
	composeWriteFixture(t, dir, `{
	  "schemaVersion": "10",
	  "modules": { "conf": "modules/conf.json" },
	  "targets": { "arch": { "useModules": ["conf"] } }
	}`, map[string]string{
		"modules/conf.json": `{"schemaVersion":"10","defaults":{"files":{"links":[{"source":"conf/app.conf","target":"/etc/genv-app.conf"}]}}}`,
	})
	// The module's source file does not exist: adoption must refuse on ownership
	// before it gets as far as resolving or backing anything up.
	captureStdout(t, func() {
		code := run([]string{"files", "adopt", "/etc/genv-app.conf", "--file", specPath, "--lock-file", lockPath, "--target", "arch"})
		if code != exitLogic {
			t.Fatalf("files adopt of a module-owned link = %d, want %d", code, exitLogic)
		}
	})
}

// --- export (Task 6) ------------------------------------------------------

func TestExportCmd_v10_materializes_modules(t *testing.T) {
	dir := twoModuleFixture(t)
	specPath := filepath.Join(dir, "genv.json")
	out := filepath.Join(dir, "out")

	captureStdout(t, func() {
		if code := run([]string{"export", "--file", specPath, "--target", "macos", "--out", out}); code != exitOK {
			t.Fatalf("export exit = %d, want %d", code, exitOK)
		}
	})

	snapshot, err := os.ReadFile(filepath.Join(out, "genv.json"))
	if err != nil {
		t.Fatalf("read snapshot: %v", err)
	}
	flat, errs, parseErr := schema.ParseAndValidate(snapshot)
	if parseErr != nil || len(errs) > 0 {
		t.Fatalf("snapshot invalid: %v %v", parseErr, errs)
	}
	ids := map[string]bool{}
	for _, p := range flat.Targets["macos"].Packages {
		ids[p.ID] = true
	}
	for _, want := range []string{"root-pkg", "jq", "ripgrep"} {
		if !ids[want] {
			t.Errorf("snapshot missing module package %q (got %v)", want, ids)
		}
	}
	// The snapshot is flat: no module registry, nothing to resolve on import.
	if len(flat.Modules) != 0 {
		t.Errorf("snapshot should not carry a module registry: %v", flat.Modules)
	}
}

func TestExportCmd_v10_records_module_attribution(t *testing.T) {
	dir := twoModuleFixture(t)
	specPath := filepath.Join(dir, "genv.json")
	out := filepath.Join(dir, "out")

	captureStdout(t, func() {
		if code := run([]string{"export", "--file", specPath, "--target", "macos", "--out", out}); code != exitOK {
			t.Fatalf("export exit = %d, want %d", code, exitOK)
		}
	})

	data, err := os.ReadFile(filepath.Join(out, "report.json"))
	if err != nil {
		t.Fatalf("read report: %v", err)
	}
	if !strings.Contains(string(data), "module.materialized") {
		t.Errorf("report should attribute materialized modules:\n%s", data)
	}
	for _, name := range []string{"base", "tools"} {
		if !strings.Contains(string(data), name) {
			t.Errorf("report should name module %q:\n%s", name, data)
		}
	}
}

func TestExportCmd_v8_unchanged(t *testing.T) {
	dir := composeFixture(t, `{
	  "schemaVersion": "8",
	  "defaults": { "packages": [{ "id": "git" }] },
	  "targets": { "macos": { "packages": [{ "id": "jq" }] } }
	}`, nil)
	specPath := filepath.Join(dir, "genv.json")
	out := filepath.Join(dir, "out")
	captureStdout(t, func() {
		if code := run([]string{"export", "--file", specPath, "--target", "macos", "--out", out}); code != exitOK {
			t.Fatalf("export exit = %d", code)
		}
	})
	report, err := os.ReadFile(filepath.Join(out, "report.json"))
	if err != nil {
		t.Fatalf("read report: %v", err)
	}
	if strings.Contains(string(report), "module.materialized") {
		t.Errorf("a module-less export must not claim module attribution:\n%s", report)
	}
}

// --- config / explain (Task 7) -------------------------------------------

func TestConfigCmd_summarizes_composition(t *testing.T) {
	dir := twoModuleFixture(t)
	specPath := filepath.Join(dir, "genv.json")

	out := captureStdout(t, func() {
		if code := run([]string{"config", "--file", specPath, "--target", "macos"}); code != exitOK {
			t.Fatalf("config exit = %d", code)
		}
	})
	for _, want := range []string{"target: macos", "modules: base, tools", "fingerprint:", "packages   3"} {
		if !strings.Contains(out, want) {
			t.Errorf("config output missing %q:\n%s", want, out)
		}
	}
}

func TestConfigCmd_json_matches_text(t *testing.T) {
	dir := twoModuleFixture(t)
	specPath := filepath.Join(dir, "genv.json")

	out := captureStdout(t, func() {
		if code := run([]string{"config", "--file", specPath, "--target", "macos", "--json"}); code != exitOK {
			t.Fatalf("config exit = %d", code)
		}
	})
	var got struct {
		Target      string   `json:"target"`
		Modules     []string `json:"modules"`
		Fingerprint string   `json:"fingerprint"`
		Counts      struct {
			Packages int `json:"packages"`
		} `json:"counts"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("parse: %v\n%s", err, out)
	}
	if got.Target != "macos" || len(got.Modules) != 2 || got.Counts.Packages != 3 || got.Fingerprint == "" {
		t.Errorf("json = %+v", got)
	}
}

func TestConfigCmd_registry_lists_unselected_modules(t *testing.T) {
	dir := composeFixture(t, `{
	  "schemaVersion": "10",
	  "modules": { "used": "modules/used.json", "idle": "modules/idle.json" },
	  "targets": { "macos": { "useModules": ["used"] } }
	}`, map[string]string{
		"modules/used.json": `{"schemaVersion":"10","defaults":{"packages":[{"id":"jq"}]}}`,
	})
	specPath := filepath.Join(dir, "genv.json")

	out := captureStdout(t, func() {
		if code := run([]string{"config", "--file", specPath, "--registry"}); code != exitOK {
			t.Fatalf("config --registry exit = %d", code)
		}
	})
	if !strings.Contains(out, "selected by: macos") {
		t.Errorf("registry should show which target selects a module:\n%s", out)
	}
	if !strings.Contains(out, "not selected by any target") {
		t.Errorf("registry should mark unselected modules:\n%s", out)
	}
}

func TestExplainCmd_reports_owner_and_ownership(t *testing.T) {
	dir := twoModuleFixture(t)
	specPath := filepath.Join(dir, "genv.json")

	out := captureStdout(t, func() {
		// Flags after positionals: explain must still honour them.
		if code := run([]string{"explain", "package", "jq", "--file", specPath, "--target", "macos"}); code != exitOK {
			t.Fatalf("explain exit = %d", code)
		}
	})
	if !strings.Contains(out, `module "base"`) || !strings.Contains(out, "modules/base.json") {
		t.Errorf("explain should name the owning module and document:\n%s", out)
	}
	if !strings.Contains(out, "owned by a module") {
		t.Errorf("explain should say the resource is not editable:\n%s", out)
	}
}

func TestExplainCmd_root_owned_resource_is_editable(t *testing.T) {
	dir := twoModuleFixture(t)
	specPath := filepath.Join(dir, "genv.json")
	out := captureStdout(t, func() {
		if code := run([]string{"explain", "package", "root-pkg", "--file", specPath, "--target", "macos"}); code != exitOK {
			t.Fatalf("explain exit = %d", code)
		}
	})
	if !strings.Contains(out, "genv.json (root)") {
		t.Errorf("explain should attribute a root resource to genv.json:\n%s", out)
	}
	if strings.Contains(out, "owned by a module") {
		t.Errorf("a root-owned resource must not be reported as module-owned:\n%s", out)
	}
}

func TestExplainCmd_absent_resource_is_not_an_error(t *testing.T) {
	dir := twoModuleFixture(t)
	specPath := filepath.Join(dir, "genv.json")
	out := captureStdout(t, func() {
		if code := run([]string{"explain", "package", "nonexistent", "--file", specPath, "--target", "macos"}); code != exitOK {
			t.Fatalf("explain of an absent resource = %d, want %d", code, exitOK)
		}
	})
	if !strings.Contains(out, "is not part of this environment") {
		t.Errorf("unexpected output: %s", out)
	}
}

func TestExplainCmd_unknown_kind_lists_valid_kinds(t *testing.T) {
	dir := twoModuleFixture(t)
	specPath := filepath.Join(dir, "genv.json")
	captureStdout(t, func() {
		if code := run([]string{"explain", "packagez", "jq", "--file", specPath, "--target", "macos"}); code != exitUsage {
			t.Fatalf("unknown kind exit = %d, want %d", code, exitUsage)
		}
	})
}

func TestExplainCmd_requires_both_arguments(t *testing.T) {
	dir := twoModuleFixture(t)
	specPath := filepath.Join(dir, "genv.json")
	captureStdout(t, func() {
		if code := run([]string{"explain", "package", "--file", specPath, "--target", "macos"}); code != exitUsage {
			t.Fatalf("explain with one argument = %d, want %d", code, exitUsage)
		}
	})
}

func TestConfigCmd_requires_kind_and_name_together(t *testing.T) {
	dir := twoModuleFixture(t)
	specPath := filepath.Join(dir, "genv.json")
	captureStdout(t, func() {
		if code := run([]string{"config", "--file", specPath, "--target", "macos", "--kind", "package"}); code != exitUsage {
			t.Fatalf("--kind without --name = %d, want %d", code, exitUsage)
		}
	})
}

func TestConfigCmd_v8_works_without_modules(t *testing.T) {
	dir := composeFixture(t, `{"schemaVersion":"8","targets":{"macos":{"packages":[{"id":"jq"}]}}}`, nil)
	specPath := filepath.Join(dir, "genv.json")
	out := captureStdout(t, func() {
		if code := run([]string{"config", "--file", specPath, "--target", "macos"}); code != exitOK {
			t.Fatalf("config exit = %d", code)
		}
	})
	if !strings.Contains(out, "modules: none") {
		t.Errorf("v8 config should report no modules:\n%s", out)
	}
}

func TestConfigCmd_reports_composition_failure(t *testing.T) {
	dir := composeFixture(t, `{
	  "schemaVersion": "10",
	  "modules": { "ghost": "modules/ghost.json" },
	  "targets": { "macos": { "useModules": ["ghost"] } }
	}`, nil)
	specPath := filepath.Join(dir, "genv.json")
	captureStdout(t, func() {
		if code := run([]string{"config", "--file", specPath, "--target", "macos"}); code != exitValidation {
			t.Fatalf("config with a missing module = %d, want %d", code, exitValidation)
		}
	})
}

func TestConfigCmd_json_kinds_and_explain_json_shape(t *testing.T) {
	dir := twoModuleFixture(t)
	specPath := filepath.Join(dir, "genv.json")
	out := captureStdout(t, func() {
		if code := run([]string{"config", "--file", specPath, "--target", "macos", "--kind", "package", "--name", "jq", "--json"}); code != exitOK {
			t.Fatalf("config --kind exit = %d", code)
		}
	})
	var got struct {
		Found    bool `json:"found"`
		Editable bool `json:"editable"`
		Owners   []struct {
			Module string `json:"module"`
		} `json:"owners"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("parse: %v\n%s", err, out)
	}
	if !got.Found || got.Editable || len(got.Owners) != 1 || got.Owners[0].Module != "base" {
		t.Errorf("json = %+v", got)
	}
}
