package service

import (
	"context"
	"fmt"
	"os"
	"os/exec"

	"github.com/ks1686/genv/internal/schema"
)

// brewServicesRestart is the seam the unit tests replace; the real
// implementation shells out to `brew services restart`.
var brewServicesRestart = BrewServicesRestart

// RestartDeclared restarts one spec-declared service on explicit request
// (`genv service restart <name>`).
//
// It deliberately does NOT restart the service's `requires` dependencies. A
// human asking for one service to be restarted means that service: taking a
// healthy database down because an API is misbehaving is a worse outage than
// the one being fixed. Dependencies are checked before the restart and reported
// if they are down, but never restarted as a side effect — that decision
// belongs to the operator, or to the upgrade restart phase, which has evidence
// that something changed.
//
// Precedence mirrors `service start` and `service stop`, so a spec that works
// for those keeps working here:
//
//  1. brew_formula  -> `brew services restart <formula>`
//  2. scheduled_task-> refused: a scheduled task is a trigger, not a resident
//     process, so there is nothing to restart
//  3. launchd       -> bootout + bootstrap (launchd has no restart verb)
//  4. systemd       -> `systemctl --user restart <unit>`
//  5. restart       -> the declared command, run once (no stop/start around it)
//  6. otherwise     -> stop, then start; a service with no stop command is
//     refused rather than half-restarted
func RestartDeclared(ctx context.Context, name string, svc schema.Service, sourceRoot string) error {
	switch {
	case svc.BrewFormula != "":
		if err := brewServicesRestart(ctx, svc.BrewFormula); err != nil {
			return fmt.Errorf("restarting brew service %q: %w", name, err)
		}
		return nil

	case svc.DeclaresScheduledTask():
		return fmt.Errorf("service %q declares a scheduled_task, which is a trigger rather than a running service; it cannot be restarted", name)

	case svc.DeclaresLaunchd():
		return restartLaunchd(ctx, name, svc, sourceRoot)

	case svc.DeclaresSystemd():
		return restartSystemd(ctx, name, svc, sourceRoot)

	case len(svc.Restart) > 0:
		if err := exec.CommandContext(ctx, svc.Restart[0], svc.Restart[1:]...).Run(); err != nil {
			return fmt.Errorf("running the restart command for %q: %w", name, err)
		}
		return nil

	case len(svc.Stop) == 0:
		return fmt.Errorf("service %q has no restart command and no stop command to fall back on; declare one of them", name)
	}

	if err := StopDeclared(ctx, name, svc, sourceRoot); err != nil {
		// Starting after a failed stop would report success for a service
		// that was never taken down.
		return fmt.Errorf("stopping %q before restart: %w", name, err)
	}
	if err := StartDeclared(ctx, name, svc, sourceRoot); err != nil {
		return fmt.Errorf("starting %q after restart: %w", name, err)
	}
	return nil
}

func restartLaunchd(ctx context.Context, name string, svc schema.Service, sourceRoot string) error {
	if !probeLaunchd() {
		return fmt.Errorf("service %q requires launchd, which is not available on this host", name)
	}
	rendered, err := renderServiceTemplate(sourceRoot, svc.Launchd.Plist)
	if err != nil {
		return fmt.Errorf("service %q launchd plist: %w", name, err)
	}
	label, err := ParseLaunchdLabel(rendered)
	if err != nil {
		return fmt.Errorf("service %q launchd plist: %w", name, err)
	}
	dest, err := launchdAgentPath(label)
	if err != nil {
		return err
	}
	if _, statErr := os.Stat(dest); statErr != nil {
		return fmt.Errorf("service %q has no launchd plist at %s; run 'genv apply' first: %w", name, dest, statErr)
	}
	// launchd has no restart verb. Bootout a loaded job, then bootstrap it
	// again. A declared but unloaded agent is bootstrapped too: the operator
	// asked to recover it, and leaving it down because the verb says "restart"
	// would be a surprising no-op.
	if launchdJobLoaded(ctx, label) {
		if err := bootoutLaunchd(ctx, label); err != nil {
			return fmt.Errorf("stopping launchd service %q: %w", name, err)
		}
	}
	if err := bootstrapLaunchd(ctx, dest); err != nil {
		return fmt.Errorf("restarting launchd service %q: %w", name, err)
	}
	return nil
}

func restartSystemd(ctx context.Context, name string, svc schema.Service, sourceRoot string) error {
	if !probeSystemd() {
		return fmt.Errorf("service %q requires systemd --user, which is not available on this host", name)
	}
	unitBase, err := systemdNameFor(name, svc)
	if err != nil {
		return fmt.Errorf("service %q systemd unit: %w", name, err)
	}
	unitName := unitBase + ".service"
	dest, err := systemdUserUnitPath(unitBase)
	if err != nil {
		return err
	}
	if _, statErr := os.Stat(dest); statErr != nil {
		return fmt.Errorf("service %q has no systemd unit at %s; run 'genv apply' first: %w", name, dest, statErr)
	}
	if err := systemctlRun(ctx, "--user", "restart", unitName); err != nil {
		return fmt.Errorf("restarting systemd service %q: %w\nTip: to view logs run: %s", unitName, err, SystemdLogsHint(name))
	}
	return nil
}

// MissingDependencies lists the declared `requires` services that are not
// running. A restart against a down dependency produces a service that starts
// and then fails, so the caller reports this instead.
func MissingDependencies(ctx context.Context, name string, svc schema.Service, specServices map[string]schema.Service, sourceRoot string) []string {
	var missing []string
	for _, dependency := range svc.Requires {
		dep, ok := specServices[dependency]
		if !ok {
			// A dangling reference is a spec error; validation reports it with
			// a line number, so here it only means "not running".
			missing = append(missing, dependency)
			continue
		}
		if !ProbeRunning(ctx, dependency, dep, sourceRoot) {
			missing = append(missing, dependency)
		}
	}
	return missing
}
