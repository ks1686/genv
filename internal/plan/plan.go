// Package plan turns declared service dependencies into a start/stop order.
//
// Services declare `requires` edges; the plan is a topological order of them.
// Two properties matter and are both tested:
//
//   - Deterministic. Two runs over the same services must produce the same
//     order, so a run log can be compared against a previous one.
//   - Honest about cycles. A cycle is an error naming the services involved,
//     never a partial order that would start services in a sequence nobody
//     declared.
package plan

import (
	"fmt"
	"sort"
	"strings"

	"github.com/ks1686/genv/internal/schema"
)

// Node is one service in a plan.
type Node struct {
	// Name is the service name as declared.
	Name string
	// Service is the declaration, for callers that need the start command.
	Service *schema.Service
	// Requires is the resolved dependency list (names only).
	Requires []string
	// RestartPolicy is normalized: empty has already become "never".
	RestartPolicy string
	// WatchTargets are the distinct watched resource names.
	WatchTargets []string
	// HealthCheck is nil when the service declares none.
	HealthCheck *schema.HealthCheck
}

// Plan is a resolved, ordered set of services.
type Plan struct {
	// Nodes are in start order: every dependency precedes its dependents.
	Nodes []Node
	// StopOrder is the reverse of Nodes, because a service must stop before the
	// services that depend on it.
	StopOrder []string
}

// Names returns the plan's node names in start order.
func (p Plan) Names() []string {
	out := make([]string, 0, len(p.Nodes))
	for _, n := range p.Nodes {
		out = append(out, n.Name)
	}
	return out
}

// Build resolves the service dependency graph into a deterministic order.
//
// A service that requires a name no declared service provides is an error: the
// alternative is to drop the edge and start services in an order the spec did
// not ask for, which fails later and less clearly.
func Build(services map[string]*schema.Service) (Plan, error) {
	// Tombstones (nil) are removals on this target. A service that requires a
	// tombstoned name is not an error here: the removal is already deliberate,
	// and reporting it on every other target would make the spec unportable.
	live := make(map[string]*schema.Service, len(services))
	for name, svc := range services {
		if svc != nil {
			live[name] = svc
		}
	}

	names := make([]string, 0, len(live))
	for name := range live {
		names = append(names, name)
	}
	sort.Strings(names)

	nodes := make(map[string]*Node, len(names))
	edges := make(map[string][]string, len(names))
	for _, name := range names {
		svc := live[name]
		requires := make([]string, 0, len(svc.Requires))
		for _, dep := range svc.Requires {
			if dep == name {
				return Plan{}, fmt.Errorf("service %q requires itself", name)
			}
			if _, ok := live[dep]; !ok {
				return Plan{}, fmt.Errorf("service %q requires %q, which no declared service provides", name, dep)
			}
			requires = append(requires, dep)
		}
		sort.Strings(requires)
		edges[name] = requires
		nodes[name] = &Node{
			Name:          name,
			Service:       svc,
			Requires:      requires,
			RestartPolicy: schema.NormalizeRestartPolicy(svc.RestartPolicy),
			WatchTargets:  schema.WatchTargets(svc.Watch),
			HealthCheck:   svc.HealthCheck,
		}
	}

	ordered, err := topoSort(names, edges)
	if err != nil {
		return Plan{}, err
	}

	plan := Plan{Nodes: make([]Node, 0, len(ordered))}
	for _, name := range ordered {
		plan.Nodes = append(plan.Nodes, *nodes[name])
	}
	plan.StopOrder = make([]string, 0, len(ordered))
	for i := len(ordered) - 1; i >= 0; i-- {
		plan.StopOrder = append(plan.StopOrder, ordered[i])
	}
	return plan, nil
}

// topoSort is Kahn's algorithm with a sorted ready set, which makes the result
// deterministic: ties are broken by name, not by map iteration order.
func topoSort(names []string, edges map[string][]string) ([]string, error) {
	remaining := make(map[string]int, len(names))
	dependents := make(map[string][]string, len(names))
	for _, name := range names {
		remaining[name] = len(edges[name])
		for _, dep := range edges[name] {
			dependents[dep] = append(dependents[dep], name)
		}
	}

	var ready []string
	for _, name := range names {
		if remaining[name] == 0 {
			ready = append(ready, name)
		}
	}
	sort.Strings(ready)

	ordered := make([]string, 0, len(names))
	for len(ready) > 0 {
		next := ready[0]
		ready = ready[1:]
		ordered = append(ordered, next)

		var freed []string
		for _, dependent := range dependents[next] {
			remaining[dependent]--
			if remaining[dependent] == 0 {
				freed = append(freed, dependent)
			}
		}
		if len(freed) > 0 {
			ready = append(ready, freed...)
			sort.Strings(ready)
		}
	}

	if len(ordered) != len(names) {
		return nil, fmt.Errorf("service dependency cycle among: %s", strings.Join(cycleMembers(names, remaining), ", "))
	}
	return ordered, nil
}

// cycleMembers lists the services still blocked when the cycle was detected,
// sorted so the message is stable.
func cycleMembers(names []string, remaining map[string]int) []string {
	var stuck []string
	for _, name := range names {
		if remaining[name] > 0 {
			stuck = append(stuck, name)
		}
	}
	sort.Strings(stuck)
	return stuck
}