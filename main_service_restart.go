package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/ks1686/genv/internal/genvfile"
	"github.com/ks1686/genv/internal/plan"
	"github.com/ks1686/genv/internal/schema"
	"github.com/ks1686/genv/internal/service"
	"github.com/ks1686/genv/internal/upgrade"
	"github.com/ks1686/genv/internal/verify"
)

// restartDeps are the side-effecting operations the restart phase performs.
// They are injectable so the decision logic can be tested without restarting
// anything: a test that needs a real restart is a test that restarts something.
type restartDeps struct {
	// isRunning reports whether a service is currently running.
	isRunning func(name string) bool
	// stop and start are the lifecycle operations.
	stop  func(ctx context.Context, name string, svc schema.Service) error
	start func(ctx context.Context, name string, svc schema.Service) error
	// runProbe executes one health-check command.
	runProbe func(ctx context.Context, command []string) error
}

func defaultRestartDeps(sourceRoot string, specServices map[string]schema.Service) restartDeps {
	return restartDeps{
		isRunning: func(name string) bool {
			svc, ok := specServices[name]
			if !ok {
				return false
			}
			return service.ProbeRunning(context.Background(), name, svc, sourceRoot)
		},
		stop: func(ctx context.Context, name string, svc schema.Service) error {
			return service.StopDeclared(ctx, name, svc, sourceRoot)
		},
		start: func(ctx context.Context, name string, svc schema.Service) error {
			return service.StartDeclared(ctx, name, svc, sourceRoot)
		},
		runProbe: func(ctx context.Context, command []string) error {
			if len(command) == 0 {
				return fmt.Errorf("health_check declares no command")
			}
			return exec.CommandContext(ctx, command[0], command[1:]...).Run()
		},
	}
}

// restartPhaseRequest is one run's restart context.
type restartPhaseRequest struct {
	// Services is the composed service map.
	Services map[string]schema.Service
	// Evidence maps a changed resource key to what could be established.
	Evidence map[string]service.Evidence
	// LockPath is where pending actions are recorded.
	LockPath string
	// SourceRoot resolves supervisor templates.
	SourceRoot string
	// Background marks an unattended run.
	Background bool
	// LockAlreadyHeld tells the phase that the caller already holds the lock
	// mutex for LockPath. Both the interactive upgrade and the unattended worker
	// hold it across their whole run, and flock is not re-entrant: acquiring it
	// again here would block the command forever on its own lock.
	LockAlreadyHeld bool
	// Deps inject the side effects.
	Deps restartDeps
}

// restartOutcome is the executed result for one service.
type restartOutcome struct {
	Service string
	Action  string
	Reason  string
	// Err is non-nil when the action was attempted and failed.
	Err error
	// ReadinessError is non-nil when the service started but never reported
	// ready. It is separate from Err because the remedy differs: one means the
	// restart failed, the other means it worked and the service is unhealthy.
	ReadinessError error
	// PendingCleared is true when the pending record was removed, meaning the
	// change is confirmed.
	PendingCleared bool
}

// runRestartPhase decides and performs restarts for one run.
//
// The order is deliberate. Pending records are written *before* anything is
// stopped, so an interruption anywhere in this function leaves a trace rather
// than a silent half-restart. They are cleared only after the action succeeded
// and readiness was confirmed — a cleared record means "this really happened".
func runRestartPhase(ctx context.Context, req restartPhaseRequest) []restartOutcome {
	servicePointers := make(map[string]*schema.Service, len(req.Services))
	for name, svc := range req.Services {
		svc := svc
		servicePointers[name] = &svc
	}
	p, err := plan.Build(servicePointers)
	if err != nil {
		// A cycle or dangling requires is a spec error, reported by validation;
		// here it means no restart can be ordered, so nothing is attempted.
		return nil
	}
	if req.Background {
		// In background the services whose readiness cannot be judged without a
		// human are removed from the plan entirely, so their restarts become
		// "no reason to act" rather than a blind restart.
		p = filterBackgroundServices(p)
	}

	decisions := service.PlanServiceRestarts(&p, req.Evidence, req.Deps.isRunning)
	if len(decisions) == 0 {
		return nil
	}

	now := time.Now().UTC().Format(time.RFC3339)
	outcomes := make([]restartOutcome, 0, len(decisions))
	for _, d := range decisions {
		out := restartOutcome{Service: d.Service, Action: d.Action, Reason: d.Reason}
		if d.Action == service.ActionSkip || d.Action == service.ActionDefer {
			outcomes = append(outcomes, out)
			continue
		}
		svc, ok := req.Services[d.Service]
		if !ok {
			out.Err = fmt.Errorf("service %q disappeared from the composed spec", d.Service)
			outcomes = append(outcomes, out)
			continue
		}

		recordPendingActions(req.LockPath, req.LockAlreadyHeld, d.Service, d.Triggers, now)

		if d.Action == service.ActionRestart {
			if err := req.Deps.stop(ctx, d.Service, svc); err != nil {
				out.Err = fmt.Errorf("stopping %q: %w", d.Service, err)
				outcomes = append(outcomes, out)
				continue
			}
		}
		if err := req.Deps.start(ctx, d.Service, svc); err != nil {
			out.Action = service.ActionStart
			out.Err = fmt.Errorf("starting %q: %w", d.Service, err)
			outcomes = append(outcomes, out)
			continue
		}
		if svc.HealthCheck != nil {
			if err := verify.Health(ctx, svc.HealthCheck, func(ctx context.Context) error {
				return req.Deps.runProbe(ctx, svc.HealthCheck.Command)
			}); err != nil {
				out.ReadinessError = fmt.Errorf("%q started but %w", d.Service, err)
				outcomes = append(outcomes, out)
				continue
			}
		}
		out.PendingCleared = clearPendingActions(req.LockPath, req.LockAlreadyHeld, d.Service)
		outcomes = append(outcomes, out)
	}
	return outcomes
}

// pastTenseRestartAction renders an action for the completion line. The report
// is about what happened, so the present-tense constant would read as an
// instruction ("service api: restart") and quietly invite a re-run.
func pastTenseRestartAction(action string) string {
	switch action {
	case service.ActionRestart:
		return "restarted"
	case service.ActionStart:
		return "started"
	case service.ActionSkip:
		return "left running state alone"
	case service.ActionDefer:
		return "deferred"
	default:
		return action
	}
}

// filterBackgroundServices drops services whose health check is not permitted to
// run unattended. Their restarts are deferred to the next interactive run
// rather than performed blind.
func filterBackgroundServices(p plan.Plan) plan.Plan {
	filtered := plan.Plan{StopOrder: p.StopOrder}
	for _, node := range p.Nodes {
		if node.HealthCheck != nil && !node.HealthCheck.AllowBackground {
			continue
		}
		filtered.Nodes = append(filtered.Nodes, node)
	}
	return filtered
}

// mutateLock applies fn to the lock file and writes it back when fn reports a
// change.
//
// alreadyHeld must be true when the caller already holds the mutex: flock on the
// sidecar is exclusive and not re-entrant, so re-acquiring it inside a command
// that holds it blocks the command on its own lock forever.
func mutateLock(lockPath string, alreadyHeld bool, fn func(*genvfile.LockFile) bool) {
	if lockPath == "" {
		return
	}
	if !alreadyHeld {
		unlock, err := genvfile.LockMutation(lockPath)
		if err != nil {
			return
		}
		defer unlock()
	}
	lf, err := genvfile.ReadLock(lockPath)
	if err != nil {
		return
	}
	if fn(lf) {
		_ = genvfile.WriteLock(lockPath, lf)
	}
}

// recordPendingActions writes the in-flight record before the service is
// touched. Failure to record is not fatal: the restart still proceeds, but the
// outcome says the run left no trace, which is itself worth reporting.
func recordPendingActions(lockPath string, alreadyHeld bool, serviceName string, triggers []string, recordedAt string) {
	mutateLock(lockPath, alreadyHeld, func(lf *genvfile.LockFile) bool {
		for _, trigger := range triggers {
			lf.PendingActions = append(lf.PendingActions, genvfile.PendingAction{
				Name:       serviceName,
				Package:    trigger,
				Reason:     "restart in progress; readiness not yet confirmed",
				RecordedAt: recordedAt,
			})
		}
		return true
	})
}

// clearPendingActions removes the record after the action and readiness both
// succeeded, and reports whether anything was removed.
func clearPendingActions(lockPath string, alreadyHeld bool, serviceName string) bool {
	cleared := false
	mutateLock(lockPath, alreadyHeld, func(lf *genvfile.LockFile) bool {
		if lf.ClearPendingActions(serviceName) == 0 {
			return false
		}
		cleared = true
		return true
	})
	return cleared
}

// reportUncertainPendingActions prints the interrupted changes found in the
// lock. They are reported and left in place: replaying a restart unattended
// would be acting on a guess, and deleting the record would destroy the only
// evidence that something needs a human.
func reportUncertainPendingActions(lockPath string) []genvfile.PendingAction {
	if lockPath == "" {
		return nil
	}
	lf, err := genvfile.ReadLock(lockPath)
	if err != nil {
		return nil
	}
	for _, pa := range lf.PendingActions {
		fprintf(os.Stderr, "genv: uncertain pending change: service %q was to be restarted after %q changed (%s); it was not confirmed. Re-run 'genv upgrade' interactively.\n",
			pa.Name, pa.Package, pa.RecordedAt)
	}
	return lf.PendingActions
}

// backgroundDeferrals returns the packages the unattended worker must not
// upgrade because a service watching them cannot be restarted and verified
// without a human.
//
// The check runs *before* any upgrade so an unattended run cannot leave a
// service running against a binary it cannot restart.
func backgroundDeferrals(p *plan.Plan, evidence map[string]service.Evidence) []string {
	if p == nil {
		return nil
	}
	deferred := map[string]bool{}
	for _, node := range p.Nodes {
		if node.HealthCheck == nil || node.HealthCheck.AllowBackground {
			continue
		}
		for _, watch := range node.WatchTargets {
			resource := service.TriggerResource(watch)
			if _, tracked := evidence[resource]; !tracked {
				continue
			}
			deferred[resource] = true
		}
	}
	if len(deferred) == 0 {
		return nil
	}
	out := make([]string, 0, len(deferred))
	for name := range deferred {
		out = append(out, name)
	}
	sortStrings(out)
	return out
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}

// backgroundUpgradeDeferrals returns the planned package ids the unattended
// worker must not upgrade, because a service watching them cannot be restarted
// and verified without a human.
//
// The evidence map is built from the *planned* packages, because the question is
// prospective: if this upgrade happens, can genv responsibly follow it through?
func backgroundUpgradeDeferrals(services map[string]schema.Service, upPlan upgrade.UpgradePlan) []string {
	if len(services) == 0 || len(upPlan.Actions) == 0 {
		return nil
	}
	servicePointers := make(map[string]*schema.Service, len(services))
	for name, svc := range services {
		svc := svc
		servicePointers[name] = &svc
	}
	p, err := plan.Build(servicePointers)
	if err != nil {
		// An unorderable service graph is a spec error; validate reports it.
		// Deferring every planned upgrade here would be a surprising response
		// to a typo, so only the packages we can reason about are deferred.
		return nil
	}
	evidence := make(map[string]service.Evidence)
	for _, action := range upPlan.Actions {
		for _, lp := range action.LPs {
			evidence[lp.ID] = service.EvidenceChanged
		}
	}
	return backgroundDeferrals(&p, evidence)
}

// filterUpgradePlanExcluding drops the actions for the given package ids,
// leaving the rest of the plan intact.
func filterUpgradePlanExcluding(upPlan upgrade.UpgradePlan, drop []string) upgrade.UpgradePlan {
	if len(drop) == 0 {
		return upPlan
	}
	dropped := make(map[string]bool, len(drop))
	for _, id := range drop {
		dropped[id] = true
	}
	filtered := upPlan
	filtered.Actions = nil
	for _, action := range upPlan.Actions {
		keep := make([]genvfile.LockedPackage, 0, len(action.LPs))
		for _, lp := range action.LPs {
			if dropped[lp.ID] {
				continue
			}
			keep = append(keep, lp)
		}
		if len(keep) == 0 {
			continue
		}
		action.LPs = keep
		filtered.Actions = append(filtered.Actions, action)
	}
	return filtered
}
