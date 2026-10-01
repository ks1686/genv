package service

import (
	"strings"
	"testing"

	"github.com/ks1686/genv/internal/schema"
)

// #212: services had no declarative Windows Task Scheduler backend, so a spec
// had to carry an opaque `start` command to run anything scheduled on Windows.
// These tests cover the pure rendering and query-parsing logic, which is
// testable on any OS; the registration path itself needs Windows to verify.
func TestSchtasksTaskXML_UserPrincipalLogonTrigger(t *testing.T) {
	svc := schema.ScheduledTaskSpec{
		Action:  `C:\Program Files\Acme\agent.exe`,
		Trigger: "logon",
	}
	got := SchtasksTaskXML("genv-acme", svc, `C:\Windows\System32\wscript.exe`, `C:\Users\me\.config\genv\services\acme.vbs`)

	for _, want := range []string{
		`<Command>C:\Windows\System32\wscript.exe</Command>`,
		`<LogonTrigger>`,
		`<LogonType>InteractiveToken</LogonType>`,
		`<RunLevel>LeastPrivilege</RunLevel>`,
		`genv-acme`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("task XML missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "<BootTrigger>") {
		t.Errorf("logon trigger must not also emit a boot trigger:\n%s", got)
	}
	if strings.Contains(got, "S-1-5-18") {
		t.Errorf("user principal must not carry the SYSTEM account:\n%s", got)
	}
}

func TestSchtasksTaskXML_SystemPrincipalBootTrigger(t *testing.T) {
	svc := schema.ScheduledTaskSpec{
		Action:    `C:\Windows\System32\agent.exe`,
		Trigger:   "boot",
		Principal: "system",
	}
	got := SchtasksTaskXML("genv-acme", svc, `C:\Windows\System32\wscript.exe`, `C:\x\a.vbs`)

	if !strings.Contains(got, "<BootTrigger>") {
		t.Errorf("boot trigger not rendered:\n%s", got)
	}
	// SYSTEM is S-1-5-18 and must run with highest available privileges;
	// InteractiveToken would be a no-op because there is no interactive logon.
	if !strings.Contains(got, "S-1-5-18") {
		t.Errorf("system principal must set the SYSTEM account:\n%s", got)
	}
	if !strings.Contains(got, "<RunLevel>HighestAvailable</RunLevel>") {
		t.Errorf("system principal must run elevated:\n%s", got)
	}
}

func TestSchtasksTaskXML_DailyTriggerCarriesStartBoundary(t *testing.T) {
	svc := schema.ScheduledTaskSpec{Action: `C:\a.exe`, Trigger: "daily", At: "09:30"}
	got := SchtasksTaskXML("genv-acme", svc, `C:\w.exe`, `C:\a.vbs`)

	if !strings.Contains(got, "<StartBoundary>") {
		t.Errorf("daily trigger needs a StartBoundary or Task Scheduler ignores it:\n%s", got)
	}
	if !strings.Contains(got, "T09:30:00") {
		t.Errorf("daily trigger must use the declared time 09:30:\n%s", got)
	}
}

func TestSchtasksTaskXML_WeeklyTriggerCarriesDay(t *testing.T) {
	svc := schema.ScheduledTaskSpec{Action: `C:\a.exe`, Trigger: "weekly", At: "02:15", DayOfWeek: "monday"}
	got := SchtasksTaskXML("genv-acme", svc, `C:\w.exe`, `C:\a.vbs`)

	if !strings.Contains(got, "T02:15:00") {
		t.Errorf("weekly trigger must use the declared time:\n%s", got)
	}
	if !strings.Contains(got, "Monday") {
		t.Errorf("weekly trigger must carry the declared weekday:\n%s", got)
	}
}

func TestSchtasksTaskXML_RestartOnFailure(t *testing.T) {
	base := schema.ScheduledTaskSpec{Action: `C:\a.exe`}
	off := SchtasksTaskXML("genv-acme", base, `C:\w.exe`, `C:\a.vbs`)
	if strings.Contains(off, "<RestartOnFailure>") {
		t.Errorf("restart must be opt-in:\n%s", off)
	}
	on := base
	on.RestartOnFailure = true
	on.RetryInterval = "PT10M"
	got := SchtasksTaskXML("genv-acme", on, `C:\w.exe`, `C:\a.vbs`)
	if !strings.Contains(got, "<RestartOnFailure>") {
		t.Errorf("restart_on_failure not rendered:\n%s", got)
	}
	if !strings.Contains(got, "PT10M") {
		t.Errorf("retry_interval not rendered:\n%s", got)
	}
}

// Declared args are rendered into the .cmd wrapper, not into the task XML, so
// that the action's own exit code becomes the task's last result — which is
// what RestartOnFailure keys on. An arg containing a space or a quote must
// survive as one argument.
func TestSchtasksServiceCmd_QuotesArgsIndividually(t *testing.T) {
	svc := schema.ScheduledTaskSpec{
		Action: `C:\Program Files\Acme\agent.exe`,
		Args:   []string{`--mode`, `fast`, `a b`, `say "hi"`, `end&`},
	}
	got := schtasksServiceCmdContent(&svc)

	for _, want := range []string{`--mode`, `fast`, `"a b"`, `"say ""hi"""`, `"end&"`} {
		if !strings.Contains(got, want) {
			t.Errorf("cmd wrapper missing %q:\n%s", want, got)
		}
	}
	if !strings.Contains(got, `"C:\Program Files\Acme\agent.exe"`) {
		t.Errorf("action with a space must be quoted:\n%s", got)
	}
	if !strings.Contains(got, "exit /b %ERRORLEVEL%") {
		t.Errorf("cmd wrapper must propagate the exit code so RestartOnFailure can see it:\n%s", got)
	}
}

// The spec field must never reach the task XML verbatim: a newline in a
// description or name would let a spec author inject task XML into genv's own
// generated definition, which Task Scheduler then registers.
func TestSchtasksTaskXML_StripsLineBreaksFromInjectedFields(t *testing.T) {
	// The injected markup must not become structure. Stripping the newline
	// leaves the text as inert content inside <Description>, which is safe;
	// what must not happen is a second <Exec> or </Description> appearing.
	svc := schema.ScheduledTaskSpec{
		Action:      "C:\\a.exe",
		Description: "harmless</Description><Exec><Command>evil.exe</Command></Exec><Description>",
	}
	got := SchtasksTaskXML("genv-acme", svc, `C:\w.exe`, `C:\a.vbs`)
	if n := strings.Count(got, "<Exec>"); n != 1 {
		t.Errorf("injected a second <Exec> block (found %d):\n%s", n, got)
	}
	if n := strings.Count(got, "<Command>"); n != 1 {
		t.Errorf("injected a second <Command> element (found %d):\n%s", n, got)
	}
	if n := strings.Count(got, "</Description>"); n != 1 {
		t.Errorf("injected a closing Description tag (found %d):\n%s", n, got)
	}
	if !strings.Contains(got, "&lt;Command&gt;evil.exe") {
		t.Errorf("injected markup was not XML-escaped:\n%s", got)
	}

	// A newline in the action is rejected at validation, but the renderer must
	// not be the thing that decides: the action reaches the .cmd wrapper, so
	// that is where a smuggled newline has to be stripped.
	bad := schema.ScheduledTaskSpec{Action: "C:\\a.exe & echo pwned\nC:\\b.exe"}
	cmd := schtasksServiceCmdContent(&bad)
	// The wrapper is exactly four lines (@echo off, setlocal, the command,
	// exit /b); a smuggled newline would add a fifth and turn the rest of the
	// action into a separate command.
	if n := strings.Count(cmd, "\r\n"); n != 4 {
		t.Errorf("cmd wrapper has %d lines, want 4: a newline in the action was not stripped: %q", n, cmd)
	}
}

func TestParseSchtasksQueryStatus(t *testing.T) {
	cases := []struct {
		name    string
		out     string
		running bool
	}{
		{"running", "Status:  Running\n", true},
		{"ready", "Status:  Ready\n", false},
		{"disabled", "Status:  Disabled\n", false},
		{"running localized variant", "Status:\tRunning\n", true},
		{"no status line", "Something else\n", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ParseSchtasksQueryStatus(tc.out); got != tc.running {
				t.Errorf("ParseSchtasksQueryStatus(%q) = %v, want %v", tc.out, got, tc.running)
			}
		})
	}
}

// A missing task is not an error state: apply re-creates it, and status should
// report it as absent rather than surfacing a raw schtasks error.
func TestSchtasksTaskMissingRecognised(t *testing.T) {
	for _, out := range []string{
		"ERROR: The system cannot find the file specified.\n",
		"ERROR: The system cannot find the file specified.",
		"ERROR: The system cannot find the task specified.\n",
	} {
		if !schtasksTaskMissing(out) {
			t.Errorf("schtasksTaskMissing(%q) = false, want true", out)
		}
	}
	if schtasksTaskMissing("ERROR: Access is denied.\n") {
		t.Error("an access-denied error must not be mistaken for a missing task")
	}
}
