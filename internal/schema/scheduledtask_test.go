package schema

import (
	"strings"
	"testing"
)

// #212: a service could not declare a Windows Task Scheduler task, so Windows
// users had to hand-write a `start` command or carry a launchd/systemd
// template that does not apply to them. These cover the spec-level contract.
func TestValidateService_ScheduledTaskAcceptsValidSpec(t *testing.T) {
	svcs := map[string]Service{
		"acme": {
			ScheduledTask: &ScheduledTaskSpec{
				Action:  `C:\Program Files\Acme\agent.exe`,
				Trigger: "daily",
				At:      "09:30",
			},
		},
	}
	if errs := validateService("acme", svcs["acme"], "services"); len(errs) != 0 {
		t.Fatalf("unexpected errors: %+v", errs)
	}
}

func TestValidateService_ScheduledTaskCountsAsADeclaration(t *testing.T) {
	// No `start` command: a scheduled task is the start command.
	svc := Service{ScheduledTask: &ScheduledTaskSpec{Action: `C:\a.exe`}}
	if errs := validateService("a", svc, "services"); len(errs) != 0 {
		t.Fatalf("scheduled_task should satisfy the start requirement: %+v", errs)
	}
}

func TestValidateService_ScheduledTaskErrors(t *testing.T) {
	cases := []struct {
		name string
		spec ScheduledTaskSpec
		want string
	}{
		{"no action", ScheduledTaskSpec{Trigger: "logon"}, "action"},
		{"relative action", ScheduledTaskSpec{Action: "agent.exe"}, "absolute"},
		{"unknown trigger", ScheduledTaskSpec{Action: `C:\a.exe`, Trigger: "hourly"}, "trigger"},
		{"daily without at", ScheduledTaskSpec{Action: `C:\a.exe`, Trigger: "daily"}, "at"},
		{"daily with bad at", ScheduledTaskSpec{Action: `C:\a.exe`, Trigger: "daily", At: "9:30"}, "at"},
		{"daily with 24h", ScheduledTaskSpec{Action: `C:\a.exe`, Trigger: "daily", At: "24:00"}, "at"},
		{"weekly without day", ScheduledTaskSpec{Action: `C:\a.exe`, Trigger: "weekly", At: "01:00"}, "day_of_week"},
		{"weekly with bad day", ScheduledTaskSpec{Action: `C:\a.exe`, Trigger: "weekly", At: "01:00", DayOfWeek: "someday"}, "day_of_week"},
		{"bad principal", ScheduledTaskSpec{Action: `C:\a.exe`, Principal: "root"}, "principal"},
		{"bad retry interval", ScheduledTaskSpec{Action: `C:\a.exe`, RestartOnFailure: true, RetryInterval: "10m"}, "retry_interval"},
		{"at on logon trigger", ScheduledTaskSpec{Action: `C:\a.exe`, Trigger: "logon", At: "09:00"}, "at"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := Service{ScheduledTask: &tc.spec}
			errs := validateService("a", svc, "services")
			if len(errs) == 0 {
				t.Fatalf("no errors for %s", tc.name)
			}
			joined := strings.ToLower(strings.Join(errorMessages(errs), " | "))
			if !strings.Contains(joined, strings.ToLower(tc.want)) {
				t.Errorf("errors = %v, want one mentioning %q", errorMessages(errs), tc.want)
			}
		})
	}
}

func errorMessages(errs []ValidationError) []string {
	var out []string
	for _, e := range errs {
		out = append(out, e.Message)
	}
	return out
}

// The default trigger is what most people want on a desktop, so an empty
// trigger must behave as "logon" rather than fail validation.
func TestValidateService_ScheduledTaskDefaults(t *testing.T) {
	svc := Service{ScheduledTask: &ScheduledTaskSpec{Action: `C:\a.exe`}}
	if errs := validateService("a", svc, "services"); len(errs) != 0 {
		t.Fatalf("an omitted trigger should default: %+v", errs)
	}
	if got := EffectiveScheduledTask(svc).Trigger; got != ScheduledTaskTriggerLogon {
		t.Errorf("default trigger = %q, want %q", got, ScheduledTaskTriggerLogon)
	}
	if got := EffectiveScheduledTask(svc).Principal; got != ScheduledTaskPrincipalUser {
		t.Errorf("default principal = %q, want %q", got, ScheduledTaskPrincipalUser)
	}
}

func TestServiceDeclaresScheduledTask(t *testing.T) {
	if (Service{}).DeclaresScheduledTask() {
		t.Error("empty service must not declare a scheduled task")
	}
	if (Service{ScheduledTask: &ScheduledTaskSpec{}}).DeclaresScheduledTask() {
		t.Error("empty action must not count as declared")
	}
	if !(Service{ScheduledTask: &ScheduledTaskSpec{Action: `C:\a.exe`}}).DeclaresScheduledTask() {
		t.Error("a service with an action declares a scheduled task")
	}
}

// A scheduled task is Windows-specific, so declaring one alongside a launchd
// plist is legitimate (one spec, two platforms) — but alongside a start
// command it is ambiguous which one wins.
func TestValidateService_ScheduledTaskExcludesStartAndBrew(t *testing.T) {
	svc := Service{
		Start:         []string{"agent", "--daemon"},
		ScheduledTask: &ScheduledTaskSpec{Action: `C:\a.exe`},
	}
	errs := validateService("a", svc, "services")
	joined := strings.ToLower(strings.Join(errorMessages(errs), " | "))
	if !strings.Contains(joined, "mutually exclusive") {
		t.Errorf("start + scheduled_task should be rejected, got %v", errorMessages(errs))
	}
}

func TestScheduledTaskFingerprintChangesWithSpec(t *testing.T) {
	a := ScheduledTaskSpec{Action: `C:\a.exe`, Trigger: "logon"}
	b := a
	b.Args = []string{"--x"}
	if ScheduledTaskFingerprint(&a) == ScheduledTaskFingerprint(&b) {
		t.Error("adding an argument must change the fingerprint, or drift goes undetected")
	}
	c := a
	c.Trigger = "daily"
	if ScheduledTaskFingerprint(&a) == ScheduledTaskFingerprint(&c) {
		t.Error("changing the trigger must change the fingerprint")
	}
	same := ScheduledTaskSpec{Action: `C:\a.exe`, Trigger: "logon"}
	if ScheduledTaskFingerprint(&a) != ScheduledTaskFingerprint(&same) {
		t.Error("equal specs must produce equal fingerprints")
	}
	if ScheduledTaskFingerprint(nil) != "" {
		t.Error("a nil spec must fingerprint as empty, not as a real task")
	}
}

// The strict unknown-field allowlist is a second, independent gate: a
// misspelled key inside scheduled_task is rejected as unknown, not silently
// dropped into a task that never does what the spec says.
func TestParseAndValidate_RejectsUnknownScheduledTaskField(t *testing.T) {
	input := `{"schemaVersion":"8","defaults":{},"targets":{"windows":{"services":{"a":{
		"scheduled_task":{"action":"C:\\a.exe","triggger":"daily"}}}}}}`
	_, errs, err := ParseAndValidate([]byte(input))
	if err != nil {
		t.Fatalf("ParseAndValidate: %v", err)
	}
	if len(errs) == 0 {
		t.Fatal("a misspelled scheduled_task key must be rejected")
	}
	joined := strings.ToLower(strings.Join(errorMessages(errs), " | "))
	if !strings.Contains(joined, "triggger") {
		t.Errorf("error should name the offending field, got: %v", errorMessages(errs))
	}
}
