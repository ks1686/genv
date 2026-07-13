package service

import (
	"context"
	"fmt"
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
