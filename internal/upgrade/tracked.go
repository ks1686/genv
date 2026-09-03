package upgrade

import "context"

// TrackedStep wraps the existing tracked-package plan so it is one named step
// in the runner. Apply should call RunUpgrade so lock writes stay in the
// planner path. Empty actions are still a real step (nothing to upgrade).
func TrackedStep(plan UpgradePlan, apply func(ctx context.Context) error) Step {
	cmds := make([][]string, 0, len(plan.Actions))
	for _, a := range plan.Actions {
		if len(a.Cmd) > 0 {
			cmds = append(cmds, a.Cmd)
		}
	}
	return Step{Name: "tracked", Commands: cmds, Apply: apply}
}
