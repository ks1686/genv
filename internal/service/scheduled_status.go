package service

import (
	"context"
	"fmt"
	"math"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// ScheduledRunOutcome describes the most recent completed execution known to the supervisor.
type ScheduledRunOutcome string

const (
	ScheduledRunUnknown ScheduledRunOutcome = "unknown"
	ScheduledRunSuccess ScheduledRunOutcome = "success"
	ScheduledRunFailure ScheduledRunOutcome = "failure"
)

var scheduledCommandOutput = func(ctx context.Context, command string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, command, args...).CombinedOutput()
}

func launchdScheduledStatus(ctx context.Context, label string) (ScheduledJobStatus, error) {
	target := "gui/" + strconv.Itoa(os.Getuid()) + "/" + label
	output, err := scheduledCommandOutput(ctx, "launchctl", "print", target)
	if err != nil {
		if supervisorUnitMissing(string(output)) {
			return unregisteredScheduledStatus(label), nil
		}
		return ScheduledJobStatus{}, fmt.Errorf("inspect launchd scheduled job %q: %w", label, err)
	}
	return parseLaunchdScheduledStatus(label, string(output)), nil
}

func parseLaunchdScheduledStatus(label, output string) ScheduledJobStatus {
	status := registeredScheduledStatus(label)
	fields := parseLaunchdFields(output)
	state, hasState := fields["state"]
	if hasState {
		status.Executing = strings.EqualFold(state, "running")
	}

	exitCodeText, hasExitCode := fields["last exit code"]
	if hasExitCode {
		exitCode, reason, known, err := parseLaunchdExitCode(exitCodeText)
		if err == nil && known {
			status.ExitCode = &exitCode
			status.LastRunDetail = reason
			if exitCode == 0 {
				status.LastRun = ScheduledRunSuccess
			} else {
				status.LastRun = ScheduledRunFailure
			}
		} else if err != nil {
			status.Detail = malformedStatusDetail(label, "launchctl", "invalid last exit code")
		}
	}
	if reason := fields["last exit reason"]; reason != "" {
		status.LastRunDetail = reason
		if !hasExitCode {
			status.LastRun = ScheduledRunFailure
		}
	}
	if !hasState {
		status.Detail = malformedStatusDetail(label, "launchctl", "missing state")
	}
	return status
}

func parseLaunchdFields(output string) map[string]string {
	fields := make(map[string]string)
	fieldDepths := make(map[string]int)
	depth := 0
	for line := range strings.SplitSeq(output, "\n") {
		trimmedLine := strings.TrimSpace(line)
		key, value, ok := strings.Cut(trimmedLine, "=")
		if !ok {
			depth += strings.Count(trimmedLine, "{") - strings.Count(trimmedLine, "}")
			continue
		}
		key = strings.ToLower(strings.TrimSpace(key))
		switch key {
		case "state", "last exit code", "last exit reason":
			previousDepth, exists := fieldDepths[key]
			if !exists || depth < previousDepth {
				fields[key] = strings.TrimSpace(value)
				fieldDepths[key] = depth
			}
		}
		depth += strings.Count(trimmedLine, "{") - strings.Count(trimmedLine, "}")
	}
	return fields
}

func parseLaunchdExitCode(value string) (int, string, bool, error) {
	value = strings.TrimSpace(value)
	if strings.EqualFold(value, "(never exited)") {
		return 0, "", false, nil
	}
	codeText, reason, _ := strings.Cut(value, ":")
	exitCode, err := strconv.Atoi(strings.TrimSpace(codeText))
	if err != nil {
		return 0, "", false, err
	}
	return exitCode, strings.TrimSpace(reason), true, nil
}

func systemdScheduledStatus(ctx context.Context, timerName, serviceName string) (ScheduledJobStatus, error) {
	timerOutput, err := systemdShow(ctx, timerName, "LoadState", "ActiveState", "SubState")
	if err != nil {
		if supervisorUnitMissing(string(timerOutput)) {
			return unregisteredScheduledStatus(timerName), nil
		}
		return ScheduledJobStatus{}, fmt.Errorf("inspect systemd timer %q: %w", timerName, err)
	}
	timerFields, _ := parseSystemdProperties(string(timerOutput), "LoadState", "ActiveState", "SubState")
	if timerFields["LoadState"] == "not-found" {
		return unregisteredScheduledStatus(timerName), nil
	}

	serviceOutput, err := systemdShow(ctx, serviceName, "LoadState", "ActiveState", "SubState", "Result", "ExecMainStatus", "ExecMainStartTimestampMonotonic")
	if err != nil {
		if supervisorUnitMissing(string(serviceOutput)) {
			status := registeredScheduledStatus(timerName)
			status.Detail = malformedStatusDetail(timerName, "systemctl", "service unit not found")
			return status, nil
		}
		return ScheduledJobStatus{}, fmt.Errorf("inspect systemd service %q: %w", serviceName, err)
	}
	return parseSystemdScheduledStatus(timerName, string(timerOutput), string(serviceOutput)), nil
}

func systemdShow(ctx context.Context, unit string, properties ...string) ([]byte, error) {
	args := []string{"--user", "show", unit}
	for _, property := range properties {
		args = append(args, "--property="+property)
	}
	args = append(args, "--no-pager")
	return scheduledCommandOutput(ctx, "systemctl", args...)
}

func parseSystemdScheduledStatus(timerName, timerOutput, serviceOutput string) ScheduledJobStatus {
	timerFields, timerComplete := parseSystemdProperties(timerOutput, "LoadState", "ActiveState", "SubState")
	if timerFields["LoadState"] == "not-found" {
		return unregisteredScheduledStatus(timerName)
	}
	status := registeredScheduledStatus(timerName)
	serviceFields, serviceComplete := parseSystemdProperties(serviceOutput, "LoadState", "ActiveState", "SubState", "Result", "ExecMainStatus", "ExecMainStartTimestampMonotonic")
	if timerFields["LoadState"] != "loaded" || serviceFields["LoadState"] != "loaded" || !timerComplete || !serviceComplete {
		status.Detail = malformedStatusDetail(timerName, "systemctl", "missing or unexpected properties")
		return status
	}

	activeState := serviceFields["ActiveState"]
	subState := serviceFields["SubState"]
	status.Executing = activeState == "activating" || (activeState == "active" && subState == "running")

	// A never-executed oneshot service exposes default Result=success and
	// ExecMainStatus=0, so a real invocation is only proven by a non-zero start
	// marker. Parse it as an unsigned integer and stay conservative otherwise.
	startTimestamp, err := strconv.ParseUint(serviceFields["ExecMainStartTimestampMonotonic"], 10, 64)
	if err != nil {
		status.Detail = malformedStatusDetail(timerName, "systemctl", "invalid ExecMainStartTimestampMonotonic")
		return status
	}
	if startTimestamp == 0 {
		return status
	}

	// While the service is executing, its execution fields describe the in-flight
	// invocation rather than a completed outcome, so report executing with an
	// unknown last run instead of the active run's provisional Result/status.
	if status.Executing {
		return status
	}

	result := serviceFields["Result"]
	exitCodeText := serviceFields["ExecMainStatus"]
	if result == "" && exitCodeText == "" {
		return status
	}
	status.LastRunDetail = result
	if exitCodeText != "" {
		exitCode, err := strconv.Atoi(exitCodeText)
		if err != nil {
			status.Detail = malformedStatusDetail(timerName, "systemctl", "invalid ExecMainStatus")
			return status
		}
		status.ExitCode = &exitCode
	}
	if result == "success" && status.ExitCode != nil && *status.ExitCode == 0 {
		status.LastRun = ScheduledRunSuccess
		status.LastRunDetail = ""
		return status
	}
	status.LastRun = ScheduledRunFailure
	return status
}

func parseSystemdProperties(output string, requiredProperties ...string) (map[string]string, bool) {
	fields := make(map[string]string)
	for line := range strings.SplitSeq(output, "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), "=")
		if ok {
			fields[strings.TrimSpace(key)] = strings.TrimSpace(value)
		}
	}
	for _, key := range requiredProperties {
		if _, ok := fields[key]; !ok {
			return fields, false
		}
	}
	return fields, true
}

func supervisorUnitMissing(output string) bool {
	message := strings.ToLower(output)
	return strings.Contains(message, "could not find service") ||
		strings.Contains(message, "could not be found") ||
		strings.Contains(message, "service not found") ||
		strings.Contains(message, "unit not found")
}

func registeredScheduledStatus(detail string) ScheduledJobStatus {
	return ScheduledJobStatus{Supported: true, Registered: true, LastRun: ScheduledRunUnknown, Detail: detail}
}

func unregisteredScheduledStatus(detail string) ScheduledJobStatus {
	return ScheduledJobStatus{Supported: true, LastRun: ScheduledRunUnknown, Detail: detail}
}

func malformedStatusDetail(detail, supervisor, reason string) string {
	return fmt.Sprintf("%s: malformed %s output (%s)", detail, supervisor, reason)
}

const (
	schedSTaskReady        = 0x00041300 // 267008
	schedSTaskRunning      = 0x00041301 // 267009
	schedSTaskDisabled     = 0x00041302 // 267010
	schedSTaskHasNotRun    = 0x00041303 // 267011
	schedSTaskNoMoreRuns   = 0x00041304 // 267012
	schedSTaskNotScheduled = 0x00041305 // 267013
	schedSTaskTerminated   = 0x00041306 // 267014
	schedSTaskNoValidTrig  = 0x00041307 // 267015
	schedSEventTrigger     = 0x00041308 // 267016
)

func schtasksScheduledStatus(ctx context.Context, taskName string) (ScheduledJobStatus, error) {
	output, err := schtasksRun(ctx, "/Query", "/TN", taskName, "/FO", "LIST", "/V")
	decoded := decodeSchtasksOutput(output)
	if err != nil {
		if schtasksTaskMissing(decoded) {
			return unregisteredScheduledStatus(taskName), nil
		}
		return ScheduledJobStatus{}, fmt.Errorf("inspect scheduled task %q: %w\n%s", taskName, err, decoded)
	}
	return parseSchtasksScheduledStatus(taskName, decoded), nil
}

func parseSchtasksScheduledStatus(taskName, output string) ScheduledJobStatus {
	fields := parseSchtasksList(output)
	if schtasksTaskMissing(output) {
		return unregisteredScheduledStatus(taskName)
	}
	statusField := fields["status"]
	if statusField == "" && fields["taskname"] == "" && fields["last result"] == "" {
		status := registeredScheduledStatus(taskName)
		status.Detail = malformedStatusDetail(taskName, "schtasks", "missing Status")
		return status
	}

	status := registeredScheduledStatus(taskName)
	status.Executing = strings.EqualFold(statusField, "Running")

	resultText, hasResult := fields["last result"]
	if !hasResult {
		if statusField == "" {
			status.Detail = malformedStatusDetail(taskName, "schtasks", "missing Last Result")
		}
		return status
	}
	exitCode, result, err := parseSchtasksLastResult(resultText)
	if err != nil {
		status.Detail = malformedStatusDetail(taskName, "schtasks", "invalid Last Result")
		return status
	}
	status.ExitCode = &exitCode

	if status.Executing {
		status.ExitCode = nil
		return status
	}

	lastRunTime := fields["last run time"]
	if schtasksNeverRun(result, lastRunTime) {
		status.ExitCode = nil
		return status
	}
	if result == 0 {
		status.LastRun = ScheduledRunSuccess
		status.ExitCode = &exitCode
		return status
	}
	if result == schedSTaskTerminated {
		status.LastRun = ScheduledRunFailure
		status.LastRunDetail = "terminated"
		return status
	}
	if schtasksInformationalResult(result) {
		status.ExitCode = nil
		return status
	}
	status.LastRun = ScheduledRunFailure
	status.LastRunDetail = resultText
	return status
}

// parseSchtasksLastResult parses schtasks "Last Result" into an int. CodeQL
// go/incorrect-integer-conversion requires a visible MaxInt/MinInt guard
// before narrowing the int64 ParseInt result.
func parseSchtasksLastResult(text string) (int, int64, error) {
	n, err := strconv.ParseInt(strings.TrimSpace(text), 10, 64)
	if err != nil {
		return 0, 0, err
	}
	if n <= math.MaxInt && n >= math.MinInt {
		return int(n), n, nil
	}
	return 0, n, strconv.ErrRange
}

func parseSchtasksList(output string) map[string]string {
	fields := make(map[string]string)
	for line := range strings.SplitSeq(output, "\n") {
		line = strings.TrimSpace(strings.TrimSuffix(line, "\r"))
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		key = strings.ToLower(strings.TrimSpace(key))
		switch key {
		case "taskname", "status", "last run time", "last result":
			if _, exists := fields[key]; !exists {
				fields[key] = strings.TrimSpace(value)
			}
		}
	}
	return fields
}

func schtasksNeverRun(result int64, lastRunTime string) bool {
	if result == schedSTaskHasNotRun {
		return true
	}
	normalized := strings.ToLower(strings.TrimSpace(lastRunTime))
	if normalized == "" || normalized == "n/a" {
		return result == 0 || schtasksInformationalResult(result)
	}
	// Task Scheduler historically reports 11/30/1999 (or locale equivalent)
	// when a task has never executed.
	return strings.Contains(normalized, "1999")
}

func schtasksInformationalResult(result int64) bool {
	switch result {
	case schedSTaskReady, schedSTaskRunning, schedSTaskDisabled,
		schedSTaskHasNotRun, schedSTaskNoMoreRuns, schedSTaskNotScheduled,
		schedSTaskNoValidTrig, schedSEventTrigger:
		return true
	default:
		return false
	}
}
