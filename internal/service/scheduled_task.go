package service

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ks1686/genv/internal/schema"
)

// #212: services had no declarative Windows backend, so a spec could only
// schedule work on Windows by carrying an opaque `start` command that did not
// survive to any supervisor. This is the scheduled_task backend: genv renders
// the task definition, registers it, and removes it again when the service
// leaves the spec.
//
// The action is wrapped in a .cmd plus a windowless wscript.exe launcher
// rather than invoked directly, for the same reason the updates checker is:
// a console subsystem binary launched by Task Scheduler flashes a console
// window on every trigger.

// schtasksServiceTaskName is the registered Task Scheduler name for a service.
func schtasksServiceTaskName(name string) string {
	return schtasksTaskName(name)
}

// SchtasksTaskXML renders a Task Scheduler 1.3 definition for a declared
// service. command is the windowless host (wscript.exe) and scriptPath the
// .vbs wrapper it launches.
//
// The principal defaults to the interactive user: InteractiveToken +
// LeastPrivilege needs no elevation, which is the only configuration a
// non-elevated genv can register. A service that must run as SYSTEM has to say
// so explicitly, and then gets HighestAvailable — InteractiveToken would be a
// silent no-op there, because SYSTEM has no interactive logon to attach to.
func SchtasksTaskXML(name string, t schema.ScheduledTaskSpec, command, scriptPath string) string {
	eff := schema.EffectiveScheduledTaskSpec(&t)
	if eff == nil {
		eff = &schema.ScheduledTaskSpec{}
	}
	taskName := schtasksServiceTaskName(name)

	principalID := "InteractiveUser"
	userIDXML := schtasksUserIDXML(schtasksCurrentUserID())
	logonType := "InteractiveToken"
	runLevel := "LeastPrivilege"
	if eff.Principal == schema.ScheduledTaskPrincipalSystem {
		principalID = "SystemAccount"
		userIDXML = schtasksUserIDXML("S-1-5-18")
		logonType = "ServiceAccount"
		runLevel = "HighestAvailable"
	}

	// Task Scheduler only accepts a wall-clock start on a daily or weekly
	// trigger; a logon or boot trigger has no start boundary to hang one on.
	var startBoundary string
	switch eff.Trigger {
	case schema.ScheduledTaskTriggerDaily, schema.ScheduledTaskTriggerWeekly:
		startBoundary = fmt.Sprintf("      <StartBoundary>%s</StartBoundary>\r\n", schtasksStartBoundary(eff))
	}

	triggers := schtasksTriggerXML(eff, startBoundary)

	restart := ""
	if eff.RestartOnFailure {
		interval := eff.RetryInterval
		if interval == "" {
			interval = "PT1M"
		}
		restart = "      <RestartOnFailure>\r\n" +
			"        <Interval>" + xmlEscape(interval) + "</Interval>\r\n" +
			"        <Count>3</Count>\r\n" +
			"      </RestartOnFailure>\r\n"
	}

	timeLimit := eff.ExecutionTimeLimit
	if timeLimit == "" {
		// Leave Task Scheduler's own default rather than imposing one: an
		// unlimited limit is what a long-running service expects, and
		// genv's own __run-once deadline pattern is not this code's job.
		timeLimit = "PT72H"
	}

	description := eff.Description
	if description == "" {
		description = "genv managed service: " + name
	}

	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-16"?>
<Task version="1.3" xmlns="http://schemas.microsoft.com/windows/2004/02/mit/task">
  <RegistrationInfo>
    <Description>%s</Description>
    <URI>\%s</URI>
  </RegistrationInfo>
  <Triggers>
%s  </Triggers>
  <Principals>
    <Principal id="%s">
%s      <LogonType>%s</LogonType>
      <RunLevel>%s</RunLevel>
    </Principal>
  </Principals>
  <Settings>
    <MultipleInstancesPolicy>IgnoreNew</MultipleInstancesPolicy>
    <DisallowStartIfOnBatteries>false</DisallowStartIfOnBatteries>
    <StopIfGoingOnBatteries>false</StopIfGoingOnBatteries>
    <AllowHardTerminate>true</AllowHardTerminate>
    <StartWhenAvailable>true</StartWhenAvailable>
    <RunOnlyIfNetworkAvailable>false</RunOnlyIfNetworkAvailable>
    <IdleSettings>
      <StopOnIdleEnd>false</StopOnIdleEnd>
      <RestartOnIdle>false</RestartOnIdle>
    </IdleSettings>
    <AllowStartOnDemand>true</AllowStartOnDemand>
    <Enabled>true</Enabled>
    <Hidden>false</Hidden>
    <RunOnlyIfIdle>false</RunOnlyIfIdle>
    <DisallowStartOnRemoteAppSession>false</DisallowStartOnRemoteAppSession>
    <UseUnifiedSchedulingEngine>true</UseUnifiedSchedulingEngine>
    <WakeToRun>false</WakeToRun>
    <ExecutionTimeLimit>%s</ExecutionTimeLimit>
    <Priority>7</Priority>
%s  </Settings>
  <Actions Context="%s">
    <Exec>
      <Command>%s</Command>
      <Arguments>%s</Arguments>
      <WorkingDirectory>%s</WorkingDirectory>
    </Exec>
  </Actions>
</Task>
`,
		xmlEscape(stripLineBreaks(description)),
		xmlEscape(stripLineBreaks(taskName)),
		triggers,
		principalID,
		userIDXML,
		logonType,
		runLevel,
		xmlEscape(stripLineBreaks(timeLimit)),
		restart,
		principalID,
		xmlEscape(stripLineBreaks(command)),
		xmlEscape(schtasksServiceArguments(scriptPath)),
		xmlEscape(stripLineBreaks(schtasksServiceWorkingDir(eff))),
	)
}

func schtasksTriggerXML(t *schema.ScheduledTaskSpec, startBoundary string) string {
	switch t.Trigger {
	case schema.ScheduledTaskTriggerBoot:
		return "    <BootTrigger>\r\n      <Enabled>true</Enabled>\r\n" + startBoundary +
			"    </BootTrigger>\r\n"
	case schema.ScheduledTaskTriggerDaily:
		return "    <CalendarTrigger>\r\n      <StartBoundary>" + xmlEscape(schtasksStartBoundary(t)) +
			"</StartBoundary>\r\n      <Enabled>true</Enabled>\r\n      <ScheduleByDay><DaysInterval>1</DaysInterval></ScheduleByDay>\r\n    </CalendarTrigger>\r\n"
	case schema.ScheduledTaskTriggerWeekly:
		return "    <CalendarTrigger>\r\n      <StartBoundary>" + xmlEscape(schtasksStartBoundary(t)) +
			"</StartBoundary>\r\n      <Enabled>true</Enabled>\r\n      <ScheduleByWeek><WeeksInterval>1</WeeksInterval><DaysOfWeek><Day>" +
			schtasksWeekdayName(t.DayOfWeek) + "</Day></DaysOfWeek></ScheduleByWeek>\r\n    </CalendarTrigger>\r\n"
	default:
		return "    <LogonTrigger>\r\n" + schtasksUserIDXML(schtasksCurrentUserID()) +
			"      <Enabled>true</Enabled>\r\n" + startBoundary +
			"    </LogonTrigger>\r\n"
	}
}

// schtasksStartBoundary renders the start time as an ISO 8601 boundary. The
// date is today's: Task Scheduler treats a boundary with a past date as a
// one-shot and will not run the task again on schedule, so a fixed calendar
// date would make daily and weekly tasks fire exactly once, silently.
func schtasksStartBoundary(t *schema.ScheduledTaskSpec) string {
	at := t.At
	if len(at) != 5 {
		at = "00:00"
	}
	now := time.Now()
	return fmt.Sprintf("%04d-%02d-%02dT%s:00", now.Year(), int(now.Month()), now.Day(), at)
}

func schtasksWeekdayName(day string) string {
	switch strings.ToLower(day) {
	case "monday":
		return "Monday"
	case "tuesday":
		return "Tuesday"
	case "wednesday":
		return "Wednesday"
	case "thursday":
		return "Thursday"
	case "friday":
		return "Friday"
	case "saturday":
		return "Saturday"
	default:
		return "Sunday"
	}
}

// schtasksServiceArguments renders the windowless launcher invocation.
//
// The declared action and args are deliberately *not* placed here: they are
// quoted into the .cmd wrapper instead, so a spec argument containing a quote
// cannot break out of the command line Task Scheduler builds.
func schtasksServiceArguments(vbsPath string) string {
	return `//B //Nologo ` + cmdQuoteArg(stripLineBreaks(vbsPath))
}

func schtasksServiceWorkingDir(t *schema.ScheduledTaskSpec) string {
	return filepath.Dir(t.Action)
}

// schtasksServiceCmdContent is the .cmd wrapper that runs the declared action.
// Arguments are quoted individually so a value containing a space or a quote
// is passed through as one argument.
func schtasksServiceCmdContent(t *schema.ScheduledTaskSpec) string {
	var b strings.Builder
	b.WriteString("@echo off\r\n")
	b.WriteString("setlocal\r\n")
	b.WriteString(cmdQuoteArg(stripLineBreaks(t.Action)))
	for _, a := range t.Args {
		b.WriteString(" ")
		b.WriteString(cmdQuoteArg(stripLineBreaks(a)))
	}
	b.WriteString("\r\n")
	b.WriteString("exit /b %ERRORLEVEL%\r\n")
	return b.String()
}

// schtasksServiceFiles returns the artifact paths for a service task.
func schtasksServiceFiles(name string) (cmdPath, vbsPath, xmlPath string, err error) {
	dir, err := schtasksArtifactDir()
	if err != nil {
		return "", "", "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", "", "", fmt.Errorf("creating scheduled task directory: %w", err)
	}
	return filepath.Join(dir, schtasksCmdFileName(name)),
		filepath.Join(dir, schtasksVbsFileName(name)),
		filepath.Join(dir, schtasksXMLFileName(name)), nil
}

// applyScheduledTask registers (or re-registers) a declared service task and
// reports whether anything actually changed.
//
// The second return value matters: apply runs on every reconcile, and a task
// that re-registers itself unconditionally would report every service as
// applied on every run, so a real change could never be told apart from a
// no-op. An unchanged task whose files are already on disk and whose
// registration exists is left alone.
func applyScheduledTask(ctx context.Context, name string, t *schema.ScheduledTaskSpec, verbose bool) (bool, error) {
	eff := schema.EffectiveScheduledTaskSpec(t)
	if eff == nil {
		return false, nil
	}
	cmdPath, vbsPath, xmlPath, err := schtasksServiceFiles(name)
	if err != nil {
		return false, err
	}

	cmdContent := []byte(schtasksServiceCmdContent(eff))
	vbsContent := []byte(SchtasksScheduledVbsContent(schtasksCmdExe(), cmdPath))
	xmlContent := encodeUTF16LE(SchtasksTaskXML(name, *eff, schtasksWscriptExe(), vbsPath))

	changed := !fileHasContent(cmdPath, cmdContent) ||
		!fileHasContent(vbsPath, vbsContent) ||
		!fileHasContent(xmlPath, xmlContent)

	if changed {
		if err := os.WriteFile(cmdPath, cmdContent, 0o644); err != nil {
			return false, fmt.Errorf("writing scheduled task script %q: %w", cmdPath, err)
		}
		if err := os.WriteFile(vbsPath, vbsContent, 0o644); err != nil {
			return false, fmt.Errorf("writing scheduled task host %q: %w", vbsPath, err)
		}
		if err := os.WriteFile(xmlPath, xmlContent, 0o644); err != nil {
			return false, fmt.Errorf("writing scheduled task XML %q: %w", xmlPath, err)
		}
	}

	// A task registered outside genv, or deleted by hand, must come back even
	// when the files on disk are unchanged — which is the same failure mode as
	// a lock entry that is believed forever.
	registered := schtasksTaskExists(ctx, name)
	if !registered {
		changed = true
	}
	if !changed {
		return false, nil
	}

	if verbose {
		_, _ = fmt.Fprintf(os.Stdout, "  service: registering %s via Task Scheduler (%s trigger)\n", name, eff.Trigger)
	}

	taskName := schtasksServiceTaskName(name)
	// /F overwrites an existing registration, which is what makes a changed
	// action or trigger converge rather than fail with "already exists".
	if out, err := schtasksRun(ctx, "/Create", "/TN", taskName, "/XML", xmlPath, "/F"); err != nil {
		decoded := decodeSchtasksOutput(out)
		return false, fmt.Errorf("creating scheduled task %q: %w\n%s%s", taskName, err, decoded, schtasksServiceHint(decoded))
	}
	return true, nil
}

func fileHasContent(path string, want []byte) bool {
	got, err := os.ReadFile(path)
	return err == nil && bytes.Equal(got, want)
}

func schtasksServiceHint(output string) string {
	if !schtasksAccessDenied(output) {
		return ""
	}
	return "\nHint: Task Scheduler denied registering the task. A `system` principal needs an elevated shell; a `user` principal should not. Also check Task Scheduler and Group Policy permissions, or delete a foreign-owned task of the same name and retry."
}

func removeScheduledTask(ctx context.Context, name string, verbose bool) error {
	taskName := schtasksServiceTaskName(name)
	if verbose {
		_, _ = fmt.Fprintf(os.Stdout, "  service: unregistering %s via Task Scheduler\n", name)
	}
	_, _ = schtasksRun(ctx, "/End", "/TN", taskName)
	if out, err := schtasksRun(ctx, "/Delete", "/TN", taskName, "/F"); err != nil {
		decoded := decodeSchtasksOutput(out)
		if !schtasksTaskMissing(decoded) {
			return fmt.Errorf("deleting scheduled task %q: %w\n%s", taskName, err, decoded)
		}
	}
	cmdPath, vbsPath, xmlPath, err := schtasksServiceFiles(name)
	if err != nil {
		return nil
	}
	for _, p := range []string{cmdPath, vbsPath, xmlPath} {
		if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("removing scheduled task artifact %q: %w", p, err)
		}
	}
	return nil
}

// schtasksTaskExists reports whether the task is registered. A missing task is
// not an error: apply re-creates it, and status reports it as absent.
func schtasksTaskExists(ctx context.Context, name string) bool {
	taskName := schtasksServiceTaskName(name)
	out, err := schtasksRun(ctx, "/Query", "/TN", taskName)
	if err != nil {
		return !schtasksTaskMissing(decodeSchtasksOutput(out))
	}
	return true
}

func schtasksTaskRunning(ctx context.Context, name string) bool {
	taskName := schtasksServiceTaskName(name)
	out, _ := schtasksRun(ctx, "/Query", "/TN", taskName, "/FO", "LIST", "/V")
	return ParseSchtasksQueryStatus(decodeSchtasksOutput(out))
}

// ParseSchtasksQueryStatus reads the "Status:" line of `schtasks /Query /FO
// LIST /V`. Task Scheduler reports Ready when the task is registered but idle,
// which is the normal state for a daily or logon task, so only Running counts
// as running.
func ParseSchtasksQueryStatus(output string) bool {
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		rest, ok := cutPrefixFold(line, "Status:")
		if !ok {
			continue
		}
		return strings.EqualFold(strings.TrimSpace(rest), "running")
	}
	return false
}

func cutPrefixFold(s, prefix string) (string, bool) {
	if len(s) < len(prefix) || !strings.EqualFold(s[:len(prefix)], prefix) {
		return "", false
	}
	return s[len(prefix):], true
}

// probeSchtasksServicesFn is a test seam: the Windows registration path is
// exercised on non-Windows CI by replacing the subprocess runner, but the GOOS
// gate is not something a test should have to fake to reach the logic.
var probeSchtasksServicesFn = probeSchtasksServices

// probeSchtasksServices reports whether the Task Scheduler backend can register
// services on this host. GOOS-gated so a stray schtasks binary on a Unix PATH
// cannot claim Windows support.
func probeSchtasksServices() bool { return IsSchtasksAvailable() }

// ProbeSchtasksServiceRunning reports whether a declared task is running.
func ProbeSchtasksServiceRunning(ctx context.Context, name string) bool {
	if !probeSchtasksServices() {
		return false
	}
	return schtasksTaskRunning(ctx, name)
}
