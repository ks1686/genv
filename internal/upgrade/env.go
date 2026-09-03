package upgrade

import "os/exec"

// Env is the host view used to select OS/firmware commands without executing
// them. Tests inject LookPath so CI never calls real OS updaters.
type Env struct {
	Target   string
	GOOS     string
	LookPath func(file string) (string, error)
}

func (e Env) has(bin string) bool {
	look := e.LookPath
	if look == nil {
		look = exec.LookPath
	}
	_, err := look(bin)
	return err == nil
}

func (e Env) look(bin string) string {
	look := e.LookPath
	if look == nil {
		look = exec.LookPath
	}
	path, err := look(bin)
	if err != nil {
		return ""
	}
	return path
}

// CommandPlan is a step's argv list, or a skip reason when the tool is absent.
type CommandPlan struct {
	Commands   [][]string
	SkipReason string
}

func skipPlan(reason string) CommandPlan {
	return CommandPlan{SkipReason: reason}
}

func cmdPlan(cmds ...[]string) CommandPlan {
	return CommandPlan{Commands: cmds}
}

func stepFromPlan(name string, plan CommandPlan) Step {
	return Step{Name: name, SkipReason: plan.SkipReason, Commands: plan.Commands}
}

// SystemStep is the OS vendor updater for the active genv target.
func SystemStep(env Env) Step {
	return stepFromPlan("system", PlanSystem(env))
}

// FirmwareStep is fwupd on Linux. macOS firmware ships through softwareupdate
// (system step); Windows firmware is vendor-specific and skipped.
func FirmwareStep(env Env) Step {
	return stepFromPlan("firmware", PlanFirmware(env))
}
