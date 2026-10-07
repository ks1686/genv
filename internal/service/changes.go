package service

import (
	"fmt"
	"sort"
	"strings"

	"github.com/ks1686/genv/internal/plan"
	"github.com/ks1686/genv/internal/schema"
)

// Evidence is what genv could establish about a resource's change during a run.
//
// The three states are deliberately not two. A run that cannot tell whether a
// package's binary changed has learned nothing, and treating that as "nothing
// changed" leaves a service running against a swapped binary forever.
type Evidence string

const (
	// EvidenceChanged means the installed version demonstrably moved.
	EvidenceChanged Evidence = "changed"
	// EvidenceUnchanged means the upgrade ran and the installed version did
	// not move, so there is nothing to restart for.
	EvidenceUnchanged Evidence = "unchanged"
	// EvidenceUnknown means the version could not be established on one side of
	// the upgrade. Callers must defer rather than assume.
	EvidenceUnknown Evidence = "unknown"
)

// ChangedEvidence compares a package's installed version before and after an
// upgrade.
//
// queryVersion is called once for `before` and once for `after`; callers capture
// the pre-upgrade version and pass a lookup for the post-upgrade read. Anything
// short of two comparable, non-empty versions is EvidenceUnknown — including a
// nil query function, which is a programming error rather than a data state,
// but which must not be allowed to report "unchanged".
func ChangedEvidence(before, after schema.Package, queryVersion func(string) (string, error)) Evidence {
	if queryVersion == nil {
		return EvidenceUnknown
	}
	beforeVersion, err := queryVersion(before.ID)
	if err != nil {
		return EvidenceUnknown
	}
	afterVersion, err := queryVersion(after.ID)
	if err != nil {
		return EvidenceUnknown
	}
	if beforeVersion == "" || afterVersion == "" {
		// Many managers never report a version. That is not evidence of
		// sameness; it is absence of evidence.
		return EvidenceUnknown
	}
	if beforeVersion != afterVersion {
		return EvidenceChanged
	}
	return EvidenceUnchanged
}

// Restart actions a decision can carry.
const (
	// ActionRestart stops and starts a service that was running.
	ActionRestart = "restart"
	// ActionStart starts a service that was stopped, under ifRunning.
	ActionStart = "start"
	// ActionSkip means genv deliberately left the service alone.
	ActionSkip = "skip"
	// ActionDefer means genv could not establish whether a restart was needed
	// and declined to guess. The pending action is kept for a human.
	ActionDefer = "defer"
)

// RestartDecision is one service's resolved fate for one run.
type RestartDecision struct {
	// Service is the declared service name.
	Service string
	// Action is one of the Action* constants.
	Action string
	// Reason explains the decision in one sentence, for humans.
	Reason string
	// Triggers lists the watched resources that motivated the decision.
	Triggers []string
	// Uncertain lists watched resources whose change could not be established.
	Uncertain []string
}

// WatchPrefix marks a watch entry as referring to a package or file rather than
// to another service. `watch: [postgres]` and `watch: ["services:db"]` are
// equivalent; the explicit prefix exists so a package and a service may share a
// name without ambiguity.
const WatchPrefix = "watch:"

// PlanServiceRestarts decides, per service, whether a run's package changes
// require a restart.
//
// One decision is produced per service per run: several changed resources that
// point at the same service coalesce into a single restart, because restarting
// twice is worse than restarting once. Decisions come back in plan order, so a
// dependent service is restarted after the service it depends on.
//
// isRunning reports whether a service is currently running. It is consulted
// only after genv has decided a change is relevant; an unknown running state
// for a service with nothing to do never causes a probe.
func PlanServiceRestarts(p *plan.Plan, evidence map[string]Evidence, isRunning func(string) bool) []RestartDecision {
	if p == nil || len(p.Nodes) == 0 {
		return nil
	}
	decisions := make([]RestartDecision, 0, len(p.Nodes))
	for _, node := range p.Nodes {
		var triggers, uncertain []string
		for _, watch := range node.WatchTargets {
			resource := strings.TrimPrefix(watch, WatchPrefix)
			got, tracked := evidence[resource]
			if !tracked {
				// This run did not touch the resource at all. That is not
				// uncertainty; it is simply not a reason to act.
				continue
			}
			switch got {
			case EvidenceChanged:
				triggers = append(triggers, resource)
			case EvidenceUnknown:
				uncertain = append(uncertain, resource)
			case EvidenceUnchanged:
				// Explicitly no reason to act.
			default:
				uncertain = append(uncertain, resource)
			}
		}

		if len(triggers) == 0 && len(uncertain) == 0 {
			continue
		}

		sort.Strings(triggers)
		sort.Strings(uncertain)

		d := RestartDecision{
			Service:   node.Name,
			Triggers:  triggers,
			Uncertain: uncertain,
		}

		if len(triggers) == 0 {
			d.Action = ActionDefer
			d.Reason = fmt.Sprintf("cannot tell whether %s changed, so %q was left alone rather than restarted on a guess",
				strings.Join(uncertain, ", "), node.Name)
			decisions = append(decisions, d)
			continue
		}

		running := isRunning != nil && isRunning(node.Name)
		switch {
		case running:
			d.Action = ActionRestart
			d.Reason = fmt.Sprintf("%s changed", strings.Join(triggers, ", "))
		case node.RestartPolicy == schema.RestartPolicyIfRunning:
			d.Action = ActionStart
			d.Reason = fmt.Sprintf("%s changed and restart_policy is ifRunning", strings.Join(triggers, ", "))
		default:
			d.Action = ActionSkip
			d.Reason = fmt.Sprintf("%s changed but %q is not running and restart_policy is never",
				strings.Join(triggers, ", "), node.Name)
		}
		if len(uncertain) > 0 {
			d.Reason += fmt.Sprintf("; could not determine %s", strings.Join(uncertain, ", "))
		}
		decisions = append(decisions, d)
	}
	if len(decisions) == 0 {
		return nil
	}
	return decisions
}

// TriggerResource maps a watch entry to the resource key used in the evidence
// map, so the two sides of the coordinator cannot disagree about naming.
func TriggerResource(watch string) string {
	return strings.TrimPrefix(strings.TrimSpace(watch), WatchPrefix)
}
