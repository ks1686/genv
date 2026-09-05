package upgrade

import (
	"context"
	"testing"

	"github.com/ks1686/genv/internal/genvfile"
	"github.com/ks1686/genv/internal/schema"
	"github.com/ks1686/genv/internal/testutil"
)

func TestTrackedStep_commands_match_existing_planner(t *testing.T) {
	testutil.InstallFakeBinary(t, "brew", "exit 0")
	spec := &schema.GenvFile{Packages: []schema.Package{{ID: "git"}}}
	lock := &genvfile.LockFile{Packages: []genvfile.LockedPackage{
		{ID: "git", Manager: "brew", PkgName: "git", InstalledVersion: "1.0.0"},
	}}
	plan, err := BuildUpgradePlan(UpgradeOptions{Spec: spec, Lock: lock, Filters: planAll})
	if err != nil {
		t.Fatalf("BuildUpgradePlan: %v", err)
	}
	if len(plan.Actions) != 1 {
		t.Fatalf("actions = %d, want 1", len(plan.Actions))
	}

	var applyCalls int
	step := TrackedStep(plan, func(ctx context.Context) error {
		applyCalls++
		return nil
	})
	if step.Name != "tracked" {
		t.Fatalf("Name = %q, want tracked", step.Name)
	}
	if len(step.Commands) != 1 {
		t.Fatalf("Commands = %v, want the planned brew upgrade argv", step.Commands)
	}
	if !equalCmd(step.Commands[0], plan.Actions[0].Cmd) {
		t.Fatalf("Commands[0] = %v, want %v", step.Commands[0], plan.Actions[0].Cmd)
	}

	got := Run(context.Background(), ModeApply, []Step{step}, func(ctx context.Context, cmd []string) error {
		t.Fatalf("generic exec ran %v", cmd)
		return nil
	})
	if applyCalls != 1 {
		t.Fatalf("Apply calls = %d, want 1", applyCalls)
	}
	if len(got) != 1 || got[0].Status != StatusRan {
		t.Fatalf("outcome = %#v", got)
	}
}

func TestTrackedStep_empty_plan_is_still_named_tracked(t *testing.T) {
	step := TrackedStep(UpgradePlan{}, nil)
	if step.Name != "tracked" || len(step.Commands) != 0 || step.SkipReason != "" {
		t.Fatalf("empty tracked step = %#v", step)
	}
}

func equalCmd(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
