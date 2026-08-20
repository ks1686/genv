package resolver

import (
	"bytes"
	"context"
	"io"
	"os/exec"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ks1686/genv/internal/adapter"
	"github.com/ks1686/genv/internal/genvfile"
	"github.com/ks1686/genv/internal/schema"
)

func TestRunSubcmd_EmptyArgv(t *testing.T) {
	err := runSubcmd(context.Background(), nil, nil, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected empty argv to fail")
	}
	if !strings.Contains(err.Error(), "empty command") {
		t.Fatalf("error = %v, want empty command", err)
	}
}

func TestRunSubcmd_PerSpawnTimeout(t *testing.T) {
	if _, err := exec.LookPath("sleep"); err != nil {
		t.Skip("sleep not in PATH")
	}
	ctx := WithSubprocessTimeout(context.Background(), 2*time.Second)
	err := runSubcmd(ctx, []string{"sleep", "5"}, nil, io.Discard, io.Discard)
	if err == nil {
		t.Fatal("expected sleep to hit per-spawn timeout")
	}
	// `true` is not reliable on Windows CI (LookPath can find a non-POSIX
	// true.exe that exits 1). `go` is on PATH wherever these tests run.
	// 50ms was too tight: after killing sleep, `go env GOVERSION` often
	// exceeds 50ms on Windows runners and fails the reuse assertion.
	if err := runSubcmd(ctx, []string{"go", "env", "GOVERSION"}, nil, io.Discard, io.Discard); err != nil {
		t.Fatalf("later command after a timed-out spawn: %v", err)
	}
}

func TestPlan_PreferredManagerAvailable(t *testing.T) {
	f := &schema.GenvFile{
		Packages: []schema.Package{
			{ID: "neovim", Prefer: "brew"},
		},
	}
	actions := Plan(f, map[string]bool{"brew": true})
	if len(actions) != 1 {
		t.Fatalf("expected 1 action, got %d", len(actions))
	}
	a := actions[0]
	if !a.Resolved() {
		t.Fatal("expected resolved")
	}
	if a.Manager != "brew" {
		t.Errorf("manager: got %q, want %q", a.Manager, "brew")
	}
	if a.PkgName != "neovim" {
		t.Errorf("pkgName: got %q, want %q", a.PkgName, "neovim")
	}
}

func TestPlan_PreferredManagerUnavailable_FallsBackToAvailable(t *testing.T) {
	f := &schema.GenvFile{
		Packages: []schema.Package{
			{ID: "neovim", Prefer: "brew"},
		},
	}
	// brew not available; paru is
	actions := Plan(f, map[string]bool{"paru": true})
	a := actions[0]
	if !a.Resolved() {
		t.Fatal("expected resolved via fallback")
	}
	if a.Manager != "paru" {
		t.Errorf("manager: got %q, want %q", a.Manager, "paru")
	}
}

func TestPlan_ManagersMapPicksCorrectName(t *testing.T) {
	f := &schema.GenvFile{
		Packages: []schema.Package{
			{
				ID: "hello",
				Managers: map[string]string{
					"snap": "hello",
					"brew": "hello",
				},
			},
		},
	}
	actions := Plan(f, map[string]bool{"snap": true})
	a := actions[0]
	if !a.Resolved() {
		t.Fatal("expected resolved")
	}
	if a.Manager != "snap" {
		t.Errorf("manager: got %q, want %q", a.Manager, "snap")
	}
	if a.PkgName != "hello" {
		t.Errorf("pkgName: got %q, want %q", a.PkgName, "hello")
	}
}

func TestPlan_ManagersMap_FallbackOrder(t *testing.T) {
	// Both brew and snap are in managers map; brew is first in fallbackOrder.
	f := &schema.GenvFile{
		Packages: []schema.Package{
			{
				ID: "hello",
				Managers: map[string]string{
					"snap": "hello",
					"brew": "hello",
				},
			},
		},
	}
	actions := Plan(f, map[string]bool{"brew": true, "snap": true})
	a := actions[0]
	if a.Manager != "brew" {
		t.Errorf("expected brew (higher priority), got %q", a.Manager)
	}
}

func TestPlan_Unresolved_NoManagersAvailable(t *testing.T) {
	f := &schema.GenvFile{
		Packages: []schema.Package{
			{ID: "git"},
		},
	}
	actions := Plan(f, map[string]bool{})
	a := actions[0]
	if a.Resolved() {
		t.Fatal("expected unresolved")
	}
	if a.Cmd != nil {
		t.Error("unresolved action should have nil Cmd")
	}
	if a.Manager != "" {
		t.Errorf("unresolved Manager should be empty, got %q", a.Manager)
	}
}

func TestPlan_FallsBackToIDWhenNoManagersMap(t *testing.T) {
	f := &schema.GenvFile{
		Packages: []schema.Package{
			{ID: "git"}, // no managers map, no prefer
		},
	}
	actions := Plan(f, map[string]bool{"paru": true})
	a := actions[0]
	if !a.Resolved() {
		t.Fatal("expected resolved via generic fallback")
	}
	if a.PkgName != "git" {
		t.Errorf("pkgName: got %q, want %q", a.PkgName, "git")
	}
}

func TestPlan_PreferWithManagersMap_UsesMapName(t *testing.T) {
	f := &schema.GenvFile{
		Packages: []schema.Package{
			{
				ID:     "neovim",
				Prefer: "brew",
				Managers: map[string]string{
					"brew": "neovim",
					"paru": "neovim",
				},
			},
		},
	}
	actions := Plan(f, map[string]bool{"brew": true})
	a := actions[0]
	if a.Manager != "brew" {
		t.Errorf("manager: got %q, want %q", a.Manager, "brew")
	}
	if a.PkgName != "neovim" {
		t.Errorf("pkgName: got %q, want %q", a.PkgName, "neovim")
	}
}

func TestPrintPlan_NoCrash_AllUnresolved(t *testing.T) {
	f := &schema.GenvFile{
		Packages: []schema.Package{
			{ID: "git"},
			{ID: "neovim", Prefer: "brew"},
		},
	}
	actions := Plan(f, map[string]bool{}) // no managers
	var sb strings.Builder
	PrintPlan(actions, &sb) // must not panic
	out := sb.String()
	if !strings.Contains(out, "git") {
		t.Error("expected git in plan output")
	}
	if !strings.Contains(out, "unresolved") {
		t.Error("expected 'unresolved' in plan output")
	}
}

func TestPrintPlan_ShowsInstallCommand(t *testing.T) {
	f := &schema.GenvFile{
		Packages: []schema.Package{
			{ID: "git"},
		},
	}
	actions := planOnGOOS(f, map[string]bool{"brew": true}, "darwin")
	var sb strings.Builder
	PrintPlan(actions, &sb)
	out := sb.String()
	if !strings.Contains(out, "brew install git") {
		t.Errorf("expected 'brew install git' in output, got:\n%s", out)
	}
}

func TestPrintPlan_MixedResolved(t *testing.T) {
	f := &schema.GenvFile{
		Packages: []schema.Package{
			{ID: "git"},
			{ID: "mystery-pkg"},
		},
	}
	// Only git gets resolved (brew available, mystery-pkg falls back to brew too)
	// Actually both would resolve via brew... Let's test with an empty available set
	// so both are unresolved, and separately test with brew so both resolve.
	available := map[string]bool{"brew": true}
	actions := Plan(f, available)

	var sb strings.Builder
	PrintPlan(actions, &sb)
	out := sb.String()
	if !strings.Contains(out, "git") || !strings.Contains(out, "mystery-pkg") {
		t.Errorf("expected both packages in output:\n%s", out)
	}
}

func TestPlanInstall_AllManagers(t *testing.T) {
	tests := []struct {
		mgr     string
		pkg     string
		wantBin string
	}{
		{"paru", "git", "paru"},
		{"yay", "git", "yay"},
		{"snap", "git", "sudo"},
		{"brew", "git", "brew"},
		{"linuxbrew", "git", "brew"},
	}
	for _, tc := range tests {
		a := adapter.ByName(tc.mgr)
		if a == nil {
			t.Errorf("ByName(%q): no adapter found", tc.mgr)
			continue
		}
		args := a.PlanInstall(tc.pkg)
		if len(args) == 0 {
			t.Errorf("PlanInstall(%q, %q): got empty slice", tc.mgr, tc.pkg)
			continue
		}
		if args[0] != tc.wantBin {
			t.Errorf("PlanInstall(%q, %q): binary = %q, want %q", tc.mgr, tc.pkg, args[0], tc.wantBin)
		}
		if args[len(args)-1] != tc.pkg {
			t.Errorf("PlanInstall(%q, %q): last arg = %q, want pkg name", tc.mgr, tc.pkg, args[len(args)-1])
		}
	}
}

func TestPlanUpgrade_SkipsMissingManagers(t *testing.T) {
	packages := []genvfile.LockedPackage{
		{ID: "git", Manager: "brew", PkgName: "git"},
		{ID: "legacy", Manager: "yum", PkgName: "legacy"},
	}

	plan, skipped := PlanUpgrade(packages)

	if len(plan) != 1 {
		t.Fatalf("expected 1 upgrade action, got %d", len(plan))
	}
	if len(skipped) != 1 {
		t.Fatalf("expected 1 skipped package, got %d", len(skipped))
	}
	if len(plan[0].LPs) != 1 || plan[0].LPs[0].ID != "git" {
		t.Fatalf("expected git upgrade action, got %v", plan[0].LPs)
	}
	if got := skipped[0].ID; got != "legacy" {
		t.Fatalf("expected legacy to be skipped, got %q", got)
	}
}

func TestPlanUpgrade_BatchesSameManager(t *testing.T) {
	packages := []genvfile.LockedPackage{
		{ID: "git", Manager: "brew", PkgName: "git"},
		{ID: "neovim", Manager: "brew", PkgName: "neovim"},
		{ID: "ruff", Manager: "uv", PkgName: "ruff"},
	}

	plan, skipped := PlanUpgrade(packages)
	if len(skipped) != 0 {
		t.Fatalf("expected no skipped packages, got %v", skipped)
	}
	if len(plan) != 2 {
		t.Fatalf("expected 2 upgrade actions (brew batch + uv single), got %d", len(plan))
	}

	// Brew packages should be batched into one action.
	brewAction := plan[0]
	if len(brewAction.LPs) != 2 {
		t.Fatalf("expected brew action with 2 packages, got %d", len(brewAction.LPs))
	}
	if brewAction.LPs[0].ID != "git" || brewAction.LPs[1].ID != "neovim" {
		t.Errorf("expected brew action ids [git neovim], got %v", brewAction.LPs)
	}
	want := []string{"brew", "upgrade", "git", "neovim"}
	if len(brewAction.Cmd) != len(want) {
		t.Fatalf("brew command: got %v, want %v", brewAction.Cmd, want)
	}
	for i, w := range want {
		if brewAction.Cmd[i] != w {
			t.Errorf("brew command[%d] = %q, want %q", i, brewAction.Cmd[i], w)
		}
	}

	// uv is not a BatchUpgrader, so it stays single-package.
	uvAction := plan[1]
	if len(uvAction.LPs) != 1 || uvAction.LPs[0].ID != "ruff" {
		t.Fatalf("expected uv action with [ruff], got %v", uvAction.LPs)
	}
	if len(uvAction.Cmd) == 0 || uvAction.Cmd[0] != "uv" {
		t.Fatalf("expected uv upgrade command, got %v", uvAction.Cmd)
	}
}

func TestPlanUpgrade_PreservesManagerOrder(t *testing.T) {
	packages := []genvfile.LockedPackage{
		{ID: "a", Manager: "snap", PkgName: "a"},
		{ID: "b", Manager: "brew", PkgName: "b"},
		{ID: "c", Manager: "snap", PkgName: "c"},
	}

	plan, _ := PlanUpgrade(packages)
	if len(plan) != 2 {
		t.Fatalf("expected 2 actions, got %d", len(plan))
	}
	if plan[0].Mgr.Name() != "snap" {
		t.Errorf("expected first action manager snap, got %q", plan[0].Mgr.Name())
	}
	if plan[1].Mgr.Name() != "brew" {
		t.Errorf("expected second action manager brew, got %q", plan[1].Mgr.Name())
	}
}

func TestExecuteUpgrade_BatchUsesVersionLister(t *testing.T) {
	mgr := &testBatchVersionListerMgr{versions: map[string]string{"git": "2.45.0", "neovim": "0.10.0"}}
	plan := []UpgradeAction{
		{
			LPs: []genvfile.LockedPackage{
				{ID: "git", Manager: "batchmgr", PkgName: "git", InstalledVersion: "2.44.0"},
				{ID: "neovim", Manager: "batchmgr", PkgName: "neovim", InstalledVersion: "0.9.0"},
			},
			Mgr: mgr,
			Cmd: []string{"true"},
		},
	}

	out := ExecuteUpgrade(context.Background(), plan, nil, &bytes.Buffer{}, &bytes.Buffer{})
	if len(out.Errors) != 0 {
		t.Fatalf("expected no errors, got %v", out.Errors)
	}
	if len(out.Upgraded) != 2 {
		t.Fatalf("expected 2 upgraded packages, got %d", len(out.Upgraded))
	}
	if mgr.listCalls != 1 {
		t.Fatalf("expected 1 ListInstalledVersions call, got %d", mgr.listCalls)
	}
	versions := map[string]string{}
	for _, lp := range out.Upgraded {
		versions[lp.ID] = lp.InstalledVersion
	}
	if versions["git"] != "2.45.0" {
		t.Errorf("git version = %q, want %q", versions["git"], "2.45.0")
	}
	if versions["neovim"] != "0.10.0" {
		t.Errorf("neovim version = %q, want %q", versions["neovim"], "0.10.0")
	}
}

func TestExecuteUpgrade_PartialFailureStillUpdatesChangedVersions(t *testing.T) {
	// Given: a failed batch command whose version query shows one package changed anyway.
	mgr := &testBatchVersionListerMgr{versions: map[string]string{"git": "2.45.0", "neovim": "0.9.0"}}
	plan := []UpgradeAction{
		{
			LPs: []genvfile.LockedPackage{
				{ID: "git", Manager: "batchmgr", PkgName: "git", InstalledVersion: "2.44.0"},
				{ID: "neovim", Manager: "batchmgr", PkgName: "neovim", InstalledVersion: "0.9.0"},
			},
			Mgr: mgr,
			Cmd: []string{"false"},
		},
	}

	// When: the batch executes.
	out := ExecuteUpgrade(context.Background(), plan, nil, &bytes.Buffer{}, &bytes.Buffer{})

	// Then: the legacy error and typed action failure are both retained.
	if len(out.Errors) != 1 {
		t.Fatalf("expected 1 error, got %d", len(out.Errors))
	}
	if len(out.Failures) != 1 {
		t.Fatalf("expected 1 typed failure, got %d", len(out.Failures))
	}
	if !slices.Equal(out.Failures[0].IDs, []string{"git", "neovim"}) {
		t.Fatalf("failure IDs = %v, want [git neovim]", out.Failures[0].IDs)
	}
	if out.Failures[0].Err == nil || out.Failures[0].Err.Error() != out.Errors[0].Error() {
		t.Fatalf("typed failure error = %v, legacy error = %v", out.Failures[0].Err, out.Errors[0])
	}
	if len(out.Upgraded) != 1 {
		t.Fatalf("expected 1 upgraded package (git), got %d", len(out.Upgraded))
	}
	if out.Upgraded[0].ID != "git" || out.Upgraded[0].InstalledVersion != "2.45.0" {
		t.Errorf("expected upgraded git 2.45.0, got %v", out.Upgraded[0])
	}
}

// testBatchVersionListerMgr is a minimal adapter implementing BatchUpgrader and
// VersionLister for resolver-level upgrade tests.
type testBatchVersionListerMgr struct {
	versions  map[string]string
	listCalls int
}

func (m *testBatchVersionListerMgr) Name() string { return "batchmgr" }

func (m *testBatchVersionListerMgr) Available() bool { return true }

func (m *testBatchVersionListerMgr) NormalizeID(id string, _ map[string]string) (string, bool) {
	return id, false
}

func (m *testBatchVersionListerMgr) PlanInstall(pkgName string) []string {
	return []string{"install", pkgName}
}

func (m *testBatchVersionListerMgr) PlanUninstall(pkgName string) []string {
	return []string{"uninstall", pkgName}
}

func (m *testBatchVersionListerMgr) PlanUpgrade(pkgName string) []string {
	return []string{"upgrade", pkgName}
}

func (m *testBatchVersionListerMgr) PlanUpgradeBatch(pkgNames []string) []string {
	return append([]string{"upgrade-batch"}, pkgNames...)
}

func (m *testBatchVersionListerMgr) PlanClean() [][]string { return nil }

func (m *testBatchVersionListerMgr) Query(pkgName string) (bool, error) {
	_, ok := m.versions[pkgName]
	return ok, nil
}

func (m *testBatchVersionListerMgr) ListInstalled() ([]string, error) {
	var names []string
	for name := range m.versions {
		names = append(names, name)
	}
	return names, nil
}

func (m *testBatchVersionListerMgr) QueryVersion(pkgName string) (string, error) {
	return m.versions[pkgName], nil
}

func (m *testBatchVersionListerMgr) ListInstalledVersions() (map[string]string, error) {
	m.listCalls++
	return m.versions, nil
}

func TestReconcile_RemovalPathSkipsMissingManagers(t *testing.T) {
	result := Reconcile(
		nil,
		[]genvfile.LockedPackage{
			{ID: "git", Manager: "brew", PkgName: "git"},
			{ID: "legacy", Manager: "yum", PkgName: "legacy"},
		},
		map[string]bool{"brew": true},
	)

	if len(result.ToRemove) != 1 {
		t.Fatalf("expected 1 removal action, got %d", len(result.ToRemove))
	}
	if got := result.ToRemove[0].Pkg.ID; got != "git" {
		t.Fatalf("expected git removal action, got %q", got)
	}
	if got := result.ToRemove[0].Manager; got != "brew" {
		t.Fatalf("expected brew removal action, got %q", got)
	}
	if len(result.Unchanged) != 1 {
		t.Fatalf("expected missing manager to stay locked, got %d unchanged", len(result.Unchanged))
	}
	if got := result.Unchanged[0].ID; got != "legacy" {
		t.Fatalf("expected legacy to remain in lock, got %q", got)
	}
	if len(result.Warnings) != 1 {
		t.Fatalf("expected 1 warning, got %d: %v", len(result.Warnings), result.Warnings)
	}
}

func TestByName_UnknownManager(t *testing.T) {
	a := adapter.ByName("yum")
	if a != nil {
		t.Errorf("ByName for unknown manager should return nil, got %v", a)
	}
}

func TestDetect_ReturnsMap(t *testing.T) {
	m := Detect()
	if m == nil {
		t.Error("Detect() should return a non-nil map")
	}
	// All values in the returned map must be true (only available managers are listed).
	for mgr, ok := range m {
		if !ok {
			t.Errorf("Detect(): map[%q] = false; only true entries should be present", mgr)
		}
	}
}

func TestExecute_SkipsUnresolved(t *testing.T) {
	// An unresolved action has an empty Cmd; Execute must skip it without error.
	actions := []Action{
		{Pkg: schema.Package{ID: "mystery"}, Manager: "", Cmd: nil},
	}
	var out, errOut bytes.Buffer
	errs := Execute(context.Background(), actions, nil, &out, &errOut)
	if len(errs) != 0 {
		t.Errorf("expected no errors for all-unresolved actions, got: %v", errs)
	}
	if out.Len() != 0 {
		t.Errorf("expected no stdout output for all-unresolved actions, got: %q", out.String())
	}
}

func TestExecute_RunsCommand(t *testing.T) {
	// Execute a real "echo" command and verify it produces output and no errors.
	actions := []Action{
		{
			Pkg:     schema.Package{ID: "echo-test"},
			Manager: "brew",
			PkgName: "echo-test",
			Cmd:     []string{"echo", "hello-from-execute"},
		},
	}
	var out, errOut bytes.Buffer
	errs := Execute(context.Background(), actions, nil, &out, &errOut)
	if len(errs) != 0 {
		t.Fatalf("Execute with 'echo': unexpected errors: %v", errs)
	}
	if !strings.Contains(out.String(), "hello-from-execute") {
		t.Errorf("expected 'hello-from-execute' in stdout, got: %q", out.String())
	}
}

func TestExecute_FailedCommand(t *testing.T) {
	// A command that exits non-zero should produce one error entry.
	actions := []Action{
		{
			Pkg:     schema.Package{ID: "failing-pkg"},
			Manager: "brew",
			PkgName: "failing-pkg",
			Cmd:     []string{"false"},
		},
	}
	var out, errOut bytes.Buffer
	errs := Execute(context.Background(), actions, nil, &out, &errOut)
	if len(errs) == 0 {
		t.Error("expected error for failing command, got none")
	}
}

func TestPrintPlan_SinglePackage(t *testing.T) {
	// Singular "package" (not "packages") in the header.
	f := &schema.GenvFile{
		Packages: []schema.Package{{ID: "git"}},
	}
	actions := Plan(f, map[string]bool{"brew": true})
	var sb strings.Builder
	PrintPlan(actions, &sb)
	out := sb.String()
	if !strings.Contains(out, "1 package") {
		t.Errorf("expected '1 package' (singular) in output, got:\n%s", out)
	}
	if strings.Contains(out, "1 packages") {
		t.Errorf("unexpected '1 packages' (plural) in output; should be singular:\n%s", out)
	}
}

func TestPrintPlan_UnresolvedHint(t *testing.T) {
	// When unresolved packages exist the output must mention the hint lines.
	f := &schema.GenvFile{
		Packages: []schema.Package{{ID: "mystery"}},
	}
	actions := Plan(f, map[string]bool{})
	var sb strings.Builder
	PrintPlan(actions, &sb)
	out := sb.String()
	if !strings.Contains(out, "Hint:") {
		t.Errorf("expected 'Hint:' in output for unresolved packages, got:\n%s", out)
	}
	if !strings.Contains(out, "--strict") {
		t.Errorf("expected '--strict' mention in output, got:\n%s", out)
	}
}

// TestPrintPlan_ReturnsCorrectCounts verifies the resolved/unresolved return
// values for all combinations.
func TestPrintPlan_ReturnsCorrectCounts(t *testing.T) {
	tests := []struct {
		name           string
		pkgs           []schema.Package
		available      map[string]bool
		wantResolved   int
		wantUnresolved int
	}{
		{
			name:           "all resolved",
			pkgs:           []schema.Package{{ID: "git"}, {ID: "neovim"}},
			available:      map[string]bool{"brew": true},
			wantResolved:   2,
			wantUnresolved: 0,
		},
		{
			name:           "all unresolved",
			pkgs:           []schema.Package{{ID: "git"}, {ID: "neovim"}},
			available:      map[string]bool{},
			wantResolved:   0,
			wantUnresolved: 2,
		},
		{
			name: "mixed",
			pkgs: []schema.Package{
				{ID: "git"},
				{
					ID:       "only-snap",
					Managers: map[string]string{"snap": "io.pkg"},
					Prefer:   "snap",
				},
			},
			// brew available → git resolves; prefer=snap but snap absent → falls
			// back to brew for only-snap too (step 3 fallback), so both resolve.
			available:      map[string]bool{"brew": true},
			wantResolved:   2,
			wantUnresolved: 0,
		},
		{
			name:           "empty packages",
			pkgs:           nil,
			available:      map[string]bool{"brew": true},
			wantResolved:   0,
			wantUnresolved: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := &schema.GenvFile{Packages: tc.pkgs}
			actions := planOnGOOS(f, tc.available, "darwin")
			var sb strings.Builder
			resolved, unresolved := PrintPlan(actions, &sb)
			if resolved != tc.wantResolved {
				t.Errorf("resolved: got %d, want %d", resolved, tc.wantResolved)
			}
			if unresolved != tc.wantUnresolved {
				t.Errorf("unresolved: got %d, want %d", unresolved, tc.wantUnresolved)
			}
		})
	}
}

// TestPlan_EmptyPackages verifies that Plan with no packages returns an empty
// slice (not nil) and does not panic.
func TestPlan_EmptyPackages(t *testing.T) {
	f := &schema.GenvFile{Packages: []schema.Package{}}
	actions := Plan(f, map[string]bool{"brew": true})
	if len(actions) != 0 {
		t.Errorf("expected 0 actions, got %d", len(actions))
	}
}

// TestPlan_MultiplePackagesMixed verifies a file with several packages where
// some resolve and some don't.
func TestPlan_MultiplePackagesMixed(t *testing.T) {
	f := &schema.GenvFile{
		Packages: []schema.Package{
			{ID: "git"},
			{ID: "neovim", Prefer: "brew"},
			{ID: "secret-pkg", Prefer: "snap", Managers: map[string]string{"snap": "io.secret"}},
		},
	}
	// brew available, snap absent → git and neovim resolve; secret-pkg's
	// prefer is snap (unavailable) and its managers map has only snap
	// (unavailable), so it falls back to the generic fallback at step 3 (brew).
	available := map[string]bool{"brew": true}
	actions := planOnGOOS(f, available, "darwin")
	if len(actions) != 3 {
		t.Fatalf("expected 3 actions, got %d", len(actions))
	}
	for _, a := range actions {
		if !a.Resolved() {
			t.Errorf("expected all packages to resolve via brew fallback; %q is unresolved", a.Pkg.ID)
		}
	}
}

// TestNormalizeID verifies that each adapter uses the managers map when present
// and falls back to the package ID otherwise.
func TestNormalizeID(t *testing.T) {
	tests := []struct {
		name         string
		mgr          string
		id           string
		managers     map[string]string
		wantName     string
		wantExplicit bool
	}{
		{
			name:         "uses managers map",
			mgr:          "snap",
			id:           "hello",
			managers:     map[string]string{"snap": "hello"},
			wantName:     "hello",
			wantExplicit: true,
		},
		{
			name:         "falls back to id when no map entry",
			mgr:          "brew",
			id:           "firefox",
			managers:     nil,
			wantName:     "firefox",
			wantExplicit: false,
		},
		{
			name:         "falls back to id when manager not in map",
			mgr:          "brew",
			id:           "firefox",
			managers:     map[string]string{"snap": "hello"},
			wantName:     "firefox",
			wantExplicit: false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			a := adapter.ByName(tc.mgr)
			if a == nil {
				t.Fatalf("ByName(%q): no adapter found", tc.mgr)
			}
			gotName, gotExplicit := a.NormalizeID(tc.id, tc.managers)
			if gotName != tc.wantName {
				t.Errorf("NormalizeID name: got %q, want %q", gotName, tc.wantName)
			}
			if gotExplicit != tc.wantExplicit {
				t.Errorf("NormalizeID explicit: got %v, want %v", gotExplicit, tc.wantExplicit)
			}
		})
	}
}

// TestExecute_MultipleActions verifies that Execute runs all resolved actions
// and collects errors correctly.
func TestExecute_MultipleActions(t *testing.T) {
	actions := []Action{
		{
			Pkg:     schema.Package{ID: "pkg1"},
			Manager: "brew",
			PkgName: "pkg1",
			Cmd:     []string{"echo", "installing-pkg1"},
		},
		{
			Pkg:     schema.Package{ID: "pkg2"},
			Manager: "brew",
			PkgName: "pkg2",
			Cmd:     []string{"echo", "installing-pkg2"},
		},
	}
	var out, errOut strings.Builder
	errs := Execute(context.Background(), actions, nil, &out, &errOut)
	if len(errs) != 0 {
		t.Fatalf("Execute: unexpected errors: %v", errs)
	}
	if !strings.Contains(out.String(), "installing-pkg1") {
		t.Errorf("expected pkg1 output, got: %q", out.String())
	}
	if !strings.Contains(out.String(), "installing-pkg2") {
		t.Errorf("expected pkg2 output, got: %q", out.String())
	}
}

// TestExecute_MixedResolvedAndUnresolved verifies that Execute only runs
// resolved actions and returns errors only for resolved commands that fail.
func TestExecute_MixedResolvedAndUnresolved(t *testing.T) {
	actions := []Action{
		// Unresolved — must be skipped silently.
		{Pkg: schema.Package{ID: "mystery"}, Manager: "", Cmd: nil},
		// Resolved — runs echo successfully.
		{
			Pkg:     schema.Package{ID: "echo-pkg"},
			Manager: "brew",
			PkgName: "echo-pkg",
			Cmd:     []string{"echo", "ok"},
		},
		// Unresolved — also skipped.
		{Pkg: schema.Package{ID: "another-mystery"}, Manager: "", Cmd: nil},
	}
	var out, errOut strings.Builder
	errs := Execute(context.Background(), actions, nil, &out, &errOut)
	if len(errs) != 0 {
		t.Fatalf("Execute: unexpected errors: %v", errs)
	}
	if !strings.Contains(out.String(), "ok") {
		t.Errorf("expected 'ok' from echo command, got: %q", out.String())
	}
}

// TestPlan_PreferUnavailable_ManagersMapFallback verifies that when the
// preferred manager is unavailable but a valid entry exists in the managers map
// for a different available manager, it is used.
func TestPlan_PreferUnavailable_ManagersMapFallback(t *testing.T) {
	f := &schema.GenvFile{
		Packages: []schema.Package{
			{
				ID:     "hello",
				Prefer: "snap", // snap not available
				Managers: map[string]string{
					"snap": "hello",
					"brew": "hello",
				},
			},
		},
	}
	actions := Plan(f, map[string]bool{"brew": true})
	a := actions[0]
	if !a.Resolved() {
		t.Fatal("expected resolved via managers map fallback")
	}
	if a.Manager != "brew" {
		t.Errorf("manager: got %q, want %q", a.Manager, "brew")
	}
	if a.PkgName != "hello" {
		t.Errorf("pkgName: got %q, want %q", a.PkgName, "hello")
	}
}

// ---------------------------------------------------------------------------
// Reconcile regression tests — lock replay and version-constraint behavior
// ---------------------------------------------------------------------------

// TestReconcile_NewPackage_ToInstall verifies that a package in the spec but
// absent from the lock ends up in ToInstall.
func TestReconcile_NewPackage_ToInstall(t *testing.T) {
	desired := []schema.Package{{ID: "git"}}
	var managed []genvfile.LockedPackage // empty lock
	result := Reconcile(desired, managed, map[string]bool{"brew": true})
	if len(result.ToInstall) != 1 {
		t.Fatalf("ToInstall: got %d, want 1", len(result.ToInstall))
	}
	if result.ToInstall[0].Pkg.ID != "git" {
		t.Errorf("ToInstall[0].Pkg.ID = %q, want \"git\"", result.ToInstall[0].Pkg.ID)
	}
	if len(result.ToRemove) != 0 || len(result.Unchanged) != 0 {
		t.Errorf("unexpected ToRemove/Unchanged entries")
	}
}

func TestReconcileWith_NilLive_SameAsReconcile(t *testing.T) {
	desired := []schema.Package{{ID: "git"}}
	got := ReconcileWith(desired, nil, map[string]bool{"brew": true}, nil)
	want := Reconcile(desired, nil, map[string]bool{"brew": true})
	if len(got.ToInstall) != len(want.ToInstall) || len(got.Adopted) != 0 {
		t.Fatalf("nil live: ToInstall=%d Adopted=%d, want ToInstall=%d Adopted=0",
			len(got.ToInstall), len(got.Adopted), len(want.ToInstall))
	}
}

func TestReconcileWith_LiveInstalled_NotInLock_Adopts(t *testing.T) {
	desired := []schema.Package{{
		ID:       "cursor",
		Managers: map[string]string{"winget": "Anysphere.Cursor"},
	}}
	live := LiveSet{"winget": {"Anysphere.Cursor": true}}
	got := ReconcileWith(desired, nil, map[string]bool{"winget": true}, live)
	if len(got.ToInstall) != 0 {
		t.Fatalf("ToInstall=%d, want 0 (already installed)", len(got.ToInstall))
	}
	if len(got.Adopted) != 1 {
		t.Fatalf("Adopted=%d, want 1", len(got.Adopted))
	}
	if got.Adopted[0].ID != "cursor" || got.Adopted[0].Manager != "winget" || got.Adopted[0].PkgName != "Anysphere.Cursor" {
		t.Fatalf("Adopted[0]=%+v, want cursor/winget/Anysphere.Cursor", got.Adopted[0])
	}
}

func TestReconcileWith_LiveMissing_StillInstalls(t *testing.T) {
	desired := []schema.Package{{
		ID:       "syncthing",
		Managers: map[string]string{"winget": "Syncthing.Syncthing"},
	}}
	live := LiveSet{"winget": {"Anysphere.Cursor": true}}
	got := ReconcileWith(desired, nil, map[string]bool{"winget": true}, live)
	if len(got.ToInstall) != 1 || got.ToInstall[0].Pkg.ID != "syncthing" {
		t.Fatalf("ToInstall=%v, want syncthing", got.ToInstall)
	}
	if len(got.Adopted) != 0 {
		t.Fatalf("Adopted=%d, want 0", len(got.Adopted))
	}
}

func TestReconcileWith_LiveMatchIsCaseInsensitive(t *testing.T) {
	desired := []schema.Package{{
		ID:       "cursor",
		Managers: map[string]string{"winget": "Anysphere.Cursor"},
	}}
	live := LiveSet{"winget": {"anysphere.cursor": true}}
	got := ReconcileWith(desired, nil, map[string]bool{"winget": true}, live)
	if len(got.Adopted) != 1 {
		t.Fatalf("Adopted=%d, want 1 for case-insensitive live match", len(got.Adopted))
	}
}

func TestLoadLiveSet_NoManagers(t *testing.T) {
	got, warns := LoadLiveSet(nil)
	if len(got) != 0 || len(warns) != 0 {
		t.Fatalf("got %#v warns %v, want empty", got, warns)
	}
}

func TestLoadLiveSet_UnavailableManagerSkipped(t *testing.T) {
	got, _ := LoadLiveSet(map[string]bool{"not-a-manager": true})
	if len(got) != 0 {
		t.Fatalf("got %#v, want empty", got)
	}
}

func TestLoadLiveSet_FalseAvailabilitySkipped(t *testing.T) {
	got, warns := LoadLiveSet(map[string]bool{"brew": false})
	if len(got) != 0 || len(warns) != 0 {
		t.Fatalf("got %#v warns %v, want empty", got, warns)
	}
}

func TestPrintReconcilePlan_ShowsAdopted(t *testing.T) {
	var buf bytes.Buffer
	result := ReconcileResult{
		Adopted: []genvfile.LockedPackage{{ID: "cursor", Manager: "winget", PkgName: "Anysphere.Cursor"}},
	}
	_, _, _ = PrintReconcilePlan(result, &buf)
	out := buf.String()
	if !strings.Contains(out, "already installed") || !strings.Contains(out, "cursor") {
		t.Fatalf("plan = %q, want adopted cursor", out)
	}
	if !strings.Contains(out, "1 already installed") {
		t.Fatalf("plan = %q, want summary count", out)
	}
}

// TestReconcile_RemovedPackage_ToRemove verifies that a package in the lock
// but absent from the spec ends up in ToRemove.
func TestReconcile_RemovedPackage_ToRemove(t *testing.T) {
	var desired []schema.Package
	managed := []genvfile.LockedPackage{
		{ID: "htop", Manager: "brew", PkgName: "htop"},
	}
	result := Reconcile(desired, managed, map[string]bool{"brew": true})
	if len(result.ToRemove) != 1 {
		t.Fatalf("ToRemove: got %d, want 1", len(result.ToRemove))
	}
	if result.ToRemove[0].Pkg.ID != "htop" {
		t.Errorf("ToRemove[0].Pkg.ID = %q, want \"htop\"", result.ToRemove[0].Pkg.ID)
	}
}

// TestReconcile_Unchanged_NoVersion verifies that a package in both spec and
// lock with no version constraint stays Unchanged.
func TestReconcile_Unchanged_NoVersion(t *testing.T) {
	desired := []schema.Package{{ID: "git"}}
	managed := []genvfile.LockedPackage{
		{ID: "git", Manager: "brew", PkgName: "git", InstalledVersion: "2.43.0"},
	}
	result := Reconcile(desired, managed, map[string]bool{"brew": true})
	if len(result.Unchanged) != 1 {
		t.Fatalf("Unchanged: got %d, want 1", len(result.Unchanged))
	}
	if len(result.ToInstall) != 0 {
		t.Errorf("unexpected ToInstall: %v", result.ToInstall)
	}
}

// TestReconcile_VersionSatisfied_StaysUnchanged verifies that a lock entry
// whose InstalledVersion satisfies the spec constraint stays Unchanged.
func TestReconcile_VersionSatisfied_StaysUnchanged(t *testing.T) {
	desired := []schema.Package{{ID: "vim", Version: "9.*"}}
	managed := []genvfile.LockedPackage{
		{ID: "vim", Manager: "brew", PkgName: "vim", InstalledVersion: "9.1.0"},
	}
	result := Reconcile(desired, managed, map[string]bool{"brew": true})
	if len(result.Unchanged) != 1 {
		t.Fatalf("Unchanged: got %d, want 1 (version 9.1.0 satisfies 9.*)", len(result.Unchanged))
	}
	if len(result.ToInstall) != 0 {
		t.Errorf("unexpected reinstall queued for satisfying version")
	}
}

// TestReconcile_VersionDrift_MovesToInstall verifies that a lock entry whose
// InstalledVersion does not satisfy the spec constraint is queued for reinstall.
func TestReconcile_VersionDrift_MovesToInstall(t *testing.T) {
	desired := []schema.Package{{ID: "neovim", Version: "0.10.*"}}
	managed := []genvfile.LockedPackage{
		{ID: "neovim", Manager: "brew", PkgName: "neovim", InstalledVersion: "0.9.5"},
	}
	result := Reconcile(desired, managed, map[string]bool{"brew": true})
	if len(result.ToInstall) != 1 {
		t.Fatalf("ToInstall: got %d, want 1 (0.9.5 does not satisfy 0.10.*)", len(result.ToInstall))
	}
	if len(result.Unchanged) != 0 {
		t.Errorf("drifted package must not appear in Unchanged")
	}
}

// TestReconcile_NoInstalledVersion_AlwaysUnchanged verifies backward
// compatibility: old lock entries with empty InstalledVersion are never
// treated as drifted, even when the spec has a version constraint.
func TestReconcile_NoInstalledVersion_AlwaysUnchanged(t *testing.T) {
	desired := []schema.Package{{ID: "git", Version: "2.40.*"}}
	managed := []genvfile.LockedPackage{
		{ID: "git", Manager: "apt", PkgName: "git"}, // InstalledVersion == ""
	}
	result := Reconcile(desired, managed, map[string]bool{"brew": true})
	if len(result.Unchanged) != 1 {
		t.Fatalf("Unchanged: got %d, want 1 (old lock entries must not cause drift)", len(result.Unchanged))
	}
	if len(result.ToInstall) != 0 {
		t.Errorf("old lock entry with empty InstalledVersion must not be queued for reinstall")
	}
}

// TestReconcile_ExactVersionMatch_StaysUnchanged verifies an exact-version
// constraint is satisfied by an identical InstalledVersion.
func TestReconcile_ExactVersionMatch_StaysUnchanged(t *testing.T) {
	desired := []schema.Package{{ID: "ripgrep", Version: "14.1.0"}}
	managed := []genvfile.LockedPackage{
		{ID: "ripgrep", Manager: "brew", PkgName: "ripgrep", InstalledVersion: "14.1.0"},
	}
	result := Reconcile(desired, managed, map[string]bool{"brew": true})
	if len(result.Unchanged) != 1 {
		t.Fatalf("Unchanged: got %d, want 1", len(result.Unchanged))
	}
}

// ---------------------------------------------------------------------------
// PrintReconcilePlan — output and count verification
// ---------------------------------------------------------------------------

// TestPrintReconcilePlan_CountsAndOutput verifies that PrintReconcilePlan
// correctly counts installs, removals, and unchanged packages and writes
// expected markers to the output stream.
func TestPrintReconcilePlan_CountsAndOutput(t *testing.T) {
	result := ReconcileResult{
		ToInstall: []Action{
			{Pkg: schema.Package{ID: "neovim"}, Manager: "brew", PkgName: "neovim", Cmd: []string{"brew", "install", "neovim"}},
			{Pkg: schema.Package{ID: "mystery"}}, // unresolved
		},
		ToRemove: []Action{
			{Pkg: schema.Package{ID: "htop"}, Manager: "brew", PkgName: "htop", UninstallCmd: []string{"brew", "uninstall", "htop"}},
		},
		Unchanged: []genvfile.LockedPackage{
			{ID: "git", Manager: "brew"},
		},
	}
	var sb strings.Builder
	toInstall, toRemove, unresolved := PrintReconcilePlan(result, &sb)
	if toInstall != 2 {
		t.Errorf("toInstall: got %d, want 2", toInstall)
	}
	if toRemove != 1 {
		t.Errorf("toRemove: got %d, want 1", toRemove)
	}
	if unresolved != 1 {
		t.Errorf("unresolved: got %d, want 1", unresolved)
	}
	out := sb.String()
	if !strings.Contains(out, "+") {
		t.Error("expected '+' marker for installs")
	}
	if !strings.Contains(out, "-") {
		t.Error("expected '-' marker for removals")
	}
	if !strings.Contains(out, "git") {
		t.Error("expected unchanged package 'git' in output")
	}
	if !strings.Contains(out, "Hint:") {
		t.Error("expected 'Hint:' for unresolved packages")
	}
}

// TestPrintReconcilePlan_NothingToDo verifies that an empty reconcile result
// produces a "0 packages" header and returns zero counts.
func TestPrintReconcilePlan_NothingToDo(t *testing.T) {
	result := ReconcileResult{}
	var sb strings.Builder
	toInstall, toRemove, unresolved := PrintReconcilePlan(result, &sb)
	if toInstall != 0 || toRemove != 0 || unresolved != 0 {
		t.Errorf("expected all zeros, got install=%d remove=%d unresolved=%d", toInstall, toRemove, unresolved)
	}
	out := sb.String()
	if !strings.Contains(out, "0 packages") {
		t.Errorf("expected output to contain \"0 packages\", got %q", out)
	}
}

// TestPrintReconcilePlan_AllResolved verifies no "unresolved" hint is emitted
// when every package resolves.
func TestPrintReconcilePlan_AllResolved(t *testing.T) {
	result := ReconcileResult{
		ToInstall: []Action{
			{Pkg: schema.Package{ID: "git"}, Manager: "brew", PkgName: "git", Cmd: []string{"brew", "install", "git"}},
		},
	}
	var sb strings.Builder
	_, _, unresolved := PrintReconcilePlan(result, &sb)
	if unresolved != 0 {
		t.Errorf("unresolved: got %d, want 0", unresolved)
	}
	if strings.Contains(sb.String(), "Hint:") {
		t.Error("unexpected 'Hint:' when all packages resolve")
	}
}

// TestPrintReconcilePlan_SingularHeader verifies the "1 package" (not "1 packages")
// header is used when the total is exactly 1.
func TestPrintReconcilePlan_SingularHeader(t *testing.T) {
	result := ReconcileResult{
		ToInstall: []Action{
			{Pkg: schema.Package{ID: "git"}, Manager: "brew", PkgName: "git", Cmd: []string{"brew", "install", "git"}},
		},
	}
	var sb strings.Builder
	PrintReconcilePlan(result, &sb)
	out := sb.String()
	if !strings.Contains(out, "1 package") {
		t.Errorf("expected '1 package' (singular) in output:\n%s", out)
	}
	if strings.Contains(out, "1 packages") {
		t.Errorf("unexpected '1 packages' (plural) in output:\n%s", out)
	}
}

// ---------------------------------------------------------------------------
// ExecuteApply — successful apply, failed removal, failed install
// ---------------------------------------------------------------------------

// TestExecuteApply_SuccessfulInstall verifies that a successful install command
// populates Installed and produces no errors.
func TestExecuteApply_SuccessfulInstall(t *testing.T) {
	result := ReconcileResult{
		ToInstall: []Action{
			{
				Pkg:     schema.Package{ID: "echo-pkg"},
				Manager: "brew",
				PkgName: "echo-pkg",
				Cmd:     []string{"echo", "installing"},
			},
		},
	}
	var out, errOut bytes.Buffer
	exec := ExecuteApply(context.Background(), result, nil, &out, &errOut)
	if len(exec.Errors) != 0 {
		t.Fatalf("ExecuteApply: unexpected errors: %v", exec.Errors)
	}
	if len(exec.Installed) != 1 {
		t.Fatalf("Installed: got %d, want 1", len(exec.Installed))
	}
	if exec.Installed[0].ID != "echo-pkg" {
		t.Errorf("Installed[0].ID: got %q, want \"echo-pkg\"", exec.Installed[0].ID)
	}
}

// TestExecuteApply_FailedInstall verifies that a failed install command
// results in an error and no Installed entry.
func TestExecuteApply_FailedInstall(t *testing.T) {
	result := ReconcileResult{
		ToInstall: []Action{
			{
				Pkg:     schema.Package{ID: "fail-pkg"},
				Manager: "brew",
				PkgName: "fail-pkg",
				Cmd:     []string{"false"},
			},
		},
	}
	var out, errOut bytes.Buffer
	exec := ExecuteApply(context.Background(), result, nil, &out, &errOut)
	if len(exec.Errors) == 0 {
		t.Error("expected error for failing install command")
	}
	if len(exec.Installed) != 0 {
		t.Errorf("Installed: got %d, want 0 (failed install must not appear in Installed)", len(exec.Installed))
	}
}

// TestExecuteApply_SuccessfulRemoval verifies that a successful uninstall command
// populates Uninstalled and produces no errors. We use "snap" as the manager
// because its PlanClean returns nil (no cache-clean invocations that would fail
// when snap is not installed on the test host).
func TestExecuteApply_SuccessfulRemoval(t *testing.T) {
	result := ReconcileResult{
		ToRemove: []Action{
			{
				Pkg:          schema.Package{ID: "old-pkg"},
				Manager:      "snap",
				PkgName:      "old-pkg",
				UninstallCmd: []string{"echo", "removing"},
			},
		},
	}
	var out, errOut bytes.Buffer
	exec := ExecuteApply(context.Background(), result, nil, &out, &errOut)
	if len(exec.Errors) != 0 {
		t.Fatalf("ExecuteApply: unexpected errors: %v", exec.Errors)
	}
	if len(exec.Uninstalled) != 1 || exec.Uninstalled[0] != "old-pkg" {
		t.Errorf("Uninstalled: got %v, want [\"old-pkg\"]", exec.Uninstalled)
	}
}

// TestExecuteApply_FailedRemoval verifies that a failed removal produces an
// error and the package is NOT in Uninstalled.
func TestExecuteApply_FailedRemoval(t *testing.T) {
	result := ReconcileResult{
		ToRemove: []Action{
			{
				Pkg:          schema.Package{ID: "stuck-pkg"},
				Manager:      "snap",
				PkgName:      "stuck-pkg",
				UninstallCmd: []string{"false"},
			},
		},
	}
	var out, errOut bytes.Buffer
	exec := ExecuteApply(context.Background(), result, nil, &out, &errOut)
	if len(exec.Errors) == 0 {
		t.Error("expected error for failing removal")
	}
	if len(exec.Uninstalled) != 0 {
		t.Errorf("Uninstalled: got %v, want empty (failed removal must not appear)", exec.Uninstalled)
	}
}

// TestExecuteApply_SkipsUnresolvedInstall verifies that unresolved install
// actions are silently skipped.
func TestExecuteApply_SkipsUnresolvedInstall(t *testing.T) {
	result := ReconcileResult{
		ToInstall: []Action{
			{Pkg: schema.Package{ID: "mystery"}, Manager: "", Cmd: nil}, // unresolved
		},
	}
	var out, errOut bytes.Buffer
	exec := ExecuteApply(context.Background(), result, nil, &out, &errOut)
	if len(exec.Errors) != 0 {
		t.Errorf("unexpected errors for unresolved install: %v", exec.Errors)
	}
	if len(exec.Installed) != 0 {
		t.Errorf("Installed: got %d, want 0 (unresolved must be skipped)", len(exec.Installed))
	}
}

// TestResolveOne verifies that ResolveOne resolves a single package correctly.
func TestResolveOne(t *testing.T) {
	pkg := schema.Package{ID: "git"}
	action := resolveOnGOOS(pkg, map[string]bool{"brew": true}, "darwin")
	if !action.Resolved() {
		t.Fatal("ResolveOne: expected resolved action")
	}
	if action.Manager != "brew" {
		t.Errorf("Manager: got %q, want \"brew\"", action.Manager)
	}
	if action.PkgName != "git" {
		t.Errorf("PkgName: got %q, want \"git\"", action.PkgName)
	}
}

func TestResolveOne_DefaultFallbackSkipsEcosystemManagers(t *testing.T) {
	pkg := schema.Package{ID: "git"}
	action := ResolveOne(pkg, map[string]bool{"npm": true, "cargo": true, "vscode": true})
	if action.Resolved() {
		t.Fatalf("ResolveOne resolved via %q; ecosystem managers must be explicit-only fallback targets", action.Manager)
	}
}

func TestResolveOne_DefaultFallbackStillUsesSystemManagers(t *testing.T) {
	pkg := schema.Package{ID: "git"}
	action := ResolveOne(pkg, map[string]bool{"npm": true, "paru": true})
	if !action.Resolved() {
		t.Fatal("ResolveOne: expected resolved action")
	}
	if action.Manager != "paru" {
		t.Errorf("Manager: got %q, want %q", action.Manager, "paru")
	}
}

func TestResolveOneOnGOOS_PlatformFallbackUsesNativeHomebrew(t *testing.T) {
	tests := []struct {
		name string
		goos string
		want string
	}{
		{name: "Darwin uses Homebrew", goos: "darwin", want: "brew"},
		{name: "Linux uses Linuxbrew", goos: "linux", want: "linuxbrew"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			action := resolveOnGOOS(
				schema.Package{ID: "git"},
				map[string]bool{"brew": true, "linuxbrew": true},
				tt.goos,
			)

			if action.Manager != tt.want {
				t.Errorf("manager = %q, want %q", action.Manager, tt.want)
			}
		})
	}
}

func TestResolveOneOnGOOS_PlatformExplicitHomebrewSelectionRemainsAuthoritative(t *testing.T) {
	tests := []struct {
		name string
		goos string
		pkg  schema.Package
		want string
	}{
		{
			name: "Darwin prefer can select Linuxbrew",
			goos: "darwin",
			pkg:  schema.Package{ID: "git", Prefer: "linuxbrew"},
			want: "linuxbrew",
		},
		{
			name: "Linux managers map can select Homebrew",
			goos: "linux",
			pkg:  schema.Package{ID: "git", Managers: map[string]string{"brew": "git"}},
			want: "brew",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			action := resolveOnGOOS(tt.pkg, map[string]bool{"brew": true, "linuxbrew": true}, tt.goos)

			if action.Manager != tt.want {
				t.Errorf("manager = %q, want %q", action.Manager, tt.want)
			}
		})
	}
}

func TestResolveOne_PreferCanSelectEcosystemManager(t *testing.T) {
	pkg := schema.Package{ID: "typescript", Prefer: "npm"}
	action := ResolveOne(pkg, map[string]bool{"npm": true})
	if !action.Resolved() {
		t.Fatal("ResolveOne: expected resolved action")
	}
	if action.Manager != "npm" {
		t.Errorf("Manager: got %q, want %q", action.Manager, "npm")
	}
}

func TestResolveOne_ManagersMapCanSelectEcosystemManager(t *testing.T) {
	pkg := schema.Package{ID: "kubectx", Managers: map[string]string{"krew": "ctx"}}
	action := ResolveOne(pkg, map[string]bool{"krew": true})
	if !action.Resolved() {
		t.Fatal("ResolveOne: expected resolved action")
	}
	if action.Manager != "krew" {
		t.Errorf("Manager: got %q, want %q", action.Manager, "krew")
	}
	if action.PkgName != "ctx" {
		t.Errorf("PkgName: got %q, want %q", action.PkgName, "ctx")
	}
}

// TestResolveOne_Unresolved verifies that ResolveOne returns an unresolved
// action when no manager is available.
func TestResolveOne_Unresolved(t *testing.T) {
	pkg := schema.Package{ID: "git"}
	action := ResolveOne(pkg, map[string]bool{})
	if action.Resolved() {
		t.Error("ResolveOne: expected unresolved action with empty available map")
	}
}

// ---------------------------------------------------------------------------
// Benchmarks — cold-start budget enforcement
// ---------------------------------------------------------------------------

// BenchmarkDetect measures the cost of discovering available package managers.
// This exercises PATH lookups for all registered adapters and must stay
// well under the 200ms cold-start budget enforced in CI.
func BenchmarkDetect(b *testing.B) {
	for b.Loop() {
		_ = Detect()
	}
}

// BenchmarkResolve measures the cost of resolving a realistic set of packages
// against a fixed available-manager map. This is pure computation (no I/O).
func BenchmarkResolve(b *testing.B) {
	f := &schema.GenvFile{
		Packages: []schema.Package{
			{ID: "git"},
			{ID: "neovim", Prefer: "brew"},
			{ID: "hello", Managers: map[string]string{"snap": "hello", "brew": "hello"}},
			{ID: "ripgrep"},
			{ID: "tmux"},
		},
	}
	available := map[string]bool{"brew": true, "paru": true, "snap": true}
	b.ResetTimer()
	for b.Loop() {
		_ = Plan(f, available)
	}
}

// BenchmarkReconcile measures the cost of computing the delta between desired
// and managed state — the core of `genv apply`.
func BenchmarkReconcile(b *testing.B) {
	desired := []schema.Package{
		{ID: "git"},
		{ID: "neovim"},
		{ID: "ripgrep"},
	}
	managed := []genvfile.LockedPackage{
		{ID: "git", Manager: "brew", PkgName: "git", InstalledVersion: "2.43.0"},
		{ID: "htop", Manager: "brew", PkgName: "htop"},
	}
	available := map[string]bool{"brew": true}
	b.ResetTimer()
	for b.Loop() {
		_ = Reconcile(desired, managed, available)
	}
}

// TestReconcile_ExactVersionMismatch_MovesToInstall verifies that an exact
// constraint with a different installed version is treated as drift.
func TestReconcile_ExactVersionMismatch_MovesToInstall(t *testing.T) {
	desired := []schema.Package{{ID: "ripgrep", Version: "14.1.0"}}
	managed := []genvfile.LockedPackage{
		{ID: "ripgrep", Manager: "brew", PkgName: "ripgrep", InstalledVersion: "13.0.0"},
	}
	result := Reconcile(desired, managed, map[string]bool{"brew": true})
	if len(result.ToInstall) != 1 {
		t.Fatalf("ToInstall: got %d, want 1", len(result.ToInstall))
	}
}
