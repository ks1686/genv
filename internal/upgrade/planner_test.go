package upgrade

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/ks1686/genv/internal/genvfile"
	"github.com/ks1686/genv/internal/output"
	"github.com/ks1686/genv/internal/resolver"
	"github.com/ks1686/genv/internal/schema"
	"github.com/ks1686/genv/internal/testutil"
)

// planAll skips outdated detection so planner unit tests exercise filter/constraint
// logic without requiring fake outdated CLIs.
var planAll = output.UpgradeFilters{All: true}

func TestBuildUpgradePlan_FiltersAll_skipsOutdatedDetection(t *testing.T) {
	// Given: brew outdated reports only git, but --all requests every package.
	dir := t.TempDir()
	script := "#!/bin/sh\n" +
		`if [ "$1" = "outdated" ]; then echo '{"formulae":[{"name":"git","current_version":"2.44.0"}],"casks":[]}'; exit 0; fi` + "\n"
	if err := os.WriteFile(filepath.Join(dir, "brew"), []byte(script), 0o755); err != nil {
		t.Fatalf("write fake brew: %v", err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	spec := &schema.GenvFile{Packages: []schema.Package{{ID: "git"}, {ID: "jq"}}}
	lock := &genvfile.LockFile{Packages: []genvfile.LockedPackage{
		{ID: "git", Manager: "brew", PkgName: "git"},
		{ID: "jq", Manager: "brew", PkgName: "jq"},
	}}

	plan, err := BuildUpgradePlan(UpgradeOptions{
		Spec:    spec,
		Lock:    lock,
		Filters: output.UpgradeFilters{All: true},
	})
	if err != nil {
		t.Fatalf("BuildUpgradePlan: %v", err)
	}
	var gotIDs []string
	for _, a := range plan.Actions {
		for _, lp := range a.LPs {
			gotIDs = append(gotIDs, lp.ID)
		}
	}
	if !slices.Equal(gotIDs, []string{"git", "jq"}) {
		t.Fatalf("Filters.All plan packages = %v, want [git jq]", gotIDs)
	}
}

func TestBuildUpgradePlan_DetectOutdated_filters_to_outdated(t *testing.T) {
	// Given: two tracked brew packages, but a fake `brew outdated` reports only git.
	testutil.InstallFakeBinary(t, "brew",
		`if [ "$1" = "outdated" ]; then echo '{"formulae":[{"name":"git","current_version":"2.44.0"}],"casks":[]}'; exit 0; fi`)

	spec := &schema.GenvFile{Packages: []schema.Package{{ID: "git"}, {ID: "jq"}}}
	lock := &genvfile.LockFile{Packages: []genvfile.LockedPackage{
		{ID: "git", Manager: "brew", PkgName: "git"},
		{ID: "jq", Manager: "brew", PkgName: "jq"},
	}}

	// When: the plan is built with default outdated detection (Filters.All false).
	plan, err := BuildUpgradePlan(UpgradeOptions{Spec: spec, Lock: lock})
	if err != nil {
		t.Fatalf("BuildUpgradePlan: %v", err)
	}

	// Then: only the outdated package remains in the plan.
	var gotIDs []string
	for _, a := range plan.Actions {
		for _, lp := range a.LPs {
			gotIDs = append(gotIDs, lp.ID)
		}
	}
	if !slices.Equal(gotIDs, []string{"git"}) {
		t.Fatalf("outdated plan packages = %v, want [git]", gotIDs)
	}
}

func TestBuildUpgradePlan_Constraint_unconstrained_package_plans_normally(t *testing.T) {
	// Given: an unconstrained package tracked by a registered manager.
	spec := &schema.GenvFile{Packages: []schema.Package{{ID: "git"}}}
	lock := &genvfile.LockFile{Packages: []genvfile.LockedPackage{{ID: "git", Manager: "brew", PkgName: "git"}}}

	// When: the shared upgrade plan is built.
	plan, err := BuildUpgradePlan(UpgradeOptions{Spec: spec, Lock: lock, Filters: planAll})

	// Then: the package retains the existing upgrade behavior.
	if err != nil {
		t.Fatalf("BuildUpgradePlan: %v", err)
	}
	if len(plan.Actions) != 1 || !slices.Equal([]string{plan.Actions[0].LPs[0].ID}, []string{"git"}) {
		t.Fatalf("actions = %#v, want one action for git", plan.Actions)
	}
	if len(plan.Skipped) != 0 {
		t.Fatalf("skipped = %#v, want none", plan.Skipped)
	}
}

func TestBuildUpgradePlan_Constraint_constrained_packages_are_skipped_in_lock_order(t *testing.T) {
	// Given: two constrained packages separated by an unconstrained package in lock order.
	spec := &schema.GenvFile{Packages: []schema.Package{
		{ID: "ripgrep", Version: "14.1.0"},
		{ID: "git"},
		{ID: "jq", Version: "1.7.x"},
	}}
	lock := &genvfile.LockFile{Packages: []genvfile.LockedPackage{
		{ID: "jq", Manager: "brew", PkgName: "jq"},
		{ID: "git", Manager: "brew", PkgName: "git"},
		{ID: "ripgrep", Manager: "brew", PkgName: "ripgrep"},
	}}

	// When: the shared upgrade plan is built.
	plan, err := BuildUpgradePlan(UpgradeOptions{Spec: spec, Lock: lock, Filters: planAll})

	// Then: only the unconstrained package is planned and constraint skips retain lock order and identity.
	if err != nil {
		t.Fatalf("BuildUpgradePlan: %v", err)
	}
	if len(plan.Actions) != 1 || len(plan.Actions[0].LPs) != 1 || plan.Actions[0].LPs[0].ID != "git" {
		t.Fatalf("actions = %#v, want one action for git", plan.Actions)
	}
	wantSkipped := []struct {
		id      string
		manager string
	}{
		{id: "jq", manager: "brew"},
		{id: "ripgrep", manager: "brew"},
	}
	if len(plan.Skipped) != len(wantSkipped) {
		t.Fatalf("skipped = %#v, want %d constraint skips", plan.Skipped, len(wantSkipped))
	}
	for i, want := range wantSkipped {
		got := plan.Skipped[i]
		if got.ID != want.id || got.Manager != want.manager || got.Reason != "version-constrained package requires an explicit compatible target" {
			t.Errorf("skipped[%d] = %#v, want ID %q, manager %q, stable constraint reason", i, got, want.id, want.manager)
		}
	}
}

func TestBuildUpgradePlan_Constraint_mixed_batch_excludes_only_constrained_packages(t *testing.T) {
	// Given: a batch-capable manager tracks constrained and unconstrained packages together.
	spec := &schema.GenvFile{Packages: []schema.Package{
		{ID: "git"},
		{ID: "jq", Version: "1.7.1"},
		{ID: "ripgrep"},
	}}
	lock := &genvfile.LockFile{Packages: []genvfile.LockedPackage{
		{ID: "git", Manager: "brew", PkgName: "git"},
		{ID: "jq", Manager: "brew", PkgName: "jq"},
		{ID: "ripgrep", Manager: "brew", PkgName: "ripgrep"},
	}}

	// When: the shared upgrade plan is built.
	plan, err := BuildUpgradePlan(UpgradeOptions{Spec: spec, Lock: lock, Filters: planAll})

	// Then: the remaining unconstrained packages still form one batch.
	if err != nil {
		t.Fatalf("BuildUpgradePlan: %v", err)
	}
	if len(plan.Actions) != 1 {
		t.Fatalf("actions = %#v, want one brew batch", plan.Actions)
	}
	gotIDs := make([]string, len(plan.Actions[0].LPs))
	for i, lp := range plan.Actions[0].LPs {
		gotIDs[i] = lp.ID
	}
	if !slices.Equal(gotIDs, []string{"git", "ripgrep"}) {
		t.Fatalf("batch IDs = %v, want [git ripgrep]", gotIDs)
	}
	if len(plan.Skipped) != 1 || plan.Skipped[0].ID != "jq" {
		t.Fatalf("skipped = %#v, want only jq", plan.Skipped)
	}
}

func TestBuildUpgradePlan_Constraint_existing_package_filters_run_first(t *testing.T) {
	// Given: a constrained package selected by overlapping package filters.
	spec := &schema.GenvFile{Packages: []schema.Package{{ID: "jq", Version: "1.7.1"}}}
	lock := &genvfile.LockFile{Packages: []genvfile.LockedPackage{{ID: "jq", Manager: "brew", PkgName: "jq"}}}
	tests := []struct {
		name       string
		filters    output.UpgradeFilters
		wantReason string
	}{
		{
			name:       "skip excludes before constraint evaluation",
			filters:    output.UpgradeFilters{All: true, Skip: []string{"jq"}},
			wantReason: "excluded by --skip",
		},
		{
			name:       "only takes precedence over skip before constraint evaluation",
			filters:    output.UpgradeFilters{All: true, Only: []string{"jq"}, Skip: []string{"jq"}},
			wantReason: "version-constrained package requires an explicit compatible target",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// When: the shared upgrade plan is built with the overlapping filters.
			plan, err := BuildUpgradePlan(UpgradeOptions{Spec: spec, Lock: lock, Filters: tt.filters})

			// Then: existing filter precedence determines which skip reason is reported.
			if err != nil {
				t.Fatalf("BuildUpgradePlan: %v", err)
			}
			if len(plan.Actions) != 0 {
				t.Fatalf("actions = %#v, want none", plan.Actions)
			}
			if len(plan.Skipped) != 1 || plan.Skipped[0].Reason != tt.wantReason {
				t.Fatalf("skipped = %#v, want reason %q", plan.Skipped, tt.wantReason)
			}
		})
	}
}

func TestBuildPlan(t *testing.T) {
	spec := &schema.GenvFile{
		Packages: []schema.Package{
			{ID: "pkg1"},
			{ID: "pkg2"},
			{ID: "pkg3"},
		},
	}
	lock := &genvfile.LockFile{
		Packages: []genvfile.LockedPackage{
			{ID: "pkg1", Manager: "brew", PkgName: "pkg1"},
			{ID: "pkg2", Manager: "brew", PkgName: "pkg2"},
			{ID: "pkg3", Manager: "bun", PkgName: "pkg3"},
		},
	}

	tests := []struct {
		name        string
		filters     output.UpgradeFilters
		wantActions int
		wantSkipped int
		wantErr     bool
	}{
		{
			name:        "no filters",
			filters:     output.UpgradeFilters{All: true},
			wantActions: 2, // brew (batch), npm
			wantSkipped: 0,
		},
		{
			name: "only pkg1",
			filters: output.UpgradeFilters{
				All:  true,
				Only: []string{"pkg1"},
			},
			wantActions: 1,
			wantSkipped: 2,
		},
		{
			name: "skip pkg3",
			filters: output.UpgradeFilters{
				All:  true,
				Skip: []string{"pkg3"},
			},
			wantActions: 1, // brew batch
			wantSkipped: 1,
		},
		{
			name: "only manager brew",
			filters: output.UpgradeFilters{
				All:         true,
				OnlyManager: []string{"brew"},
			},
			wantActions: 1,
			wantSkipped: 1,
		},
		{
			name: "skip manager brew",
			filters: output.UpgradeFilters{
				All:         true,
				SkipManager: []string{"brew"},
			},
			wantActions: 1,
			wantSkipped: 2,
		},
		{
			name: "unknown manager",
			filters: output.UpgradeFilters{
				All:         true,
				OnlyManager: []string{"unknown"},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := Options{
				Spec:    spec,
				Lock:    lock,
				Filters: tt.filters,
			}
			plan, err := BuildPlan(opts)
			if (err != nil) != tt.wantErr {
				t.Errorf("BuildPlan() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil {
				return
			}
			if len(plan.Actions) != tt.wantActions {
				t.Errorf("BuildPlan() actions = %v, want %v", len(plan.Actions), tt.wantActions)
			}
			if len(plan.Skipped) != tt.wantSkipped {
				t.Errorf("BuildPlan() skipped = %v, want %v", len(plan.Skipped), tt.wantSkipped)
			}
		})
	}
}

func TestBuildUpgradePlan_is_callable_without_cli_side_effects(t *testing.T) {
	// Given: a spec and lock that can be planned without file paths, hooks, or command IO.
	spec := &schema.GenvFile{Packages: []schema.Package{{ID: "git"}}}
	lock := &genvfile.LockFile{Packages: []genvfile.LockedPackage{{ID: "git", Manager: "brew", PkgName: "git", InstalledVersion: "1.0.0"}}}

	// When: the reusable planner is called directly, outside main.go flag parsing.
	plan, err := BuildUpgradePlan(UpgradeOptions{Spec: spec, Lock: lock, Filters: planAll})

	// Then: planning produces a reusable UpgradePlan and leaves execution results absent.
	if err != nil {
		t.Fatalf("BuildUpgradePlan: %v", err)
	}
	if len(plan.Actions) != 1 {
		t.Fatalf("plan actions = %d, want 1", len(plan.Actions))
	}
	result := UpgradeRunResult{Plan: plan}
	if len(result.Upgraded) != 0 || len(result.Errors) != 0 {
		t.Fatalf("empty run result = %#v, want no subprocess outcomes", result)
	}
	if lock.Packages[0].InstalledVersion != "1.0.0" {
		t.Fatalf("planner mutated lock version to %q", lock.Packages[0].InstalledVersion)
	}
}

func TestRunUpgrade_propagates_typed_action_failures(t *testing.T) {
	// Given: one synthetic failed upgrade action.
	mgr := upgradeFailureTestAdapter{}
	plan := UpgradePlan{Actions: []resolver.UpgradeAction{{
		LPs: []genvfile.LockedPackage{{ID: "alpha", Manager: "brew", PkgName: "alpha"}},
		Mgr: mgr,
		Cmd: []string{"false"},
	}}}

	// When: the shared upgrade executor runs it.
	result := RunUpgrade(context.Background(), UpgradeRunOptions{
		Plan:   plan,
		Lock:   &genvfile.LockFile{},
		Stdout: &bytes.Buffer{},
		Stderr: &bytes.Buffer{},
	})

	// Then: callers receive both the existing error slice and structured action identity.
	if len(result.Errors) != 1 {
		t.Fatalf("Errors = %v, want one", result.Errors)
	}
	if len(result.Failures) != 1 || !slices.Equal(result.Failures[0].IDs, []string{"alpha"}) {
		t.Fatalf("Failures = %#v, want alpha action failure", result.Failures)
	}
}

type upgradeFailureTestAdapter struct{}

func (upgradeFailureTestAdapter) Name() string { return "test-failure" }
func (upgradeFailureTestAdapter) Available() bool {
	return true
}
func (upgradeFailureTestAdapter) NormalizeID(id string, managers map[string]string) (string, bool) {
	return id, false
}
func (upgradeFailureTestAdapter) PlanInstall(pkgName string) []string   { return nil }
func (upgradeFailureTestAdapter) PlanUninstall(pkgName string) []string { return nil }
func (upgradeFailureTestAdapter) PlanUpgrade(pkgName string) []string   { return nil }
func (upgradeFailureTestAdapter) PlanClean() [][]string                 { return nil }
func (upgradeFailureTestAdapter) Query(pkgName string) (bool, error)    { return true, nil }
func (upgradeFailureTestAdapter) ListInstalled() ([]string, error)      { return nil, nil }
func (upgradeFailureTestAdapter) QueryVersion(pkgName string) (string, error) {
	return "", nil
}
