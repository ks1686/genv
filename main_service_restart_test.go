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
var errDBDown = errors.New("dependency is down")

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
	// Both are independent, so nothing orders them relative to each other. What
	// must hold is that each service is stopped before it is started. Asserting
	// that per service rather than as adjacent pairs is deliberate: the phase
	// now stops everything before it starts anything, which is still correct
	// for independent services and is what dependent ones need.
	seen := map[string]int{}
	for i, call := range deps.calls {
		kind, name, _ := strings.Cut(call, ":")
		if kind == "start" {
			if stop, ok := seen[name]; !ok || stop > i {
				t.Errorf("call order = %v, %s started without being stopped first", deps.calls, name)
			}
		}
		seen[name] = i
	}
	for _, call := range deps.calls {
		if !strings.HasPrefix(call, "stop:") && !strings.HasPrefix(call, "start:") {
			t.Errorf("unexpected call %q in %v", call, deps.calls)
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

// A restart whose in-flight record could not be written leaves nothing to
// recover from if it is interrupted. The comment claimed the outcome said so;
// it did not. It does now.
// unwritableLockPath returns a lock path that cannot be written: a regular file
// sits where the lock's parent directory would have to be.
func unwritableLockPath(t *testing.T) string {
	t.Helper()
	blocker := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatalf("write blocker: %v", err)
	}
	return filepath.Join(blocker, "genv.lock.json")
}

func TestRestartPhase_reports_a_pending_record_it_could_not_write(t *testing.T) {
	svcs := apiService("watch:postgres")
	outcomes := runRestartPhase(context.Background(), restartPhaseRequest{
		Services:   svcs,
		Evidence:   map[string]service.Evidence{"postgres": service.EvidenceChanged},
		LockPath:   unwritableLockPath(t),
		Deps:       (&recordingDeps{running: map[string]bool{"api": true}}).deps(),
		Background: true,
	})
	if len(outcomes) != 1 {
		t.Fatalf("outcomes = %+v, want one", outcomes)
	}
	if outcomes[0].PendingError == nil {
		t.Fatalf("outcome = %+v, want the failed pending record reported", outcomes[0])
	}
	if outcomes[0].Action != service.ActionRestart {
		t.Errorf("action = %q, want the restart to proceed anyway", outcomes[0].Action)
	}
}

// A service must stop before the services that depend on it, and start after
// them. The coordinator walked decisions in start order and did stop→start per
// service, so restarting both `db` and `api` (where api requires db) took `db`
// down while `api` was still running against it — and plan.StopOrder, which
// exists for exactly this, was never used.
func TestRestartPhase_stops_dependents_before_restarting_dependencies(t *testing.T) {
	dir := t.TempDir()
	deps := &recordingDeps{running: map[string]bool{"db": true, "api": true}}
	svcs := map[string]schema.Service{
		"db":  {Start: []string{"true"}, Watch: []string{"postgres"}},
		"api": {Start: []string{"true"}, Watch: []string{"postgres"}, Requires: []string{"db"}},
	}

	outcomes := runRestartPhase(context.Background(), restartPhaseRequest{
		Services: svcs,
		Evidence: map[string]service.Evidence{"postgres": service.EvidenceChanged},
		LockPath: filepath.Join(dir, "genv.lock.json"),
		Deps:     deps.deps(),
	})

	if len(outcomes) != 2 {
		t.Fatalf("outcomes = %+v, want one per service", outcomes)
	}
	want := "stop:api,stop:db,start:db,start:api"
	if got := strings.Join(deps.calls, ","); got != want {
		t.Errorf("call order = %s\nwant       = %s", got, want)
	}
}

// Three levels deep, so the order cannot be right by accident.
func TestRestartPhase_orders_a_three_level_chain(t *testing.T) {
	dir := t.TempDir()
	deps := &recordingDeps{running: map[string]bool{"db": true, "cache": true, "api": true}}
	svcs := map[string]schema.Service{
		"db":    {Start: []string{"true"}, Watch: []string{"postgres"}},
		"cache": {Start: []string{"true"}, Watch: []string{"postgres"}, Requires: []string{"db"}},
		"api":   {Start: []string{"true"}, Watch: []string{"postgres"}, Requires: []string{"cache"}},
	}

	runRestartPhase(context.Background(), restartPhaseRequest{
		Services: svcs,
		Evidence: map[string]service.Evidence{"postgres": service.EvidenceChanged},
		LockPath: filepath.Join(dir, "genv.lock.json"),
		Deps:     deps.deps(),
	})

	want := "stop:api,stop:cache,stop:db,start:db,start:cache,start:api"
	if got := strings.Join(deps.calls, ","); got != want {
		t.Errorf("call order = %s\nwant       = %s", got, want)
	}
}

// A service being started (never restarted) still belongs in the start phase,
// in dependency order — not interleaved into the stop phase.
func TestRestartPhase_starts_stopped_services_after_dependencies(t *testing.T) {
	dir := t.TempDir()
	// db is running and restarting; api is stopped and has ifRunning.
	deps := &recordingDeps{running: map[string]bool{"db": true, "api": false}}
	apiSvc := schema.Service{Start: []string{"true"}, Watch: []string{"postgres"},
		Requires: []string{"db"}, RestartPolicy: schema.RestartPolicyIfRunning}
	svcs := map[string]schema.Service{
		"db":  {Start: []string{"true"}, Watch: []string{"postgres"}},
		"api": apiSvc,
	}

	runRestartPhase(context.Background(), restartPhaseRequest{
		Services: svcs,
		Evidence: map[string]service.Evidence{"postgres": service.EvidenceChanged},
		LockPath: filepath.Join(dir, "genv.lock.json"),
		Deps:     deps.deps(),
	})

	want := "stop:db,start:db,start:api"
	if got := strings.Join(deps.calls, ","); got != want {
		t.Errorf("call order = %s\nwant       = %s", got, want)
	}
}

// If a dependency fails to come back, its dependents must not be started on
// top of it: a service with no health check would otherwise be reported as
// successfully started while its dependency was down.
func TestRestartPhase_does_not_start_a_dependent_after_its_dependency_failed(t *testing.T) {
	dir := t.TempDir()
	deps := &recordingDeps{running: map[string]bool{"db": true, "api": true}}
	deps.startErr = errDBDown
	svcs := map[string]schema.Service{
		"db":  {Start: []string{"true"}, Watch: []string{"postgres"}},
		"api": {Start: []string{"true"}, Watch: []string{"postgres"}, Requires: []string{"db"}},
	}

	outcomes := runRestartPhase(context.Background(), restartPhaseRequest{
		Services: svcs,
		Evidence: map[string]service.Evidence{"postgres": service.EvidenceChanged},
		LockPath: filepath.Join(dir, "genv.lock.json"),
		Deps:     deps.deps(),
	})

	byService := map[string]restartOutcome{}
	for _, o := range outcomes {
		byService[o.Service] = o
	}
	if o := byService["db"]; o.Err == nil {
		t.Errorf("db outcome = %+v, want the start failure reported", o)
	}
	o := byService["api"]
	if o.Err == nil {
		t.Errorf("api outcome = %+v, want it reported rather than silently started", o)
	}
	if !strings.Contains(o.Err.Error(), "db") {
		t.Errorf("api error = %v, want it to name the dependency that failed", o.Err)
	}
	for _, c := range deps.calls {
		if c == "start:api" {
			t.Error("api was started on top of a failed dependency")
		}
	}
	// Both records stay pending: neither service was confirmed healthy.
	lf, err := genvfile.ReadLock(filepath.Join(dir, "genv.lock.json"))
	if err != nil {
		t.Fatalf("read lock: %v", err)
	}
	if len(lf.PendingActions) != 2 {
		t.Errorf("pending = %+v, want both records kept", lf.PendingActions)
	}
}

// Filtering a service out of a background plan must also drop it from
// StopOrder. The stop phase iterates StopOrder directly, so a stale entry names
// a service the plan no longer contains.
func TestFilterBackgroundServices_rebuilds_stop_order(t *testing.T) {
	svcs := apiService("watch:postgres")
	svc := svcs["api"]
	// No allow_background: api is dropped, so it must not remain in StopOrder.
	svc.HealthCheck = &schema.HealthCheck{Command: []string{"probe"}}
	svcs["api"] = svc
	svcs["db"] = schema.Service{Start: []string{"true"}, Watch: []string{"postgres"}}

	p, err := plan.Build(map[string]*schema.Service{"api": &svc, "db": {Start: []string{"true"}}})
	if err != nil {
		t.Fatalf("plan.Build: %v", err)
	}
	if len(p.StopOrder) != 2 {
		t.Fatalf("unfiltered StopOrder = %v, want both services", p.StopOrder)
	}

	filtered := filterBackgroundServices(p)
	if len(filtered.Nodes) != 1 || filtered.Nodes[0].Name != "db" {
		t.Fatalf("nodes = %+v, want only db", filtered.Nodes)
	}
	if strings.Join(filtered.StopOrder, ",") != "db" {
		t.Errorf("StopOrder = %v, want only the services that survived the filter", filtered.StopOrder)
	}
}
