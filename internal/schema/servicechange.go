package schema

import (
	"fmt"
	"strings"
	"time"
)

// Service dependency and change fields (schemaVersion 10).
//
// `requires` and `watch` are deliberately different relationships and are not
// interchangeable:
//
//   - requires is an ordering constraint between services: this service starts
//     after those and stops before them.
//   - watch is a change trigger: when one of these resources changes, this
//     service may need to be restarted.
//
// Conflating them would make genv restart a service because an unrelated service
// was touched, or refuse to start one because its trigger package was upgraded.

// RestartPolicy decides what genv does with a service that was not running when
// its watched resource changed.
const (
	// RestartPolicyNever (the default) leaves a stopped service stopped.
	RestartPolicyNever = "never"
	// RestartPolicyIfRunning starts the service as part of the change, because
	// the change is what it is waiting for.
	RestartPolicyIfRunning = "ifRunning"
)

// KnownRestartPolicies is the set of accepted restart_policy values.
var KnownRestartPolicies = map[string]bool{
	RestartPolicyNever:     true,
	RestartPolicyIfRunning: true,
}

// DefaultHealthTimeout and DefaultHealthInterval apply when a HealthCheck
// leaves them empty.
const (
	DefaultHealthTimeout  = 30 * time.Second
	DefaultHealthInterval = time.Second
)

// HealthCheck is an optional readiness probe run after an authorized start or
// restart.
//
// It is never run by `genv status`, `genv apply --dry-run`, `genv upgrade
// --dry-run`, or any other planning path: a probe is a side effect, and a plan
// that has one is not a plan.
type HealthCheck struct {
	// Command is the probe argv. Non-zero exit or timeout means not ready.
	Command []string `json:"command"`
	// Timeout bounds the whole readiness wait, e.g. "30s". Empty means 30s.
	Timeout string `json:"timeout,omitempty"`
	// Interval is the gap between probe attempts, e.g. "1s". Empty means 1s.
	Interval string `json:"interval,omitempty"`
	// AllowBackground permits the unattended updates worker to run this probe.
	// False by default: a check that needs a terminal or a human must not fire
	// from a timer.
	AllowBackground bool `json:"allow_background,omitempty"`
}

// serviceFieldsV10 are the service keys that exist only from schemaVersion 10.
// They are named explicitly so validation can refuse them on v1-v9 with a
// useful message instead of reporting a generic unknown field.
var serviceFieldsV10 = []string{"requires", "watch", "restart_policy", "health_check"}

// HasV10ServiceFields reports whether a service declares any v10-only field.
// v1-v9 validation uses it to refuse rather than silently ignore.
func HasV10ServiceFields(svc *Service) bool {
	if svc == nil {
		return false
	}
	return len(svc.Requires) > 0 || len(svc.Watch) > 0 ||
		svc.RestartPolicy != "" || svc.HealthCheck != nil
}

// NormalizeRestartPolicy returns the effective policy, where empty means never.
func NormalizeRestartPolicy(policy string) string {
	if policy == "" {
		return RestartPolicyNever
	}
	return policy
}

// WatchTargets returns the distinct watch entries in declaration order.
//
// Entries are trimmed but never case-folded: package ids are case-sensitive on
// several managers, so folding them would match the wrong package.
func WatchTargets(watch []string) []string {
	if len(watch) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(watch))
	out := make([]string, 0, len(watch))
	for _, w := range watch {
		key := strings.TrimSpace(w)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, key)
	}
	return out
}

// HealthTimeout returns the probe timeout, defaulting to 30s.
func (h *HealthCheck) HealthTimeout() time.Duration {
	if h == nil {
		return 0
	}
	if d, err := time.ParseDuration(h.Timeout); err == nil && d > 0 {
		return d
	}
	return DefaultHealthTimeout
}

// HealthInterval returns the gap between probe attempts, defaulting to 1s.
func (h *HealthCheck) HealthInterval() time.Duration {
	if h == nil {
		return 0
	}
	if d, err := time.ParseDuration(h.Interval); err == nil && d > 0 {
		return d
	}
	return DefaultHealthInterval
}

// ValidateServiceV10Fields checks the v10-only service fields.
//
// The rules are about meaning rather than shape: a health check with no command
// cannot be run, and a timeout of "0s" is a readiness check that can never
// succeed while looking configured.
func ValidateServiceV10Fields(name string, svc *Service, errs []ValidationError, fieldPrefix string, positions map[string]Position) []ValidationError {
	if svc == nil {
		return errs
	}
	add := func(field, message string) {
		errs = append(errs, ValidationError{
			Position: positions[field],
			Field:    field,
			Message:  message,
		})
	}

	seen := make(map[string]bool, len(svc.Requires))
	for _, dep := range svc.Requires {
		field := fieldPrefix + ".requires[" + dep + "]"
		switch {
		case strings.TrimSpace(dep) == "":
			add(field, "requires entry must name a service")
		case dep == name:
			add(field, fmt.Sprintf("service %q cannot require itself", name))
		case seen[dep]:
			add(field, "duplicate requires entry "+dep)
		default:
			seen[dep] = true
		}
	}

	seenWatch := make(map[string]bool, len(svc.Watch))
	for _, w := range svc.Watch {
		field := fieldPrefix + ".watch[" + w + "]"
		switch {
		case strings.TrimSpace(w) == "":
			add(field, "watch entry must name a package, file destination, or service")
		case seenWatch[w]:
			add(field, "duplicate watch entry "+w)
		default:
			seenWatch[w] = true
		}
	}

	if svc.RestartPolicy != "" && !KnownRestartPolicies[svc.RestartPolicy] {
		add(fieldPrefix+".restart_policy", "restart_policy must be one of: never, ifRunning")
	}

	if hc := svc.HealthCheck; hc != nil {
		if len(hc.Command) == 0 {
			add(fieldPrefix+".health_check.command", "health_check requires a command")
		}
		if msg := positiveDurationMessage(hc.Timeout); msg != "" {
			add(fieldPrefix+".health_check.timeout", msg)
		}
		if msg := positiveDurationMessage(hc.Interval); msg != "" {
			add(fieldPrefix+".health_check.interval", msg)
		}
	}
	return errs
}

// positiveDurationMessage returns a validation message for a duration that is
// unparseable or not strictly positive, or "" when it is fine.
func positiveDurationMessage(value string) string {
	if value == "" {
		return ""
	}
	d, err := time.ParseDuration(value)
	if err != nil {
		return "invalid duration " + value
	}
	if d <= 0 {
		return "duration must be greater than zero"
	}
	return ""
}
