package service

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func intPointer(value int) *int {
	return &value
}

const launchdRunningNeverExited = `com.ks1686.genv.updates = {
	active count = 1
	state = running
	last exit code = (never exited)

	jetsam coalition = {
		ID = 1441
		state = active
	}
}
`

const launchdIdleConfigFailure = `com.ks1686.genv.updates = {
	active count = 0
	state = not running
	last exit code = 78: EX_CONFIG

	jetsam coalition = {
		ID = 1441
		state = active
	}
}
`

func TestScheduledLaunchdStatus_parses_supervisor_output(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   ScheduledJobStatus
	}{
		{
			name:   "registered idle with no known run",
			output: "state = waiting\n",
			want:   ScheduledJobStatus{Supported: true, Registered: true, LastRun: ScheduledRunUnknown, Detail: "com.ks1686.genv.updates"},
		},
		{
			name:   "currently executing before first exit despite nested coalition state",
			output: launchdRunningNeverExited,
			want:   ScheduledJobStatus{Supported: true, Registered: true, Executing: true, LastRun: ScheduledRunUnknown, Detail: "com.ks1686.genv.updates"},
		},
		{
			name:   "last run succeeded with zero exit code",
			output: "state = waiting\nlast exit code = 0\n",
			want:   ScheduledJobStatus{Supported: true, Registered: true, LastRun: ScheduledRunSuccess, ExitCode: intPointer(0), Detail: "com.ks1686.genv.updates"},
		},
		{
			name:   "last run failed with annotated exit reason and nested coalition state",
			output: launchdIdleConfigFailure,
			want: ScheduledJobStatus{
				Supported: true, Registered: true, LastRun: ScheduledRunFailure,
				ExitCode: intPointer(78), LastRunDetail: "EX_CONFIG", Detail: "com.ks1686.genv.updates",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseLaunchdScheduledStatus("com.ks1686.genv.updates", tt.output)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("status = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestScheduledLaunchdStatus_is_conservative_when_loaded_output_is_malformed(t *testing.T) {
	got := parseLaunchdScheduledStatus("com.ks1686.genv.updates", "com.ks1686.genv.updates = {\n}\n")

	if !got.Supported || !got.Registered || got.Executing || got.LastRun != ScheduledRunUnknown {
		t.Fatalf("status = %#v, want registered unknown state", got)
	}
	if !strings.Contains(got.Detail, "malformed launchctl output") {
		t.Fatalf("detail = %q, want malformed-output explanation", got.Detail)
	}
}

func TestScheduledSystemdStatus_parses_stable_show_properties(t *testing.T) {
	timer := "LoadState=loaded\nActiveState=active\nSubState=waiting\n"
	tests := []struct {
		name    string
		service string
		want    ScheduledJobStatus
	}{
		{
			name:    "registered idle with empty execution fields",
			service: "LoadState=loaded\nActiveState=inactive\nSubState=dead\nResult=\nExecMainStatus=\nExecMainStartTimestampMonotonic=0\n",
			want:    ScheduledJobStatus{Supported: true, Registered: true, LastRun: ScheduledRunUnknown, Detail: "genv-updates.timer"},
		},
		{
			name:    "never executed oneshot exposes default success without a start marker",
			service: "LoadState=loaded\nActiveState=inactive\nSubState=dead\nResult=success\nExecMainStatus=0\nExecMainStartTimestampMonotonic=0\n",
			want:    ScheduledJobStatus{Supported: true, Registered: true, LastRun: ScheduledRunUnknown, Detail: "genv-updates.timer"},
		},
		{
			name:    "first execution in progress does not claim a completed outcome",
			service: "LoadState=loaded\nActiveState=active\nSubState=running\nResult=success\nExecMainStatus=0\nExecMainStartTimestampMonotonic=987654321\n",
			want:    ScheduledJobStatus{Supported: true, Registered: true, Executing: true, LastRun: ScheduledRunUnknown, Detail: "genv-updates.timer"},
		},
		{
			name:    "last run succeeded",
			service: "LoadState=loaded\nActiveState=inactive\nSubState=dead\nResult=success\nExecMainStatus=0\nExecMainStartTimestampMonotonic=987654321\n",
			want:    ScheduledJobStatus{Supported: true, Registered: true, LastRun: ScheduledRunSuccess, ExitCode: intPointer(0), Detail: "genv-updates.timer"},
		},
		{
			name:    "last run failed",
			service: "LoadState=loaded\nActiveState=failed\nSubState=failed\nResult=exit-code\nExecMainStatus=9\nExecMainStartTimestampMonotonic=987654321\n",
			want: ScheduledJobStatus{
				Supported: true, Registered: true, LastRun: ScheduledRunFailure,
				ExitCode: intPointer(9), LastRunDetail: "exit-code", Detail: "genv-updates.timer",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseSystemdScheduledStatus("genv-updates.timer", timer, tt.service)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("status = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestScheduledSystemdStatus_is_conservative_when_start_marker_is_malformed(t *testing.T) {
	timer := "LoadState=loaded\nActiveState=active\nSubState=waiting\n"
	service := "LoadState=loaded\nActiveState=inactive\nSubState=dead\nResult=success\nExecMainStatus=0\nExecMainStartTimestampMonotonic=not-a-number\n"

	got := parseSystemdScheduledStatus("genv-updates.timer", timer, service)

	if !got.Supported || !got.Registered || got.Executing || got.LastRun != ScheduledRunUnknown {
		t.Fatalf("status = %#v, want registered unknown state", got)
	}
	if got.ExitCode != nil {
		t.Fatalf("exit code = %v, want no completed outcome", *got.ExitCode)
	}
	if !strings.Contains(got.Detail, "malformed systemctl output") {
		t.Fatalf("detail = %q, want malformed-output explanation", got.Detail)
	}
}

func TestScheduledSystemdStatus_reports_unregistered_timer(t *testing.T) {
	timer := "LoadState=not-found\nActiveState=inactive\nSubState=dead\n"

	got := parseSystemdScheduledStatus("genv-updates.timer", timer, "")

	want := ScheduledJobStatus{Supported: true, LastRun: ScheduledRunUnknown, Detail: "genv-updates.timer"}
	if got != want {
		t.Fatalf("status = %#v, want %#v", got, want)
	}
}

func TestScheduledSystemdStatus_is_conservative_when_loaded_output_is_malformed(t *testing.T) {
	got := parseSystemdScheduledStatus("genv-updates.timer", "LoadState=loaded\n", "LoadState=loaded\n")

	if !got.Supported || !got.Registered || got.Executing || got.LastRun != ScheduledRunUnknown {
		t.Fatalf("status = %#v, want registered unknown state", got)
	}
	if !strings.Contains(got.Detail, "malformed systemctl output") {
		t.Fatalf("detail = %q, want malformed-output explanation", got.Detail)
	}
}

func TestScheduledLaunchdStatus_distinguishes_missing_job_from_inspection_failure(t *testing.T) {
	tests := []struct {
		name       string
		output     string
		commandErr error
		wantStatus ScheduledJobStatus
		wantErr    bool
	}{
		{
			name: "missing job", output: "Could not find service \"com.ks1686.genv.updates\" in domain for user gui: 501",
			commandErr: errors.New("exit status 113"),
			wantStatus: ScheduledJobStatus{Supported: true, LastRun: ScheduledRunUnknown, Detail: "com.ks1686.genv.updates"},
		},
		{name: "inspection failure", output: "Operation not permitted", commandErr: errors.New("exit status 1"), wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			original := scheduledCommandOutput
			scheduledCommandOutput = func(context.Context, string, ...string) ([]byte, error) {
				return []byte(tt.output), tt.commandErr
			}
			t.Cleanup(func() { scheduledCommandOutput = original })

			got, err := launchdScheduledStatus(context.Background(), "com.ks1686.genv.updates")
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr %t", err, tt.wantErr)
			}
			if !tt.wantErr && !reflect.DeepEqual(got, tt.wantStatus) {
				t.Fatalf("status = %#v, want %#v", got, tt.wantStatus)
			}
		})
	}
}

func TestScheduledSystemdStatus_uses_stable_properties_without_real_supervisor(t *testing.T) {
	var calls [][]string
	original := scheduledCommandOutput
	scheduledCommandOutput = func(_ context.Context, command string, args ...string) ([]byte, error) {
		calls = append(calls, append([]string{command}, args...))
		if strings.HasSuffix(args[2], ".timer") {
			return []byte("LoadState=loaded\nActiveState=active\nSubState=waiting\n"), nil
		}
		return []byte("LoadState=loaded\nActiveState=inactive\nSubState=dead\nResult=success\nExecMainStatus=0\nExecMainStartTimestampMonotonic=987654321\n"), nil
	}
	t.Cleanup(func() { scheduledCommandOutput = original })

	got, err := systemdScheduledStatus(context.Background(), "genv-updates.timer", "genv-updates.service")
	if err != nil {
		t.Fatalf("status error: %v", err)
	}
	if got.LastRun != ScheduledRunSuccess || got.ExitCode == nil || *got.ExitCode != 0 {
		t.Fatalf("status = %#v, want successful zero exit status", got)
	}
	timerProperties := []string{
		"--property=LoadState", "--property=ActiveState", "--property=SubState",
	}
	serviceProperties := []string{
		"--property=LoadState", "--property=ActiveState", "--property=SubState",
		"--property=Result", "--property=ExecMainStatus", "--property=ExecMainStartTimestampMonotonic",
	}
	if len(calls) != 2 {
		t.Fatalf("calls = %#v, want timer and service show calls", calls)
	}
	for index, call := range calls {
		if len(call) < 4 || call[0] != "systemctl" || call[1] != "--user" || call[2] != "show" {
			t.Fatalf("call = %v, want systemctl --user show", call)
		}
		wantProperties := serviceProperties
		if index == 0 {
			wantProperties = timerProperties
		}
		for _, property := range wantProperties {
			if !slices.Contains(call, property) {
				t.Fatalf("call = %v, want %s", call, property)
			}
		}
		if index == 0 && (slices.Contains(call, "--property=Result") || slices.Contains(call, "--property=ExecMainStatus") || slices.Contains(call, "--property=ExecMainStartTimestampMonotonic")) {
			t.Fatalf("timer call = %v, want timer-only properties", call)
		}
	}
}
