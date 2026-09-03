package upgrade

import (
	"context"
	"errors"
)

// Mode selects whether the runner only plans commands or executes them.
type Mode int

const (
	// ModePlan records commands without executing them (upgrade --dry-run).
	ModePlan Mode = iota
	// ModeApply executes each available step. Failures are recorded and the
	// runner continues, matching ExecuteUpgrade.
	ModeApply
)

// Status is the outcome of one named step.
type Status string

const (
	StatusPlanned Status = "planned"
	StatusRan     Status = "ran"
	StatusSkipped Status = "skipped"
	StatusFailed  Status = "failed"
)

// Step is one named upgrade action. SkipReason means the tool or OS is absent
// and the step must not run. When Apply is set, ModeApply calls it instead of
// executing Commands with the generic runner (used by tracked packages so the
// existing planner / RunUpgrade path stays in charge of lock updates).
//
// This slice ships tracked (via CLI) + system + firmware. Follow-ups add
// rustup, editors, containers, and other extra tools — do not add those here.
type Step struct {
	Name       string
	SkipReason string
	Commands   [][]string
	Apply      func(ctx context.Context) error
}

// Outcome is the recorded result of running one Step.
type Outcome struct {
	Name     string
	Status   Status
	Reason   string
	Commands [][]string
	Err      error
}

// ExecFunc runs one argv. Tests inject a fake so OS updaters never execute.
type ExecFunc func(ctx context.Context, cmd []string) error

// Run executes steps in order. A failed step does not abort the rest of the run.
func Run(ctx context.Context, mode Mode, steps []Step, exec ExecFunc) []Outcome {
	out := make([]Outcome, 0, len(steps))
	for _, step := range steps {
		out = append(out, runOne(ctx, mode, step, exec))
	}
	return out
}

func runOne(ctx context.Context, mode Mode, step Step, exec ExecFunc) Outcome {
	o := Outcome{Name: step.Name, Commands: step.Commands}
	if step.SkipReason != "" {
		o.Status = StatusSkipped
		o.Reason = step.SkipReason
		o.Commands = nil
		return o
	}
	if mode == ModePlan {
		o.Status = StatusPlanned
		return o
	}
	var err error
	if step.Apply != nil {
		err = step.Apply(ctx)
	} else {
		err = execCommands(ctx, step.Commands, exec)
	}
	if err != nil {
		o.Status = StatusFailed
		o.Err = err
		o.Reason = err.Error()
		return o
	}
	o.Status = StatusRan
	return o
}

func execCommands(ctx context.Context, cmds [][]string, exec ExecFunc) error {
	if exec == nil {
		return errors.New("upgrade: no command executor")
	}
	var errs error
	for _, cmd := range cmds {
		if err := exec(ctx, cmd); err != nil {
			errs = errors.Join(errs, err)
		}
	}
	return errs
}
