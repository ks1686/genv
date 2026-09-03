package main

import (
	"context"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/ks1686/genv/internal/host"
	"github.com/ks1686/genv/internal/output"
	"github.com/ks1686/genv/internal/resolver"
	"github.com/ks1686/genv/internal/upgrade"
)

type extraUpgradeJSON struct {
	steps []output.UpgradeStep
	apply func(ctx context.Context) []output.UpgradeStep
}

// upgradeLookPath is exec.LookPath for OS/firmware tool detection. Tests
// replace it so CLI upgrade --yes never runs softwareupdate, pacman -Syu,
// apt-get upgrade, fwupdmgr, or Windows Update COM.
var upgradeLookPath = exec.LookPath

func upgradeRunnerEnv(targetID string) upgrade.Env {
	if targetID == "" {
		if classified, err := host.Classify(); err == nil {
			targetID = classified
		}
	}
	return upgrade.Env{Target: targetID, GOOS: runtime.GOOS, LookPath: upgradeLookPath}
}

func extraUpgradeSteps(env upgrade.Env) []upgrade.Step {
	return []upgrade.Step{upgrade.SystemStep(env), upgrade.FirmwareStep(env)}
}

func planExtraUpgrade(env upgrade.Env) []upgrade.Outcome {
	return upgrade.Run(context.Background(), upgrade.ModePlan, extraUpgradeSteps(env), nil)
}

func jsonUpgradeSteps(outcomes []upgrade.Outcome) []output.UpgradeStep {
	out := make([]output.UpgradeStep, 0, len(outcomes))
	for _, o := range outcomes {
		cmds := make([]string, 0, len(o.Commands))
		for _, cmd := range o.Commands {
			cmds = append(cmds, strings.Join(cmd, " "))
		}
		out = append(out, output.UpgradeStep{
			Name:     o.Name,
			Status:   string(o.Status),
			Reason:   o.Reason,
			Commands: cmds,
		})
	}
	return out
}

func extraUpgradeHasCommands(outcomes []upgrade.Outcome) bool {
	for _, o := range outcomes {
		if len(o.Commands) > 0 {
			return true
		}
	}
	return false
}

func extraJSONHasCommands(steps []output.UpgradeStep) bool {
	for _, s := range steps {
		if len(s.Commands) > 0 {
			return true
		}
	}
	return false
}

func printExtraUpgradePlan(w io.Writer, outcomes []upgrade.Outcome) {
	for _, o := range outcomes {
		if o.Status == upgrade.StatusSkipped {
			fprintf(w, "  %s: skipped (%s)\n", o.Name, o.Reason)
			continue
		}
		for _, cmd := range o.Commands {
			fprintf(w, "  %s  ==> %s\n", o.Name, strings.Join(cmd, " "))
		}
	}
}

func applyExtraUpgrade(ctx context.Context, env upgrade.Env, stdin io.Reader, stdout, stderr io.Writer) []upgrade.Outcome {
	execFn := func(ctx context.Context, cmd []string) error {
		return resolver.RunCommand(ctx, cmd, stdin, stdout, stderr)
	}
	return upgrade.Run(ctx, upgrade.ModeApply, extraUpgradeSteps(env), execFn)
}

func extraStepErrors(outcomes []upgrade.Outcome) []error {
	var errs []error
	for _, o := range outcomes {
		if o.Err != nil {
			errs = append(errs, o.Err)
		}
	}
	return errs
}

func extraJSONErrorStrings(steps []output.UpgradeStep) []string {
	var errs []string
	for _, s := range steps {
		if s.Status == string(upgrade.StatusFailed) && s.Reason != "" {
			errs = append(errs, s.Name+": "+s.Reason)
		}
	}
	return errs
}

func extraApplyForJSON(env upgrade.Env) func(ctx context.Context) []output.UpgradeStep {
	return func(ctx context.Context) []output.UpgradeStep {
		return jsonUpgradeSteps(applyExtraUpgrade(ctx, env, os.Stdin, os.Stderr, os.Stderr))
	}
}
