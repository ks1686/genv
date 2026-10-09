package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/ks1686/genv/internal/files"
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
		runProbe: runHealthProbe,
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
	// PendingError is non-nil when the in-flight record could not be written.
	// The restart still proceeds, but nothing would survive an interruption.
	PendingError error
}

// runRestartPhase decides and performs restarts for one run.
//
// The order is deliberate, in three parts.
//
//  1. Every in-flight record is written *before* anything is stopped, so an
//     interruption anywhere below leaves evidence for all of them rather than a
//     silent half-restart.
//  2. Stops run in reverse dependency order. A service goes down before the
//     services that depend on it, which is what plan.StopOrder is for; taking a
//     database out from under a live client is the failure this prevents.
//  3. Starts run in dependency order, and a service whose dependency did not
//     come back is not started at all — otherwise a service with no health check
//     would be reported as successfully started against something that is down.
//
// Records are cleared only after the action succeeded and readiness was
// confirmed, so a cleared record means "this really happened".
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

	requires := make(map[string][]string, len(p.Nodes))
	startOrder := make([]string, 0, len(p.Nodes))
	for _, node := range p.Nodes {
		requires[node.Name] = node.Requires
		startOrder = append(startOrder, node.Name)
	}

	byName := make(map[string]*service.RestartDecision, len(decisions))
	outcomes := make([]restartOutcome, 0, len(decisions))
	var actionable []string // in start order

	for i := range decisions {
		d := decisions[i]
		out := restartOutcome{Service: d.Service, Action: d.Action, Reason: d.Reason}
		if d.Action == service.ActionSkip || d.Action == service.ActionDefer {
			outcomes = append(outcomes, out)
			continue
		}
		if _, ok := req.Services[d.Service]; !ok {
			out.Err = fmt.Errorf("service %q disappeared from the composed spec", d.Service)
			outcomes = append(outcomes, out)
			continue
		}
		byName[d.Service] = &decisions[i]
		actionable = append(actionable, d.Service)
	}
	if len(actionable) == 0 {
		return outcomes
	}

	now := time.Now().UTC().Format(time.RFC3339)
	pendingErr := make(map[string]error, len(actionable))
	for _, name := range actionable {
		pendingErr[name] = recordPendingActions(req.LockPath, req.LockAlreadyHeld, name, byName[name].Triggers, now)
	}

	stopped := make(map[string]bool, len(actionable))
	errs := make(map[string]error)
	readiness := make(map[string]error)

	for _, name := range p.StopOrder {
		d := byName[name]
		if d == nil || d.Action != service.ActionRestart {
			continue
		}
		if err := req.Deps.stop(ctx, name, req.Services[name]); err != nil {
			errs[name] = fmt.Errorf("stopping %q: %w", name, err)
		}
		stopped[name] = true
	}

	for _, name := range startOrder {
		d := byName[name]
		if d == nil {
			continue
		}
		if blocker := firstFailedDependency(name, requires, errs); blocker != "" {
			errs[name] = fmt.Errorf("not started: %q did not come back: %v", blocker, errs[blocker])
			continue
		}
		if err := req.Deps.start(ctx, name, req.Services[name]); err != nil {
			errs[name] = fmt.Errorf("starting %q: %w", name, err)
			continue
		}
		if hc := req.Services[name].HealthCheck; hc != nil {
			if err := verify.Health(ctx, hc, func(ctx context.Context) error {
				return req.Deps.runProbe(ctx, hc.Command)
			}); err != nil {
				readiness[name] = fmt.Errorf("%q started but %w", name, err)
			}
		}
	}

	for i := range decisions {
		d := decisions[i]
		if byName[d.Service] != &decisions[i] {
			continue // skip, defer, or already handled above
		}
		out := restartOutcome{Service: d.Service, Action: d.Action, Reason: d.Reason}
		if pe := pendingErr[d.Service]; pe != nil {
			out.PendingError = fmt.Errorf("recording the pending restart failed, so an interruption here would leave no evidence: %w", pe)
		}
		switch {
		case errs[d.Service] != nil:
			out.Err = errs[d.Service]
			// The stop already happened, so the action attempted last was a
			// start. Reporting "restart" here would claim otherwise.
			if stopped[d.Service] {
				out.Action = service.ActionStart
			}
		case readiness[d.Service] != nil:
			out.ReadinessError = readiness[d.Service]
		default:
			out.PendingCleared = clearPendingActions(req.LockPath, req.LockAlreadyHeld, d.Service)
		}
		outcomes = append(outcomes, out)
	}
	return outcomes
}

// firstFailedDependency returns the nearest service in name's dependency chain
// that failed, so the report can name what actually went wrong rather than only
// that something did.
func firstFailedDependency(name string, requires map[string][]string, errs map[string]error) string {
	for _, dep := range requires[name] {
		if errs[dep] != nil {
			return dep
		}
		if deeper := firstFailedDependency(dep, requires, errs); deeper != "" {
			return deeper
		}
	}
	return ""
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
	var filtered plan.Plan
	for _, node := range p.Nodes {
		if node.HealthCheck != nil && !node.HealthCheck.AllowBackground {
			continue
		}
		filtered.Nodes = append(filtered.Nodes, node)
	}
	// StopOrder is the reverse of the nodes that *survive* the filter. Carrying
	// the original would list services this plan no longer contains, and the
	// stop phase iterates it directly.
	for i := len(filtered.Nodes) - 1; i >= 0; i-- {
		filtered.StopOrder = append(filtered.StopOrder, filtered.Nodes[i].Name)
	}
	return filtered
}

// mutateLock applies fn to the lock file and writes it back when fn reports a
// change.
//
// alreadyHeld must be true when the caller already holds the mutex: flock on the
// sidecar is exclusive and not re-entrant, so re-acquiring it inside a command
// that holds it blocks the command on its own lock forever.
func mutateLock(lockPath string, alreadyHeld bool, fn func(*genvfile.LockFile) bool) error {
	if lockPath == "" {
		return nil
	}
	if !alreadyHeld {
		unlock, err := genvfile.LockMutation(lockPath)
		if err != nil {
			return err
		}
		defer unlock()
	}
	lf, err := genvfile.ReadLock(lockPath)
	if err != nil {
		return err
	}
	if fn(lf) {
		return genvfile.WriteLock(lockPath, lf)
	}
	return nil
}

// recordPendingActions writes the in-flight record before the service is
// touched, and reports whether it landed.
//
// A failure here is not fatal: refusing to restart because the lock could not
// be written would leave the service running the old binary, which is the worse
// outcome. But it is not silent either — the record is the only thing that
// survives an interruption, so the caller reports the gap.
func recordPendingActions(lockPath string, alreadyHeld bool, serviceName string, triggers []string, recordedAt string) error {
	return mutateLock(lockPath, alreadyHeld, func(lf *genvfile.LockFile) bool {
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
	_ = mutateLock(lockPath, alreadyHeld, func(lf *genvfile.LockFile) bool {
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

// fileWatchEvidence reports what this apply did to each watched file destination.
//
// Keys are the watch entries exactly as the spec spells them, because that is
// what PlanServiceRestarts looks up. Destinations come back from the applier
// fully expanded, so each entry is expanded before being compared — a spec may
// write "~/.config/api.conf" while the applier reports the absolute path.
//
// A destination nobody watches produces no entry at all: evidence is only
// interesting where something acts on it.
func fileWatchEvidence(services map[string]schema.Service, res *files.ApplyResult) map[string]service.Evidence {
	if len(services) == 0 || res == nil {
		return nil
	}
	changed := make(map[string]bool, len(res.Created)+len(res.Updated))
	for _, p := range append(append([]string{}, res.Created...), res.Updated...) {
		changed[p] = true
	}
	uncertain := make(map[string]bool, len(res.Mismatched))
	for _, p := range res.Mismatched {
		uncertain[p] = true
	}

	out := map[string]service.Evidence{}
	for _, svc := range services {
		if svc.Watch == nil {
			continue
		}
		for _, watch := range schema.WatchTargets(svc.Watch) {
			entry := service.TriggerResource(watch)
			expanded := entry
			if e, err := files.ExpandTarget(entry); err == nil {
				expanded = e
			}
			switch {
			case changed[expanded]:
				out[entry] = service.EvidenceChanged
			case uncertain[expanded]:
				// The file is in a state genv could not reconcile, so whether
				// the service should restart cannot be established either.
				out[entry] = service.EvidenceUnknown
			}
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// applyRestartPhase runs the restart coordinator after `genv apply` changed a
// managed file that a service watches.
//
// It is skipped on a dry run (a plan that restarts a service is not a plan) and
// when the apply itself failed, because restarting on top of a half-applied
// environment trades one broken state for another. applyCmd holds the lock
// mutex for its whole run, which is why LockAlreadyHeld is set.
func applyRestartPhase(ctx context.Context, opts applyOptions, lockPath string, f *schema.GenvFile, appliedFiles *files.ApplyResult) []restartOutcome {
	if opts.DryRun || appliedFiles == nil || len(f.Services) == 0 {
		return nil
	}
	evidence := fileWatchEvidence(f.Services, appliedFiles)
	if len(evidence) == 0 {
		return nil
	}
	sourceRoot := applySourceRoot(opts, f)
	return runRestartPhase(ctx, restartPhaseRequest{
		Services:        f.Services,
		Evidence:        evidence,
		LockPath:        lockPath,
		SourceRoot:      sourceRoot,
		Deps:            defaultRestartDeps(sourceRoot, f.Services),
		LockAlreadyHeld: true, // applyCmd holds this mutex for its whole run
	})
}

// reportApplyRestarts prints the phase's outcomes on the human path.
func reportApplyRestarts(outcomes []restartOutcome, prefix string) int {
	exitCode := exitOK
	for _, o := range outcomes {
		switch {
		case o.PendingError != nil:
			fprintf(os.Stderr, "%sservice %s: %v\n", prefix, o.Service, o.PendingError)
			exitCode = exitLogic
		case o.Err != nil:
			fprintf(os.Stderr, "%sservice %s: %v\n", prefix, o.Service, o.Err)
			exitCode = exitLogic
		case o.ReadinessError != nil:
			fprintf(os.Stderr, "%sservice %s: %v\n", prefix, o.Service, o.ReadinessError)
			exitCode = exitLogic
		case o.Action == service.ActionSkip, o.Action == service.ActionDefer:
			fprintf(os.Stderr, "%sservice %s: %s\n", prefix, o.Service, o.Reason)
		default:
			fprintf(os.Stdout, "%sservice %s: %s\n", prefix, o.Service, pastTenseRestartAction(o.Action))
		}
	}
	return exitCode
}
