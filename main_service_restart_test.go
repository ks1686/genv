package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ks1686/genv/internal/genvfile"
	"github.com/ks1686/genv/internal/plan"
	"github.com/ks1686/genv/internal/resolver"
	"github.com/ks1686/genv/internal/schema"
	"github.com/ks1686/genv/internal/service"
	"github.com/ks1686/genv/internal/upgrade"
)

// recordingDeps captures the lifecycle calls a restart phase makes, so the
// tests assert on what genv did without restarting anything.
type recordingDeps struct {
	calls    []string
	running  map[string]bool
	stopErr  error
	startErr error
	probeErr error
	probes   int
}

func (d *recordingDeps) deps() restartDeps {
	return restartDeps{
		isRunning: func(name string) bool { return d.running[name] },
		stop: func(_ context.Context, name string, _ schema.Service) error {
			d.calls = append(d.calls, "stop:"+name)
			return d.stopErr
		},
		start: func(_ context.Context, name string, _ schema.Service) error {
			d.calls = append(d.calls, "start:"+name)
			return d.startErr
		},
		runProbe: func(context.Context, []string) error {
			d.probes++
			return d.probeErr
		},
	}
}

func apiService(watch ...string) map[string]schema.Service {
	svc := schema.Service{
		Start: []string{"true"},
		Watch: watch,
	}
	return map[string]schema.Service{"api": svc}
}

func TestUpgrade_restarts_watched_service_after_proven_change(t *testing.T) {
	dir := t.TempDir()
	lockPath := filepath.Join(dir, "genv.lock.json")
	deps := &recordingDeps{running: map[string]bool{"api": true}}

	outcomes := runRestartPhase(context.Background(), restartPhaseRequest{
		Services: apiService("watch:postgres"),
		Evidence: map[string]service.Evidence{"postgres": service.EvidenceChanged},
		LockPath: lockPath,
		Deps:     deps.deps(),
	})

	if len(outcomes) != 1 {
		t.Fatalf("outcomes = %+v", outcomes)
	}
	if outcomes[0].Action != service.ActionRestart {
		t.Errorf("action = %q, want restart", outcomes[0].Action)
	}
	if outcomes[0].Err != nil || outcomes[0].ReadinessError != nil {
		t.Errorf("unexpected failure: %+v", outcomes[0])
	}
	if strings.Join(deps.calls, ",") != "stop:api,start:api" {
		t.Errorf("calls = %v, want stop then start", deps.calls)
	}
	// A confirmed change must leave no pending record behind.
	lf, err := genvfile.ReadLock(lockPath)
	if err != nil {
		t.Fatalf("read lock: %v", err)
	}
	if len(lf.PendingActions) != 0 {
		t.Errorf("pending actions = %+v, want cleared after confirmation", lf.PendingActions)
	}
}

func TestUpgrade_defers_restart_when_evidence_unknown(t *testing.T) {
	dir := t.TempDir()
	lockPath := filepath.Join(dir, "genv.lock.json")
	deps := &recordingDeps{running: map[string]bool{"api": true}}

	outcomes := runRestartPhase(context.Background(), restartPhaseRequest{
		Services: apiService("watch:postgres"),
		Evidence: map[string]service.Evidence{"postgres": service.EvidenceUnknown},
		LockPath: lockPath,
		Deps:     deps.deps(),
	})

	if len(outcomes) != 1 || outcomes[0].Action != service.ActionDefer {
		t.Fatalf("outcomes = %+v, want one defer", outcomes)
	}
	if len(deps.calls) != 0 {
		t.Errorf("an unknown change must not restart anything, calls = %v", deps.calls)
	}
	lf, _ := genvfile.ReadLock(lockPath)
	if len(lf.PendingActions) != 0 {
		t.Errorf("a deferral records nothing: %+v", lf.PendingActions)
	}
}

func TestRestartPhase_records_pending_before_stopping(t *testing.T) {
	// If the restart fails, the pending record must survive so the next run
	// reports the uncertainty instead of assuming it worked.
	dir := t.TempDir()
	lockPath := filepath.Join(dir, "genv.lock.json")
	deps := &recordingDeps{running: map[string]bool{"api": true}, startErr: errors.New("boom")}

	outcomes := runRestartPhase(context.Background(), restartPhaseRequest{
		Services: apiService("watch:postgres"),
		Evidence: map[string]service.Evidence{"postgres": service.EvidenceChanged},
		LockPath: lockPath,
		Deps:     deps.deps(),
	})
	if len(outcomes) != 1 || outcomes[0].Err == nil {
		t.Fatalf("outcomes = %+v, want a failure", outcomes)
	}
	lf, err := genvfile.ReadLock(lockPath)
	if err != nil {
		t.Fatalf("read lock: %v", err)
	}
	if len(lf.PendingActions) != 1 {
		t.Fatalf("pending actions = %+v, want the unconfirmed change retained", lf.PendingActions)
	}
	if lf.PendingActions[0].Name != "api" || lf.PendingActions[0].Package != "postgres" {
		t.Errorf("pending action = %+v", lf.PendingActions[0])
	}
}

func TestRestartPhase_unready_service_is_reported_separately(t *testing.T) {
	dir := t.TempDir()
	lockPath := filepath.Join(dir, "genv.lock.json")
	svcs := apiService("watch:postgres")
	svc := svcs["api"]
	svc.HealthCheck = &schema.HealthCheck{Command: []string{"probe"}, Timeout: "60ms", Interval: "10ms"}
	svcs["api"] = svc
	deps := &recordingDeps{running: map[string]bool{"api": true}, probeErr: errors.New("not ready")}

	outcomes := runRestartPhase(context.Background(), restartPhaseRequest{
		Services: svcs,
		Evidence: map[string]service.Evidence{"postgres": service.EvidenceChanged},
		LockPath: lockPath,
		Deps:     deps.deps(),
	})
	if len(outcomes) != 1 {
		t.Fatalf("outcomes = %+v", outcomes)
	}
	// The restart itself worked; only readiness failed. Conflating the two would
	// send a user to debug the wrong thing.
	if outcomes[0].Err != nil {
		t.Errorf("restart failure = %v, want nil", outcomes[0].Err)
	}
	if outcomes[0].ReadinessError == nil {
		t.Error("readiness failure should be reported")
	}
	if deps.probes < 2 {
		t.Errorf("probe ran %d times, want retries", deps.probes)
	}
	lf, _ := genvfile.ReadLock(lockPath)
	if len(lf.PendingActions) != 1 {
		t.Errorf("an unready service must keep its pending record: %+v", lf.PendingActions)
	}
}

func TestRestartPhase_ready_service_clears_pending(t *testing.T) {
	dir := t.TempDir()
	lockPath := filepath.Join(dir, "genv.lock.json")
	svcs := apiService("watch:postgres")
	svc := svcs["api"]
	svc.HealthCheck = &schema.HealthCheck{Command: []string{"probe"}, Timeout: "1s", Interval: "10ms"}
	svcs["api"] = svc
	deps := &recordingDeps{running: map[string]bool{"api": true}}

	outcomes := runRestartPhase(context.Background(), restartPhaseRequest{
		Services: svcs,
		Evidence: map[string]service.Evidence{"postgres": service.EvidenceChanged},
		LockPath: lockPath,
		Deps:     deps.deps(),
	})
	if len(outcomes) != 1 || outcomes[0].ReadinessError != nil {
		t.Fatalf("outcomes = %+v", outcomes)
	}
	if !outcomes[0].PendingCleared {
		t.Error("a confirmed restart should clear its pending record")
	}
	lf, _ := genvfile.ReadLock(lockPath)
	if len(lf.PendingActions) != 0 {
		t.Errorf("pending actions = %+v, want none", lf.PendingActions)
	}
}

func TestRestartPhase_stops_stopped_service_under_never_policy(t *testing.T) {
	dir := t.TempDir()
	deps := &recordingDeps{running: map[string]bool{}}
	outcomes := runRestartPhase(context.Background(), restartPhaseRequest{
		Services: apiService("watch:postgres"),
		Evidence: map[string]service.Evidence{"postgres": service.EvidenceChanged},
		LockPath: filepath.Join(dir, "genv.lock.json"),
		Deps:     deps.deps(),
	})
	if len(outcomes) != 1 || outcomes[0].Action != service.ActionSkip {
		t.Fatalf("outcomes = %+v, want a skip", outcomes)
	}
	if len(deps.calls) != 0 {
		t.Errorf("calls = %v, want none", deps.calls)
	}
}

func TestRestartPhase_ifRunning_starts_a_stopped_service(t *testing.T) {
	dir := t.TempDir()
	svcs := apiService("watch:postgres")
	svc := svcs["api"]
	svc.RestartPolicy = schema.RestartPolicyIfRunning
	svcs["api"] = svc
	deps := &recordingDeps{running: map[string]bool{}}

	outcomes := runRestartPhase(context.Background(), restartPhaseRequest{
		Services: svcs,
		Evidence: map[string]service.Evidence{"postgres": service.EvidenceChanged},
		LockPath: filepath.Join(dir, "genv.lock.json"),
		Deps:     deps.deps(),
	})
	if len(outcomes) != 1 || outcomes[0].Action != service.ActionStart {
		t.Fatalf("outcomes = %+v, want a start", outcomes)
	}
	if strings.Join(deps.calls, ",") != "start:api" {
		t.Errorf("calls = %v, want only a start", deps.calls)
	}
}

func TestRestartPhase_respects_requires_order(t *testing.T) {
	dir := t.TempDir()
	svcs := map[string]schema.Service{
		"web": {Start: []string{"true"}, Watch: []string{"watch:nginx.conf"}},
		"db":  {Start: []string{"true"}, Watch: []string{"watch:postgres"}},
	}
	deps := &recordingDeps{running: map[string]bool{"web": true, "db": true}}

	runRestartPhase(context.Background(), restartPhaseRequest{
		Services: svcs,
		Evidence: map[string]service.Evidence{
			"postgres":   service.EvidenceChanged,
			"nginx.conf": service.EvidenceChanged,
		},
		LockPath: filepath.Join(dir, "genv.lock.json"),
		Deps:     deps.deps(),
	})
	if len(deps.calls) != 4 {
		t.Fatalf("calls = %v, want two restart pairs", deps.calls)
	}
	// Both are independent, so only the per-service stop-then-start order is
	// guaranteed; what must not happen is start-before-stop for one service.
	for i := 0; i < len(deps.calls); i += 2 {
		if !strings.HasPrefix(deps.calls[i], "stop:") || !strings.HasPrefix(deps.calls[i+1], "start:") {
			t.Errorf("call order = %v, want stop before start", deps.calls)
			break
		}
	}
}

func TestRestartPhase_background_defers_health_check_without_allow_background(t *testing.T) {
	dir := t.TempDir()
	svcs := apiService("watch:postgres")
	svc := svcs["api"]
	svc.HealthCheck = &schema.HealthCheck{Command: []string{"probe"}} // allow_background false
	svcs["api"] = svc
	deps := &recordingDeps{running: map[string]bool{"api": true}}

	outcomes := runRestartPhase(context.Background(), restartPhaseRequest{
		Services:   svcs,
		Evidence:   map[string]service.Evidence{"postgres": service.EvidenceChanged},
		LockPath:   filepath.Join(dir, "genv.lock.json"),
		Background: true,
		Deps:       deps.deps(),
	})
	if len(outcomes) != 0 {
		t.Errorf("outcomes = %+v, want the service dropped entirely in background", outcomes)
	}
	if len(deps.calls) != 0 {
		t.Errorf("background must not restart without a readiness check it can judge: %v", deps.calls)
	}
}

func TestRestartPhase_background_restarts_when_allowed(t *testing.T) {
	dir := t.TempDir()
	svcs := apiService("watch:postgres")
	svc := svcs["api"]
	svc.HealthCheck = &schema.HealthCheck{Command: []string{"probe"}, AllowBackground: true, Timeout: "1s", Interval: "10ms"}
	svcs["api"] = svc
	deps := &recordingDeps{running: map[string]bool{"api": true}}

	outcomes := runRestartPhase(context.Background(), restartPhaseRequest{
		Services:   svcs,
		Evidence:   map[string]service.Evidence{"postgres": service.EvidenceChanged},
		LockPath:   filepath.Join(dir, "genv.lock.json"),
		Background: true,
		Deps:       deps.deps(),
	})
	if len(outcomes) != 1 || outcomes[0].Action != service.ActionRestart {
		t.Fatalf("outcomes = %+v", outcomes)
	}
}

func TestBackgroundDeferrals_lists_unverifiable_packages(t *testing.T) {
	svcs := apiService("watch:postgres")
	svc := svcs["api"]
	svc.HealthCheck = &schema.HealthCheck{Command: []string{"probe"}}
	svcs["api"] = svc
	p, err := plan.Build(map[string]*schema.Service{"api": &svc})
	if err != nil {
		t.Fatalf("plan.Build: %v", err)
	}
	got := backgroundDeferrals(&p, map[string]service.Evidence{"postgres": service.EvidenceChanged})
	if len(got) != 1 || got[0] != "postgres" {
		t.Errorf("deferrals = %v, want [postgres]", got)
	}
	if got := backgroundDeferrals(&p, nil); got != nil {
		t.Errorf("deferrals with nothing to do = %v, want none", got)
	}
	if got := backgroundDeferrals(nil, map[string]service.Evidence{"x": service.EvidenceChanged}); got != nil {
		t.Errorf("deferrals with no plan = %v, want none", got)
	}
}

func TestUpdatesWorker_reports_uncertain_pending_action(t *testing.T) {
	dir := t.TempDir()
	lockPath := filepath.Join(dir, "genv.lock.json")
	lf := &genvfile.LockFile{SchemaVersion: schema.Version8, PendingActions: []genvfile.PendingAction{{
		Name: "api", Package: "postgres", RecordedAt: "2026-10-06T00:00:00Z",
	}}}
	if err := genvfile.WriteLock(lockPath, lf); err != nil {
		t.Fatalf("write lock: %v", err)
	}

	var out strings.Builder
	stderr := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stderr = w
	found := reportUncertainPendingActions(lockPath)
	_ = w.Close()
	os.Stderr = stderr
	buf := make([]byte, 4096)
	n, _ := r.Read(buf)
	out.Write(buf[:n])

	if len(found) != 1 || found[0].Name != "api" {
		t.Errorf("reported = %+v, want the pending api action", found)
	}
	if !strings.Contains(out.String(), "uncertain pending change") {
		t.Errorf("output = %q, want an explicit uncertainty report", out.String())
	}
	// The record must survive: it is the only evidence a human needs.
	after, _ := genvfile.ReadLock(lockPath)
	if len(after.PendingActions) != 1 {
		t.Errorf("the pending record was consumed: %+v", after.PendingActions)
	}
}

func TestReportUncertainPendingActions_without_lock(t *testing.T) {
	if got := reportUncertainPendingActions(""); got != nil {
		t.Errorf("with no lock path = %+v, want none", got)
	}
	if got := reportUncertainPendingActions(filepath.Join(t.TempDir(), "missing.json")); got != nil {
		t.Errorf("with a missing lock = %+v, want none", got)
	}
}

func TestRestartPhase_bad_plan_restarts_nothing(t *testing.T) {
	deps := &recordingDeps{running: map[string]bool{"api": true}}
	// A cycle means no order can be produced, so nothing is attempted.
	svcs := map[string]schema.Service{
		"a": {Start: []string{"true"}, Requires: []string{"b"}, Watch: []string{"watch:postgres"}},
		"b": {Start: []string{"true"}, Requires: []string{"a"}, Watch: []string{"watch:redis"}},
	}
	outcomes := runRestartPhase(context.Background(), restartPhaseRequest{
		Services: svcs,
		Evidence: map[string]service.Evidence{"postgres": service.EvidenceChanged, "redis": service.EvidenceChanged},
		Deps:     deps.deps(),
	})
	if len(outcomes) != 0 || len(deps.calls) != 0 {
		t.Errorf("outcomes = %+v, calls = %v, want nothing attempted", outcomes, deps.calls)
	}
}

func TestRestartPhase_without_lock_path_still_restarts(t *testing.T) {
	deps := &recordingDeps{running: map[string]bool{"api": true}}
	outcomes := runRestartPhase(context.Background(), restartPhaseRequest{
		Services: apiService("watch:postgres"),
		Evidence: map[string]service.Evidence{"postgres": service.EvidenceChanged},
		Deps:     deps.deps(),
	})
	if len(outcomes) != 1 || outcomes[0].Err != nil {
		t.Fatalf("outcomes = %+v", outcomes)
	}
	if strings.Join(deps.calls, ",") != "stop:api,start:api" {
		t.Errorf("calls = %v", deps.calls)
	}
}

func TestBackgroundUpgradeDeferrals_filters_the_plan(t *testing.T) {
	svcs := map[string]schema.Service{
		"api": {
			Start:       []string{"true"},
			Watch:       []string{"watch:postgres"},
			HealthCheck: &schema.HealthCheck{Command: []string{"probe"}}, // no allow_background
		},
	}
	upPlan := upgrade.UpgradePlan{Actions: []resolver.UpgradeAction{
		{LPs: []genvfile.LockedPackage{{ID: "postgres"}}},
		{LPs: []genvfile.LockedPackage{{ID: "curl"}}},
	}}

	deferred := backgroundUpgradeDeferrals(svcs, upPlan)
	if len(deferred) != 1 || deferred[0] != "postgres" {
		t.Fatalf("deferrals = %v, want [postgres]", deferred)
	}
	filtered := filterUpgradePlanExcluding(upPlan, deferred)
	if len(filtered.Actions) != 1 {
		t.Fatalf("filtered actions = %d, want 1", len(filtered.Actions))
	}
	if filtered.Actions[0].LPs[0].ID != "curl" {
		t.Errorf("remaining action = %+v, want curl", filtered.Actions[0].LPs)
	}
	// Filtering must not mutate the caller's plan.
	if len(upPlan.Actions) != 2 {
		t.Errorf("original plan was mutated: %+v", upPlan.Actions)
	}
}

func TestBackgroundUpgradeDeferrals_drops_whole_action_when_all_ids_deferred(t *testing.T) {
	svcs := map[string]schema.Service{
		"api": {
			Start:       []string{"true"},
			Watch:       []string{"watch:postgres", "watch:redis"},
			HealthCheck: &schema.HealthCheck{Command: []string{"probe"}},
		},
	}
	upPlan := upgrade.UpgradePlan{Actions: []resolver.UpgradeAction{
		{LPs: []genvfile.LockedPackage{{ID: "postgres"}, {ID: "redis"}}},
	}}
	deferred := backgroundUpgradeDeferrals(svcs, upPlan)
	if len(deferred) != 2 {
		t.Fatalf("deferrals = %v, want both packages", deferred)
	}
	if got := filterUpgradePlanExcluding(upPlan, deferred); len(got.Actions) != 0 {
		t.Errorf("actions = %+v, want none", got.Actions)
	}
}

func TestBackgroundUpgradeDeferrals_allows_background_capable_services(t *testing.T) {
	svcs := map[string]schema.Service{
		"api": {
			Start:       []string{"true"},
			Watch:       []string{"watch:postgres"},
			HealthCheck: &schema.HealthCheck{Command: []string{"probe"}, AllowBackground: true},
		},
	}
	upPlan := upgrade.UpgradePlan{Actions: []resolver.UpgradeAction{
		{LPs: []genvfile.LockedPackage{{ID: "postgres"}}},
	}}
	if got := backgroundUpgradeDeferrals(svcs, upPlan); got != nil {
		t.Errorf("deferrals = %v, want none for an allowBackground check", got)
	}
}

func TestBackgroundUpgradeDeferrals_no_services_or_no_plan(t *testing.T) {
	upPlan := upgrade.UpgradePlan{Actions: []resolver.UpgradeAction{
		{LPs: []genvfile.LockedPackage{{ID: "postgres"}}},
	}}
	if got := backgroundUpgradeDeferrals(nil, upPlan); got != nil {
		t.Errorf("deferrals without services = %v", got)
	}
	if got := backgroundUpgradeDeferrals(map[string]schema.Service{"a": {Start: []string{"true"}}}, upgrade.UpgradePlan{}); got != nil {
		t.Errorf("deferrals without actions = %v", got)
	}
}

func TestBackgroundUpgradeDeferrals_survives_unorderable_graph(t *testing.T) {
	svcs := map[string]schema.Service{
		"a": {Start: []string{"true"}, Requires: []string{"b"}, Watch: []string{"watch:postgres"}, HealthCheck: &schema.HealthCheck{Command: []string{"probe"}}},
		"b": {Start: []string{"true"}, Requires: []string{"a"}},
	}
	upPlan := upgrade.UpgradePlan{Actions: []resolver.UpgradeAction{
		{LPs: []genvfile.LockedPackage{{ID: "postgres"}}},
	}}
	if got := backgroundUpgradeDeferrals(svcs, upPlan); got != nil {
		t.Errorf("deferrals = %v, want none for a spec that validate will report", got)
	}
}

func TestFilterUpgradePlanExcluding_without_deferrals_is_identity(t *testing.T) {
	upPlan := upgrade.UpgradePlan{Actions: []resolver.UpgradeAction{
		{LPs: []genvfile.LockedPackage{{ID: "curl"}}},
	}}
	got := filterUpgradePlanExcluding(upPlan, nil)
	if len(got.Actions) != 1 || got.Actions[0].LPs[0].ID != "curl" {
		t.Errorf("plan = %+v, want unchanged", got.Actions)
	}
}
