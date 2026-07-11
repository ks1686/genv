package upgrade

import (
	"testing"

	"github.com/ks1686/genv/internal/genvfile"
	"github.com/ks1686/genv/internal/output"
	"github.com/ks1686/genv/internal/schema"
)

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
			filters:     output.UpgradeFilters{},
			wantActions: 2, // brew (batch), npm
			wantSkipped: 0,
		},
		{
			name: "only pkg1",
			filters: output.UpgradeFilters{
				Only: []string{"pkg1"},
			},
			wantActions: 1,
			wantSkipped: 2,
		},
		{
			name: "skip pkg3",
			filters: output.UpgradeFilters{
				Skip: []string{"pkg3"},
			},
			wantActions: 1, // brew batch
			wantSkipped: 1,
		},
		{
			name: "only manager brew",
			filters: output.UpgradeFilters{
				OnlyManager: []string{"brew"},
			},
			wantActions: 1,
			wantSkipped: 1,
		},
		{
			name: "skip manager brew",
			filters: output.UpgradeFilters{
				SkipManager: []string{"brew"},
			},
			wantActions: 1,
			wantSkipped: 2,
		},
		{
			name: "unknown manager",
			filters: output.UpgradeFilters{
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
	plan, err := BuildUpgradePlan(UpgradeOptions{Spec: spec, Lock: lock})

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
