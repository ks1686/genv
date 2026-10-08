package plan

import (
	"strings"
	"testing"

	"github.com/ks1686/genv/internal/schema"
)

// node builds a service map from name -> requires, so the graph tests read as
// the dependency they are describing.
func node(requires ...string) *schema.Service {
	return &schema.Service{Start: []string{"true"}, Requires: requires}
}

func services(pairs map[string][]string) map[string]*schema.Service {
	out := make(map[string]*schema.Service, len(pairs))
	for name, deps := range pairs {
		out[name] = node(deps...)
	}
	return out
}

func order(p Plan) []string {
	out := make([]string, 0, len(p.Nodes))
	for _, n := range p.Nodes {
		out = append(out, n.Name)
	}
	return out
}

func TestPlan_orders_dependencies_before_dependents(t *testing.T) {
	got, err := Build(services(map[string][]string{
		"web":    {"db", "cache"},
		"db":     nil,
		"cache":  nil,
		"worker": {"web"},
	}))
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	seq := order(got)
	pos := map[string]int{}
	for i, n := range seq {
		pos[n] = i
	}
	if len(seq) != 4 {
		t.Fatalf("plan = %v, want 4 nodes", seq)
	}
	for _, pair := range [][2]string{{"db", "web"}, {"cache", "web"}, {"web", "worker"}} {
		if pos[pair[0]] > pos[pair[1]] {
			t.Errorf("%s must come before %s in %v", pair[0], pair[1], seq)
		}
	}
}

func TestPlan_is_deterministic(t *testing.T) {
	svc := services(map[string][]string{"a": nil, "b": nil, "c": {"a"}, "d": {"b"}})
	first, err := Build(svc)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	for i := 0; i < 20; i++ {
		again, err := Build(svc)
		if err != nil {
			t.Fatalf("Build: %v", err)
		}
		if strings.Join(order(first), ",") != strings.Join(order(again), ",") {
			t.Fatalf("plan order is not stable:\n%v\n%v", order(first), order(again))
		}
	}
}

func TestPlan_detects_cycle_with_path(t *testing.T) {
	_, err := Build(services(map[string][]string{"a": {"b"}, "b": {"c"}, "c": {"a"}}))
	if err == nil {
		t.Fatal("a cycle must be reported, not silently ordered")
	}
	msg := err.Error()
	if !strings.Contains(msg, "cycle") {
		t.Errorf("error should mention a cycle: %v", err)
	}
	// The path must name the services involved, or the user cannot find it.
	for _, want := range []string{"a", "b", "c"} {
		if !strings.Contains(msg, want) {
			t.Errorf("cycle message should name %q: %v", want, msg)
		}
	}
}

func TestPlan_detects_self_dependency(t *testing.T) {
	_, err := Build(services(map[string][]string{"a": {"a"}}))
	if err == nil || !strings.Contains(err.Error(), "itself") {
		t.Fatalf("self-dependency error = %v", err)
	}
}

func TestPlan_reports_unknown_reference(t *testing.T) {
	_, err := Build(services(map[string][]string{"web": {"ghost"}}))
	if err == nil {
		t.Fatal("a requires entry naming no declared service must be reported")
	}
	msg := err.Error()
	if !strings.Contains(msg, "ghost") || !strings.Contains(msg, "web") {
		t.Errorf("error should name both the referent and the referrer: %v", err)
	}
}

func TestPlan_ignores_tombstone_services(t *testing.T) {
	svc := map[string]*schema.Service{
		"web": node("db"),
		"db":  node(),
		// A null tombstone removes the service on this target; requiring it
		// would break every other target that keeps it.
		"gone": nil,
	}
	got, err := Build(svc)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(got.Nodes) != 2 {
		t.Fatalf("tombstone should not become a node: %v", order(got))
	}
}

func TestPlan_node_carries_service_and_policy(t *testing.T) {
	svc := map[string]*schema.Service{
		"api": {
			Start:         []string{"true"},
			Requires:      nil,
			Watch:         []string{"postgres", "postgres"},
			RestartPolicy: schema.RestartPolicyIfRunning,
			HealthCheck:   &schema.HealthCheck{Command: []string{"curl", "-fsS", "localhost/health"}},
		},
	}
	got, err := Build(svc)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(got.Nodes) != 1 {
		t.Fatalf("nodes = %v", order(got))
	}
	n := got.Nodes[0]
	if n.RestartPolicy != schema.RestartPolicyIfRunning {
		t.Errorf("restart policy = %q", n.RestartPolicy)
	}
	if n.HealthCheck == nil || len(n.HealthCheck.Command) != 3 {
		t.Errorf("health check not carried: %+v", n.HealthCheck)
	}
	if len(n.WatchTargets) != 1 || n.WatchTargets[0] != "postgres" {
		t.Errorf("watch targets = %v, want one deduped entry", got)
	}
	if n.RestartPolicy == "" {
		t.Error("restart policy should default rather than stay empty")
	}
}

func TestPlan_empty_graph(t *testing.T) {
	got, err := Build(nil)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(got.Nodes) != 0 {
		t.Errorf("nodes = %v, want none", order(got))
	}
}
