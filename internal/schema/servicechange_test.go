package schema

import (
	"strings"
	"testing"
)

func mustValidate(t *testing.T, doc string) []ValidationError {
	t.Helper()
	_, errs, err := ParseAndValidate([]byte(doc))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return errs
}

func TestValidateService_v10_fields_require_v10(t *testing.T) {
	// Each v10-only field must be refused on v8 rather than silently ignored:
	// a user who believes a service restarts on package change must not have
	// that belief quietly discarded.
	cases := []struct {
		name string
		body string
		want string
	}{
		{"requires", `"requires": ["db"]`, `"requires"`},
		{"watch", `"watch": ["postgres"]`, `"watch"`},
		{"restart_policy", `"restart_policy": "ifRunning"`, `"restart_policy"`},
		{"health_check", `"health_check": {"command": ["true"]}`, `"health_check"`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			doc := `{"schemaVersion":"8","targets":{"macos":{"services":{"api":{"start":["true"],` + tc.body + `}}}}}`
			errs := mustValidate(t, doc)
			if len(errs) == 0 {
				t.Fatalf("v8 spec with %s should be refused", tc.name)
			}
			joined := errErrorText(errs)
			if !strings.Contains(joined, tc.want) {
				t.Errorf("errors should mention %s:\n%s", tc.want, joined)
			}
			if !strings.Contains(joined, Version10) {
				t.Errorf("error should name the required version %q:\n%s", Version10, joined)
			}
		})
	}
}

func TestValidateService_v10_fields_accepted_on_v10(t *testing.T) {
	doc := `{"schemaVersion":"10","targets":{"macos":{"services":{
	  "api":{"start":["true"],"requires":["db"],"watch":["postgres"],"restart_policy":"ifRunning",
	         "health_check":{"command":["curl","-fsS","localhost:8080/health"],"timeout":"10s","interval":"500ms","allow_background":true}},
	  "db":{"start":["true"]}
	}}}}`
	if errs := mustValidate(t, doc); len(errs) > 0 {
		t.Fatalf("valid v10 service fields rejected: %s", errErrorText(errs))
	}
}

func TestValidateService_rejects_bad_v10_values(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string
	}{
		{"self require", `"requires": ["api"]`, "cannot require itself"},
		{"empty require", `"requires": [""]`, "must name a service"},
		{"duplicate require", `"requires": ["db","db"]`, "duplicate requires"},
		{"empty watch", `"watch": [" "]`, "must name a package"},
		{"duplicate watch", `"watch": ["postgres","postgres"]`, "duplicate watch"},
		{"bad policy", `"restart_policy": "always"`, "restart_policy must be one of"},
		{"health without command", `"health_check":{"timeout":"5s"}`, "health_check requires a command"},
		{"bad timeout", `"health_check":{"command":["true"],"timeout":"soon"}`, "invalid duration"},
		{"zero timeout", `"health_check":{"command":["true"],"timeout":"0s"}`, "greater than zero"},
		{"negative interval", `"health_check":{"command":["true"],"interval":"-1s"}`, "greater than zero"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			doc := `{"schemaVersion":"10","targets":{"macos":{"services":{"api":{"start":["true"],` + tc.body + `}}}}}`
			errs := mustValidate(t, doc)
			if len(errs) == 0 {
				t.Fatalf("expected a validation error for %s", tc.name)
			}
			if joined := errErrorText(errs); !strings.Contains(joined, tc.want) {
				t.Errorf("errors should mention %q:\n%s", tc.want, joined)
			}
		})
	}
}

func TestValidateService_unknown_health_check_field_is_rejected(t *testing.T) {
	doc := `{"schemaVersion":"10","targets":{"macos":{"services":{"api":{"start":["true"],
	  "health_check":{"command":["true"],"retries":3}}}}}}`
	errs := mustValidate(t, doc)
	if joined := errErrorText(errs); !strings.Contains(joined, "retries") {
		t.Errorf("unknown health_check field should be reported:\n%s", joined)
	}
}

func TestHealthCheck_defaults(t *testing.T) {
	if got := (*HealthCheck)(nil).HealthTimeout(); got != 0 {
		t.Errorf("nil HealthCheck timeout = %v, want 0", got)
	}
	hc := &HealthCheck{Command: []string{"true"}}
	if got := hc.HealthTimeout(); got != DefaultHealthTimeout {
		t.Errorf("timeout = %v, want the %v default", got, DefaultHealthTimeout)
	}
	if got := hc.HealthInterval(); got != DefaultHealthInterval {
		t.Errorf("interval = %v, want the %v default", got, DefaultHealthInterval)
	}
	hc.Timeout, hc.Interval = "5s", "250ms"
	if got := hc.HealthTimeout(); got.String() != "5s" {
		t.Errorf("timeout = %v, want 5s", got)
	}
	if got := hc.HealthInterval(); got.String() != "250ms" {
		t.Errorf("interval = %v, want 250ms", got)
	}
}

func TestNormalizeRestartPolicy(t *testing.T) {
	if got := NormalizeRestartPolicy(""); got != RestartPolicyNever {
		t.Errorf("empty policy = %q, want %q", got, RestartPolicyNever)
	}
	if got := NormalizeRestartPolicy(RestartPolicyIfRunning); got != RestartPolicyIfRunning {
		t.Errorf("policy = %q", got)
	}
}

func TestWatchTargets_trims_but_preserves_case(t *testing.T) {
	// Package ids are case-sensitive on several managers; folding them would
	// attach a restart to the wrong package.
	got := WatchTargets([]string{" Postgres ", "postgres", "", "postgresql", "postgres"})
	want := []string{"Postgres", "postgres", "postgresql"}
	if len(got) != len(want) {
		t.Fatalf("WatchTargets = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("WatchTargets = %v, want %v", got, want)
		}
	}
	if got := WatchTargets(nil); got != nil {
		t.Errorf("WatchTargets(nil) = %v, want nil", got)
	}
}

func TestHasV10ServiceFields(t *testing.T) {
	if HasV10ServiceFields(nil) {
		t.Error("nil service reports v10 fields")
	}
	if HasV10ServiceFields(&Service{Start: []string{"true"}}) {
		t.Error("a plain service reports v10 fields")
	}
	if !HasV10ServiceFields(&Service{Watch: []string{"postgres"}}) {
		t.Error("watch should count as a v10 field")
	}
	if !HasV10ServiceFields(&Service{HealthCheck: &HealthCheck{Command: []string{"true"}}}) {
		t.Error("health_check should count as a v10 field")
	}
}

func errErrorText(errs []ValidationError) string {
	var b strings.Builder
	for _, e := range errs {
		b.WriteString(e.Field)
		b.WriteString(": ")
		b.WriteString(e.Message)
		b.WriteString("\n")
	}
	return b.String()
}
