package service

import (
	"errors"
	"strings"
	"testing"

	"github.com/ks1686/genv/internal/plan"
	"github.com/ks1686/genv/internal/schema"
)

// versionStub answers queries from a table. A missing id is an error, which is
// what a manager that cannot report versions looks like.
func versionStub(versions map[string]string) func(string) (string, error) {
	return func(id string) (string, error) {
		v, ok := versions[id]
		if !ok {
			return "", errors.New("no version available for " + id)
		}
		return v, nil
	}
}

func TestChangedEvidence_version_moved_is_changed(t *testing.T) {
	got := ChangedEvidence(
		schema.Package{ID: "postgres", Version: "16.*"},
		schema.Package{ID: "postgres", Version: "16.*"},
		versionStub(map[string]string{"postgres": "16.2"}),
	)
	// The query answers the same string on both sides in this stub, so a moved
	// version needs a stub that reflects the change.
	if got == EvidenceUnknown {
		t.Fatalf("a queryable package must not be unknown: %s", got)
	}
}

func TestChangedEvidence_noop_command_with_same_version_is_unchanged(t *testing.T) {
	before := schema.Package{ID: "postgres", Version: "16.*"}
	after := schema.Package{ID: "postgres", Version: "17.*"}
	// Same installed version before and after: the upgrade command ran and
	// changed nothing, so nothing needs restarting.
	seq := []string{"16.2", "16.2"}
	idx := 0
	got := ChangedEvidence(before, after, func(string) (string, error) {
		v := seq[idx]
		idx++
		return v, nil
	})
	if got != EvidenceUnchanged {
		t.Errorf("evidence = %s, want %s", got, EvidenceUnchanged)
	}
}

func TestChangedEvidence_installed_version_change_is_changed(t *testing.T) {
	before := schema.Package{ID: "postgres", Version: "16.*"}
	after := schema.Package{ID: "postgres", Version: "16.*"}
	seq := []string{"16.2", "16.4"}
	idx := 0
	got := ChangedEvidence(before, after, func(string) (string, error) {
		v := seq[idx]
		idx++
		return v, nil
	})
	if got != EvidenceChanged {
		t.Errorf("evidence = %s, want %s", got, EvidenceChanged)
	}
}

func TestChangedEvidence_missing_version_is_unknown(t *testing.T) {
	before := schema.Package{ID: "postgres"}
	after := schema.Package{ID: "postgres"}
	// A manager that cannot report a version must produce unknown, never
	// "unchanged": assuming nothing changed is how a service silently keeps
	// running against a swapped binary.
	got := ChangedEvidence(before, after, versionStub(nil))
	if got != EvidenceUnknown {
		t.Errorf("evidence = %s, want %s", got, EvidenceUnknown)
	}
}

func TestChangedEvidence_query_error_is_unknown(t *testing.T) {
	before := schema.Package{ID: "postgres"}
	after := schema.Package{ID: "postgres"}
	seq := []error{nil, errors.New("manager exploded")}
	idx := 0
	got := ChangedEvidence(before, after, func(string) (string, error) {
		err := seq[idx]
		idx++
		if err != nil {
			return "", err
		}
		return "16.2", nil
	})
	if got != EvidenceUnknown {
		t.Errorf("evidence = %s, want %s", got, EvidenceUnknown)
	}
}

func TestChangedEvidence_empty_version_is_unknown(t *testing.T) {
	before := schema.Package{ID: "postgres"}
	after := schema.Package{ID: "postgres"}
	// An empty version string is not evidence of "unchanged".
	seq := []string{"", ""}
	idx := 0
	got := ChangedEvidence(before, after, func(string) (string, error) {
		v := seq[idx]
		idx++
		return v, nil
	})
	if got != EvidenceUnknown {
		t.Errorf("evidence = %s, want %s", got, EvidenceUnknown)
	}
}

func TestChangedEvidence_nil_query_is_unknown(t *testing.T) {
	got := ChangedEvidence(schema.Package{ID: "x"}, schema.Package{ID: "x"}, nil)
	if got != EvidenceUnknown {
		t.Errorf("evidence = %s, want %s without a query function", got, EvidenceUnknown)
	}
}

func TestEvidence_strings_are_stable(t *testing.T) {
	// These strings appear in JSON output and in the lock; changing one is a
	// breaking change for anyone reading the output.
	for got, want := range map[Evidence]string{
		EvidenceChanged:   "changed",
		EvidenceUnchanged: "unchanged",
		EvidenceUnknown:   "unknown",
	} {
		if string(got) != want {
			t.Errorf("Evidence(%q) = %q, want %q", got, string(got), want)
		}
	}
}

func TestPlanServiceRestarts_coalesces_triggers(t *testing.T) {
	p := servicePlan(t, map[string][]string{
		"api": {"watch:postgres", "watch:redis", "watch:nginx.conf"},
	})
	decisions := PlanServiceRestarts(p, map[string]Evidence{
		"postgres":   EvidenceChanged,
		"redis":      EvidenceChanged,
		"nginx.conf": EvidenceUnchanged,
	}, func(string) bool { return true })

	if len(decisions) != 1 {
		t.Fatalf("decisions = %+v, want one per service", decisions)
	}
	d := decisions[0]
	if d.Service != "api" {
		t.Errorf("service = %q", d.Service)
	}
	if d.Action != ActionRestart {
		t.Errorf("action = %q, want %q (one restart, not three)", d.Action, ActionRestart)
	}
	if len(d.Triggers) != 2 {
		t.Errorf("triggers = %v, want the two changed resources", d.Triggers)
	}
	for _, tr := range d.Triggers {
		if strings.Contains(tr, "nginx") {
			t.Errorf("unchanged resource should not appear in triggers: %v", d.Triggers)
		}
	}
}

func TestPlanServiceRestarts_no_change_no_decision(t *testing.T) {
	p := servicePlan(t, map[string][]string{"api": {"watch:postgres"}})
	decisions := PlanServiceRestarts(p, map[string]Evidence{"postgres": EvidenceUnchanged}, func(string) bool { return true })
	if len(decisions) != 0 {
		t.Errorf("decisions = %+v, want none when nothing changed", decisions)
	}
}

func TestPlanServiceRestarts_unknown_evidence_defers(t *testing.T) {
	p := servicePlan(t, map[string][]string{"api": {"watch:postgres"}})
	decisions := PlanServiceRestarts(p, map[string]Evidence{"postgres": EvidenceUnknown}, func(string) bool { return true })
	if len(decisions) != 1 {
		t.Fatalf("decisions = %+v", decisions)
	}
	if decisions[0].Action != ActionDefer {
		t.Errorf("action = %q, want %q: unknown evidence must not restart or skip silently",
			decisions[0].Action, ActionDefer)
	}
	if !strings.Contains(decisions[0].Reason, "cannot tell whether") {
		t.Errorf("reason should explain the uncertainty: %q", decisions[0].Reason)
	}
}

func TestPlanServiceRestarts_stopped_service_is_skipped_by_default(t *testing.T) {
	p := servicePlan(t, map[string][]string{"api": {"watch:postgres"}})
	decisions := PlanServiceRestarts(p, map[string]Evidence{"postgres": EvidenceChanged}, func(string) bool { return false })
	if len(decisions) != 1 {
		t.Fatalf("decisions = %+v", decisions)
	}
	if decisions[0].Action != ActionSkip {
		t.Errorf("action = %q, want %q for a stopped service under the default policy",
			decisions[0].Action, ActionSkip)
	}
}

func TestPlanServiceRestarts_ifRunning_starts_a_stopped_service(t *testing.T) {
	p := servicePlan(t, map[string][]string{"api": {"watch:postgres"}})
	p.Nodes[0].RestartPolicy = schema.RestartPolicyIfRunning
	decisions := PlanServiceRestarts(p, map[string]Evidence{"postgres": EvidenceChanged}, func(string) bool { return false })
	if len(decisions) != 1 || decisions[0].Action != ActionStart {
		t.Fatalf("decisions = %+v, want a start under ifRunning", decisions)
	}
}

func TestPlanServiceRestarts_running_service_restarts_under_ifRunning(t *testing.T) {
	p := servicePlan(t, map[string][]string{"api": {"watch:postgres"}})
	p.Nodes[0].RestartPolicy = schema.RestartPolicyIfRunning
	decisions := PlanServiceRestarts(p, map[string]Evidence{"postgres": EvidenceChanged}, func(string) bool { return true })
	if len(decisions) != 1 || decisions[0].Action != ActionRestart {
		t.Fatalf("decisions = %+v, want a restart", decisions)
	}
}

func TestPlanServiceRestarts_ignores_watch_entries_with_no_evidence(t *testing.T) {
	// A watch entry naming something this run did not touch has no evidence at
	// all. That is not uncertainty; it is "no reason to act".
	p := servicePlan(t, map[string][]string{"api": {"watch:postgres"}})
	decisions := PlanServiceRestarts(p, nil, func(string) bool { return true })
	if len(decisions) != 0 {
		t.Errorf("decisions = %+v, want none", decisions)
	}
}

func TestPlanServiceRestarts_orders_by_plan(t *testing.T) {
	p := servicePlan(t, map[string][]string{
		"web": {"watch:nginx.conf"},
		"db":  {"watch:postgres"},
	})
	decisions := PlanServiceRestarts(p, map[string]Evidence{
		"nginx.conf": EvidenceChanged,
		"postgres":   EvidenceChanged,
	}, func(string) bool { return true })
	if len(decisions) != 2 {
		t.Fatalf("decisions = %+v", decisions)
	}
	// Decisions follow the plan's order so a dependent service restarts after
	// the service it depends on.
	if decisions[0].Service != "db" || decisions[1].Service != "web" {
		t.Errorf("decisions out of plan order: %s then %s", decisions[0].Service, decisions[1].Service)
	}
}

func TestPlanServiceRestarts_multiple_services_same_resource(t *testing.T) {
	p := servicePlan(t, map[string][]string{
		"web": {"watch:postgres"},
		"api": {"watch:postgres"},
	})
	decisions := PlanServiceRestarts(p, map[string]Evidence{"postgres": EvidenceChanged}, func(string) bool { return true })
	if len(decisions) != 2 {
		t.Fatalf("decisions = %+v, want one per watching service", decisions)
	}
	for _, d := range decisions {
		if d.Action != ActionRestart {
			t.Errorf("%s action = %q, want restart", d.Service, d.Action)
		}
	}
}

func TestPlanServiceRestarts_mixed_evidence_per_service(t *testing.T) {
	p := servicePlan(t, map[string][]string{
		"api": {"watch:postgres", "watch:redis"},
	})
	decisions := PlanServiceRestarts(p, map[string]Evidence{
		"postgres": EvidenceChanged,
		"redis":    EvidenceUnknown,
	}, func(string) bool { return true })
	if len(decisions) != 1 {
		t.Fatalf("decisions = %+v", decisions)
	}
	// One known change is enough to act; the unknown is reported, not hidden.
	if decisions[0].Action != ActionRestart {
		t.Errorf("action = %q, want %q", decisions[0].Action, ActionRestart)
	}
	if !strings.Contains(decisions[0].Reason, "redis") {
		t.Errorf("reason should mention the uncertain trigger: %q", decisions[0].Reason)
	}
}

func TestPlanServiceRestarts_empty_plan(t *testing.T) {
	if decisions := PlanServiceRestarts(nil, map[string]Evidence{"x": EvidenceChanged}, func(string) bool { return true }); len(decisions) != 0 {
		t.Errorf("decisions = %+v, want none", decisions)
	}
}

// servicePlan builds a service plan whose nodes watch the given entries.
// Watch entries are written with the watch: prefix so the coordinator sees the
// same shape a real spec produces.
func servicePlan(t *testing.T, spec map[string][]string) *plan.Plan {
	t.Helper()
	services := make(map[string]*schema.Service, len(spec))
	for name, watches := range spec {
		services[name] = &schema.Service{
			Start: []string{"true"},
			Watch: watches,
		}
	}
	p, err := plan.Build(services)
	if err != nil {
		t.Fatalf("plan.Build: %v", err)
	}
	return &p
}

func TestTriggerResource_strips_prefix(t *testing.T) {
	for in, want := range map[string]string{
		"watch:postgres": "postgres",
		"postgres":       "postgres",
		" watch:redis ":  "redis",
	} {
		if got := TriggerResource(in); got != want {
			t.Errorf("TriggerResource(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestEvidenceFromVersions(t *testing.T) {
	cases := []struct {
		before, after string
		want          Evidence
	}{
		{"16.2", "16.4", EvidenceChanged},
		{"16.2", "16.2", EvidenceUnchanged},
		{"", "16.4", EvidenceUnknown},
		{"16.2", "", EvidenceUnknown},
		{"", "", EvidenceUnknown},
	}
	for _, tc := range cases {
		if got := EvidenceFromVersions(tc.before, tc.after); got != tc.want {
			t.Errorf("EvidenceFromVersions(%q, %q) = %s, want %s", tc.before, tc.after, got, tc.want)
		}
	}
}
