package upgrade

import (
	"context"
	"errors"
	"testing"
)

func TestRun_skips_absent_step_without_executing(t *testing.T) {
	var ran []string
	exec := func(ctx context.Context, cmd []string) error {
		ran = append(ran, cmd[0])
		return nil
	}

	got := Run(context.Background(), ModeApply, []Step{{
		Name:       "firmware",
		SkipReason: "fwupdmgr not found",
		Commands:   [][]string{{"sudo", "fwupdmgr", "update"}},
	}}, exec)

	if len(got) != 1 {
		t.Fatalf("outcomes = %d, want 1", len(got))
	}
	if got[0].Status != StatusSkipped {
		t.Fatalf("status = %q, want %q", got[0].Status, StatusSkipped)
	}
	if got[0].Reason != "fwupdmgr not found" {
		t.Fatalf("reason = %q", got[0].Reason)
	}
	if len(ran) != 0 {
		t.Fatalf("exec ran %v, want none", ran)
	}
}

func TestRun_plan_lists_commands_without_executing(t *testing.T) {
	exec := func(ctx context.Context, cmd []string) error {
		t.Fatalf("plan mode executed %v", cmd)
		return nil
	}

	got := Run(context.Background(), ModePlan, []Step{{
		Name:     "system",
		Commands: [][]string{{"sudo", "pacman", "-Syu", "--noconfirm"}},
	}}, exec)

	if len(got) != 1 || got[0].Status != StatusPlanned {
		t.Fatalf("outcome = %#v, want planned system step", got)
	}
	if len(got[0].Commands) != 1 {
		t.Fatalf("commands = %v", got[0].Commands)
	}
}

func TestRun_apply_continues_after_step_failure(t *testing.T) {
	var ran []string
	exec := func(ctx context.Context, cmd []string) error {
		ran = append(ran, cmd[0])
		if cmd[0] == "fail" {
			return errors.New("boom")
		}
		return nil
	}

	got := Run(context.Background(), ModeApply, []Step{
		{Name: "system", Commands: [][]string{{"fail", "os"}}},
		{Name: "firmware", Commands: [][]string{{"ok", "fw"}}},
	}, exec)

	if len(got) != 2 {
		t.Fatalf("outcomes = %d, want 2", len(got))
	}
	if got[0].Status != StatusFailed || got[0].Err == nil {
		t.Fatalf("first step = %#v, want failed with error", got[0])
	}
	if got[1].Status != StatusRan {
		t.Fatalf("second step status = %q, want %q", got[1].Status, StatusRan)
	}
	if len(ran) != 2 || ran[0] != "fail" || ran[1] != "ok" {
		t.Fatalf("exec order = %v, want [fail ok]", ran)
	}
}

func TestRun_custom_apply_used_instead_of_exec(t *testing.T) {
	var execCalled bool
	exec := func(ctx context.Context, cmd []string) error {
		execCalled = true
		return nil
	}
	var applyCalled bool
	got := Run(context.Background(), ModeApply, []Step{{
		Name:     "tracked",
		Commands: [][]string{{"brew", "upgrade", "git"}},
		Apply: func(ctx context.Context) error {
			applyCalled = true
			return nil
		},
	}}, exec)

	if !applyCalled {
		t.Fatal("expected custom Apply to run")
	}
	if execCalled {
		t.Fatal("generic exec should not run when Apply is set")
	}
	if len(got) != 1 || got[0].Status != StatusRan {
		t.Fatalf("outcome = %#v, want ran", got)
	}
}
