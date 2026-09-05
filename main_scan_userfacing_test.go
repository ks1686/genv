package main

import (
	"encoding/json"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/ks1686/genv/internal/adapter"
	"github.com/ks1686/genv/internal/genvfile"
	"github.com/ks1686/genv/internal/schema"
)

// scanLeavesAdapter is a VersionLister whose full inventory includes
// dependencies, plus a ScanLister that reports only user-facing names.
type scanLeavesAdapter struct {
	name      string
	installed []string
	leaves    []string
	versions  map[string]string

	listCalls     int
	versionCalls  int
	scanListCalls int
}

func (a *scanLeavesAdapter) Name() string { return a.name }
func (a *scanLeavesAdapter) Available() bool {
	return true
}
func (a *scanLeavesAdapter) NormalizeID(id string, managers map[string]string) (string, bool) {
	return id, false
}
func (a *scanLeavesAdapter) PlanInstall(pkgName string) []string   { return []string{"true"} }
func (a *scanLeavesAdapter) PlanUninstall(pkgName string) []string { return []string{"true"} }
func (a *scanLeavesAdapter) PlanUpgrade(pkgName string) []string   { return []string{"true"} }
func (a *scanLeavesAdapter) PlanClean() [][]string                 { return nil }
func (a *scanLeavesAdapter) Query(pkgName string) (bool, error)    { return true, nil }
func (a *scanLeavesAdapter) ListInstalled() ([]string, error) {
	a.listCalls++
	return a.installed, nil
}
func (a *scanLeavesAdapter) QueryVersion(pkgName string) (string, error) { return "", nil }
func (a *scanLeavesAdapter) ListInstalledVersions() (map[string]string, error) {
	a.versionCalls++
	return a.versions, nil
}
func (a *scanLeavesAdapter) ListForScan() ([]string, error) {
	a.scanListCalls++
	return a.leaves, nil
}

func TestScanCmd_DefaultUsesLeavesNotFullBrewList(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	path := filepath.Join(dir, "genv.json")
	lockPath := filepath.Join(dir, "genv.lock.json")

	brew := &scanLeavesAdapter{
		name:      "brew",
		installed: []string{"git", "openssl@3", "libpng", "iterm2"},
		leaves:    []string{"git", "iterm2"},
		versions: map[string]string{
			"git":       "2.45.0",
			"openssl@3": "3.4.0",
			"libpng":    "1.6.43",
			"iterm2":    "3.5.0",
		},
	}
	originalAll := adapter.All
	originalGOOS := scanGOOS
	adapter.All = []adapter.Adapter{brew}
	scanGOOS = "darwin"
	t.Cleanup(func() {
		adapter.All = originalAll
		scanGOOS = originalGOOS
	})

	var code int
	out := captureStdout(t, func() {
		code = run([]string{"scan", "--file", path, "--lock-file", lockPath, "--dry-run", "--json", "--target", "macos"})
	})
	if code != exitOK {
		t.Fatalf("scan --dry-run: expected exitOK, got %d\n%s", code, out)
	}
	ids := scanDryRunIDs(t, out)
	if len(ids) != 2 || !containsAll(ids, "git", "iterm2") {
		t.Fatalf("default scan packages = %v, want git and iterm2 (not brew deps)", ids)
	}
	for _, noise := range []string{"openssl@3", "libpng"} {
		if slices.Contains(ids, noise) {
			t.Errorf("default scan proposed brew dependency %q", noise)
		}
	}
	if brew.scanListCalls != 1 {
		t.Errorf("ListForScan calls = %d, want 1", brew.scanListCalls)
	}
}

func TestScanCmd_AllIncludesBrewDeps(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	path := filepath.Join(dir, "genv.json")
	lockPath := filepath.Join(dir, "genv.lock.json")

	brew := &scanLeavesAdapter{
		name:      "brew",
		installed: []string{"git", "openssl@3"},
		leaves:    []string{"git"},
		versions:  map[string]string{"git": "2.45.0", "openssl@3": "3.4.0"},
	}
	originalAll := adapter.All
	originalGOOS := scanGOOS
	adapter.All = []adapter.Adapter{brew}
	scanGOOS = "darwin"
	t.Cleanup(func() {
		adapter.All = originalAll
		scanGOOS = originalGOOS
	})

	var code int
	out := captureStdout(t, func() {
		code = run([]string{"scan", "--file", path, "--lock-file", lockPath, "--dry-run", "--json", "--all", "--target", "macos"})
	})
	if code != exitOK {
		t.Fatalf("scan --all --dry-run: expected exitOK, got %d\n%s", code, out)
	}
	ids := scanDryRunIDs(t, out)
	if !containsAll(ids, "git", "openssl@3") {
		t.Fatalf("--all packages = %v, want git and openssl@3", ids)
	}
	if brew.scanListCalls != 0 {
		t.Errorf("--all must not call ListForScan; got %d calls", brew.scanListCalls)
	}
}

func TestScanCmd_DepsAliasIncludesBrewDeps(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	path := filepath.Join(dir, "genv.json")
	lockPath := filepath.Join(dir, "genv.lock.json")

	brew := &scanLeavesAdapter{
		name:      "brew",
		installed: []string{"git", "openssl@3"},
		leaves:    []string{"git"},
		versions:  map[string]string{"git": "2.45.0", "openssl@3": "3.4.0"},
	}
	originalAll := adapter.All
	originalGOOS := scanGOOS
	adapter.All = []adapter.Adapter{brew}
	scanGOOS = "darwin"
	t.Cleanup(func() {
		adapter.All = originalAll
		scanGOOS = originalGOOS
	})

	var code int
	out := captureStdout(t, func() {
		code = run([]string{"scan", "--file", path, "--lock-file", lockPath, "--dry-run", "--json", "--deps", "--target", "macos"})
	})
	if code != exitOK {
		t.Fatalf("scan --deps --dry-run: expected exitOK, got %d\n%s", code, out)
	}
	ids := scanDryRunIDs(t, out)
	if !containsAll(ids, "git", "openssl@3") {
		t.Fatalf("--deps packages = %v, want git and openssl@3", ids)
	}
}

func TestScanCmd_DryRunNearEmptyWhenLeavesTracked(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	path := filepath.Join(dir, "genv.json")
	lockPath := filepath.Join(dir, "genv.lock.json")

	seed := &schema.GenvFile{
		SchemaVersion: schema.Version8,
		Targets: map[string]*schema.TargetBundle{
			"macos": {Packages: []schema.Package{{ID: "git"}, {ID: "iterm2"}}},
		},
	}
	if err := genvfile.Write(path, seed); err != nil {
		t.Fatalf("seeding spec: %v", err)
	}

	brew := &scanLeavesAdapter{
		name:      "brew",
		installed: []string{"git", "openssl@3", "libpng", "gettext", "iterm2"},
		leaves:    []string{"git", "iterm2"},
		versions: map[string]string{
			"git":       "2.45.0",
			"openssl@3": "3.4.0",
			"libpng":    "1.6.43",
			"gettext":   "0.22",
			"iterm2":    "3.5.0",
		},
	}
	originalAll := adapter.All
	originalGOOS := scanGOOS
	adapter.All = []adapter.Adapter{brew}
	scanGOOS = "darwin"
	t.Cleanup(func() {
		adapter.All = originalAll
		scanGOOS = originalGOOS
	})

	var code int
	out := captureStdout(t, func() {
		code = run([]string{"scan", "--file", path, "--lock-file", lockPath, "--dry-run", "--json", "--target", "macos"})
	})
	if code != exitOK {
		t.Fatalf("scan --dry-run: expected exitOK, got %d\n%s", code, out)
	}
	ids := scanDryRunIDs(t, out)
	if len(ids) != 0 {
		t.Fatalf("tracked-leaves host dry-run packages = %v, want none", ids)
	}

	out = captureStdout(t, func() {
		code = run([]string{"scan", "--file", path, "--lock-file", lockPath, "--dry-run", "--json", "--all", "--target", "macos"})
	})
	if code != exitOK {
		t.Fatalf("scan --all --dry-run: expected exitOK, got %d\n%s", code, out)
	}
	ids = scanDryRunIDs(t, out)
	if !containsAll(ids, "openssl@3", "libpng", "gettext") {
		t.Fatalf("--all on tracked-leaves host = %v, want leftover brew deps", ids)
	}
}

func TestScanCmd_NeverProposesDashNpmOrToolchain(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	path := filepath.Join(dir, "genv.json")
	lockPath := filepath.Join(dir, "genv.lock.json")

	uv := &scanLeavesAdapter{
		name:      "uv",
		installed: []string{"-", "ruff"},
		leaves:    []string{"-", "ruff"},
		versions:  map[string]string{"-": "", "ruff": "0.6.9"},
	}
	npm := &scanLeavesAdapter{
		name:      "npm",
		installed: []string{"npm", "corepack", "typescript"},
		leaves:    []string{"npm", "corepack", "typescript"},
		versions:  map[string]string{"npm": "10.9.2", "corepack": "0.31.0", "typescript": "5.9.2"},
	}
	rustup := &scanLeavesAdapter{
		name:      "rustup",
		installed: []string{"toolchain:stable-aarch64-apple-darwin"},
		leaves:    []string{"toolchain:stable-aarch64-apple-darwin"},
		versions:  map[string]string{"toolchain:stable-aarch64-apple-darwin": "stable-aarch64-apple-darwin"},
	}

	originalAll := adapter.All
	originalGOOS := scanGOOS
	adapter.All = []adapter.Adapter{uv, npm, rustup}
	scanGOOS = "darwin"
	t.Cleanup(func() {
		adapter.All = originalAll
		scanGOOS = originalGOOS
	})

	for _, args := range [][]string{
		{"scan", "--file", path, "--lock-file", lockPath, "--dry-run", "--json", "--target", "macos"},
		{"scan", "--file", path, "--lock-file", lockPath, "--dry-run", "--json", "--all", "--target", "macos"},
	} {
		var code int
		out := captureStdout(t, func() {
			code = run(args)
		})
		if code != exitOK {
			t.Fatalf("%v: expected exitOK, got %d\n%s", args, code, out)
		}
		ids := scanDryRunIDs(t, out)
		for _, id := range ids {
			if id == "-" || id == "npm" || strings.HasPrefix(id, "toolchain:") {
				t.Errorf("%v proposed non-package id %q in %v", args, id, ids)
			}
		}
		if !slices.Contains(ids, "ruff") {
			t.Errorf("%v missing uv tool name ruff: %v", args, ids)
		}
		if !slices.Contains(ids, "corepack") || !slices.Contains(ids, "typescript") {
			t.Errorf("%v missing user npm globals: %v", args, ids)
		}
	}
}

func TestScanCmd_HelpDescribesUserFacingInventory(t *testing.T) {
	var code int
	out := captureStderr(t, func() {
		code = run([]string{"scan", "--help"})
	})
	if code != exitUsage {
		t.Fatalf("scan --help: expected exitUsage (%d), got %d\n%s", exitUsage, code, out)
	}
	for _, want := range []string{"leaves", "casks", "--all", "user-facing"} {
		if !strings.Contains(strings.ToLower(out), strings.ToLower(want)) {
			t.Errorf("scan --help missing %q\n%s", want, out)
		}
	}
}

func scanDryRunIDs(t *testing.T, out string) []string {
	t.Helper()
	var raw map[string]any
	if err := json.Unmarshal([]byte(out), &raw); err != nil {
		t.Fatalf("json: %v\n%s", err, out)
	}
	data, _ := raw["data"].(map[string]any)
	rawPkgs, _ := data["packages"].([]any)
	ids := make([]string, 0, len(rawPkgs))
	for _, p := range rawPkgs {
		id, _ := p.(string)
		ids = append(ids, id)
	}
	return ids
}

func containsAll(got []string, want ...string) bool {
	for _, w := range want {
		if !slices.Contains(got, w) {
			return false
		}
	}
	return true
}
