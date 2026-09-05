package service

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode/utf16"

	"github.com/ks1686/genv/internal/testutil"
)

func TestSchtasksScheduledJob_registerStatusStop(t *testing.T) {
	home := t.TempDir()
	testutil.SetHome(t, home)

	var calls [][]string
	queryOutput := schtasksListOutput("Ready", "8/27/2026 6:22:00 PM", "0")
	withSchtasksRun(t, func(_ context.Context, args ...string) ([]byte, error) {
		calls = append(calls, append([]string{}, args...))
		if len(args) == 0 {
			return nil, errors.New("missing schtasks args")
		}
		switch args[0] {
		case "/Create", "/Run", "/End", "/Delete":
			return []byte("SUCCESS\n"), nil
		case "/Query":
			return []byte(queryOutput), nil
		default:
			return nil, fmt.Errorf("unexpected schtasks args %v", args)
		}
	})

	ctx := context.Background()
	job := ScheduledJob{
		Name:        "updates",
		Command:     []string{`C:\Users\qa\scoop\shims\genv.exe`, "updates", "__run-once", "--file", `C:\Users\qa\.config\genv\genv.json`},
		Interval:    time.Hour,
		Environment: map[string]string{"PATH": `C:\Users\qa\scoop\shims;C:\Windows\System32`},
	}

	if err := startSchtasksScheduledJob(ctx, job); err != nil {
		t.Fatalf("startSchtasksScheduledJob: %v", err)
	}

	scriptPath := filepath.Join(home, ".config", "genv", "scheduled", "genv-updates.cmd")
	vbsPath := filepath.Join(home, ".config", "genv", "scheduled", "genv-updates.vbs")
	xmlPath := filepath.Join(home, ".config", "genv", "scheduled", "genv-updates.xml")
	script, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatalf("read cmd wrapper: %v", err)
	}
	if !bytes.Contains(script, []byte(`rem genv-program: C:\Users\qa\scoop\shims\genv.exe`)) {
		t.Fatalf("cmd wrapper = %q, want genv-program marker", script)
	}
	if !bytes.Contains(script, []byte(`set "PATH=C:\Users\qa\scoop\shims;C:\Windows\System32"`)) {
		t.Fatalf("cmd wrapper = %q, want PATH assignment", script)
	}
	vbs, err := os.ReadFile(vbsPath)
	if err != nil {
		t.Fatalf("read vbs wrapper: %v", err)
	}
	assertSchtasksWindowlessVbs(t, string(vbs), scriptPath)
	xmlBytes, err := os.ReadFile(xmlPath)
	if err != nil {
		t.Fatalf("read XML: %v", err)
	}
	if len(xmlBytes) < 2 || xmlBytes[0] != 0xFF || xmlBytes[1] != 0xFE {
		t.Fatalf("XML missing UTF-16 LE BOM")
	}
	xmlText := string(utf16.Decode(bytesToUTF16(xmlBytes[2:])))
	if !strings.Contains(xmlText, "<LogonTrigger>") || !strings.Contains(xmlText, "<Interval>PT1H</Interval>") {
		t.Fatalf("XML = %q, want logon + hourly repetition", xmlText)
	}
	assertSchtasksWindowlessXML(t, xmlText, vbsPath)
	if !strings.Contains(xmlText, "<LogonType>InteractiveToken</LogonType>") {
		t.Fatalf("XML = %q, want InteractiveToken so desktop notifications still work", xmlText)
	}

	if len(calls) < 2 || !schtasksCallHas(calls, "/Create", "/TN", "genv-updates", "/XML") || !schtasksCallHas(calls, "/Run", "/TN", "genv-updates") {
		t.Fatalf("schtasks calls = %#v, want /Create then /Run of genv-updates", calls)
	}

	status, err := schtasksScheduledStatus(ctx, "genv-updates")
	if err != nil {
		t.Fatalf("status after start: %v", err)
	}
	if !status.Registered || status.LastRun != ScheduledRunSuccess || status.ExitCode == nil || *status.ExitCode != 0 {
		t.Fatalf("status = %#v, want registered last-run success", status)
	}

	if err := stopSchtasksScheduledJob(ctx, job.Name); err != nil {
		t.Fatalf("stopSchtasksScheduledJob: %v", err)
	}
	if !schtasksCallHas(calls, "/End", "/TN", "genv-updates") {
		t.Fatalf("schtasks calls = %#v, want /End of genv-updates before delete", calls)
	}
	if !schtasksCallHas(calls, "/Delete", "/TN", "genv-updates", "/F") {
		t.Fatalf("schtasks calls = %#v, want /Delete of genv-updates", calls)
	}
	if _, err := os.Stat(scriptPath); !os.IsNotExist(err) {
		t.Fatalf("cmd wrapper still exists: %v", err)
	}
	if _, err := os.Stat(vbsPath); !os.IsNotExist(err) {
		t.Fatalf("vbs wrapper still exists: %v", err)
	}
	if _, err := os.Stat(xmlPath); !os.IsNotExist(err) {
		t.Fatalf("XML still exists: %v", err)
	}
}

func TestSchtasksScheduledVbsContent_runs_cmd_hidden(t *testing.T) {
	script := `C:\Users\qa\.config\genv\scheduled\genv-updates.cmd`
	vbs := SchtasksScheduledVbsContent(`C:\Windows\System32\cmd.exe`, script)
	assertSchtasksWindowlessVbs(t, vbs, script)

	quoted := SchtasksScheduledVbsContent(`C:\Windows\System32\cmd.exe`, `C:\genv "quoted"\job.cmd`)
	// cmd doubles " to "", then VBS doubles each of those again.
	if !strings.Contains(quoted, `C:\genv """"quoted""""\job.cmd`) {
		t.Fatalf("vbs = %q, want cmd+VBS-escaped quoted path", quoted)
	}
}

func TestSchtasksXML_action_is_windowless(t *testing.T) {
	vbs := `C:\Users\qa\.config\genv\scheduled\genv-updates.vbs`
	xml := SchtasksScheduledTaskXML("updates", `C:\Windows\System32\wscript.exe`, vbs, time.Hour)
	assertSchtasksWindowlessXML(t, xml, vbs)
	if !strings.Contains(xml, "<LogonType>InteractiveToken</LogonType>") || !strings.Contains(xml, "<Hidden>true</Hidden>") {
		t.Fatalf("XML = %q, want InteractiveToken and Hidden", xml)
	}
}

func TestSchtasksScheduledStatus_parses_query_output(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   ScheduledJobStatus
	}{
		{
			name:   "registered idle with no known run",
			output: schtasksListOutput("Ready", "N/A", "267011"),
			want:   ScheduledJobStatus{Supported: true, Registered: true, LastRun: ScheduledRunUnknown, Detail: "genv-updates"},
		},
		{
			name:   "never-run sentinel date does not claim success",
			output: schtasksListOutput("Ready", "11/30/1999 12:00:00 AM", "0"),
			want:   ScheduledJobStatus{Supported: true, Registered: true, LastRun: ScheduledRunUnknown, Detail: "genv-updates"},
		},
		{
			name:   "currently executing",
			output: schtasksListOutput("Running", "8/27/2026 6:22:00 PM", "267009"),
			want:   ScheduledJobStatus{Supported: true, Registered: true, Executing: true, LastRun: ScheduledRunUnknown, Detail: "genv-updates"},
		},
		{
			name:   "last run succeeded",
			output: schtasksListOutput("Ready", "8/27/2026 6:22:00 PM", "0"),
			want: ScheduledJobStatus{
				Supported: true, Registered: true, LastRun: ScheduledRunSuccess,
				ExitCode: intPointer(0), Detail: "genv-updates",
			},
		},
		{
			name:   "last run failed",
			output: schtasksListOutput("Ready", "8/27/2026 6:22:00 PM", "1"),
			want: ScheduledJobStatus{
				Supported: true, Registered: true, LastRun: ScheduledRunFailure,
				ExitCode: intPointer(1), LastRunDetail: "1", Detail: "genv-updates",
			},
		},
		{
			name:   "last run terminated by scheduler",
			output: schtasksListOutput("Ready", "8/27/2026 6:22:00 PM", "267014"),
			want: ScheduledJobStatus{
				Supported: true, Registered: true, LastRun: ScheduledRunFailure,
				ExitCode: intPointer(267014), LastRunDetail: "terminated", Detail: "genv-updates",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseSchtasksScheduledStatus("genv-updates", tt.output)
			if !scheduledStatusEqual(got, tt.want) {
				t.Fatalf("status = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestSchtasksScheduledStatus_distinguishes_missing_job_from_inspection_failure(t *testing.T) {
	tests := []struct {
		name       string
		output     string
		commandErr error
		wantStatus ScheduledJobStatus
		wantErr    bool
	}{
		{
			name:       "missing job",
			output:     "ERROR: The system cannot find the file specified.\r\n",
			commandErr: errors.New("exit status 1"),
			wantStatus: ScheduledJobStatus{Supported: true, LastRun: ScheduledRunUnknown, Detail: "genv-updates"},
		},
		{
			name:       "missing named task",
			output:     `ERROR: The specified task name "genv-updates" does not exist in the system.`,
			commandErr: errors.New("exit status 1"),
			wantStatus: ScheduledJobStatus{Supported: true, LastRun: ScheduledRunUnknown, Detail: "genv-updates"},
		},
		{name: "inspection failure", output: "Access is denied.", commandErr: errors.New("exit status 1"), wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withSchtasksRun(t, func(context.Context, ...string) ([]byte, error) {
				return []byte(tt.output), tt.commandErr
			})

			got, err := schtasksScheduledStatus(context.Background(), "genv-updates")
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr %t", err, tt.wantErr)
			}
			if !tt.wantErr && !scheduledStatusEqual(got, tt.wantStatus) {
				t.Fatalf("status = %#v, want %#v", got, tt.wantStatus)
			}
		})
	}
}

func TestSchtasksScheduledStatus_is_conservative_when_loaded_output_is_malformed(t *testing.T) {
	got := parseSchtasksScheduledStatus("genv-updates", "Folder: \\\nHostName: QA\n")

	if !got.Supported || !got.Registered || got.Executing || got.LastRun != ScheduledRunUnknown {
		t.Fatalf("status = %#v, want registered unknown state", got)
	}
	if !strings.Contains(got.Detail, "malformed schtasks output") {
		t.Fatalf("detail = %q, want malformed-output explanation", got.Detail)
	}
}

func TestParseSchtasksLastResult_does_not_truncate_int32(t *testing.T) {
	// MaxInt32+1 is a legal int64 / 64-bit int, but wraps to MinInt32 if stored as int32.
	widerThanInt32 := int64(math.MaxInt32) + 1
	wrappedInt32 := int32(widerThanInt32)
	if wrappedInt32 != math.MinInt32 {
		t.Fatalf("int32(%d) = %d, want MinInt32 so this test still detects truncation", widerThanInt32, wrappedInt32)
	}

	got, n, err := parseSchtasksLastResult(strconv.FormatInt(widerThanInt32, 10))
	if widerThanInt32 > math.MaxInt {
		if err == nil {
			t.Fatalf("parse(%d) = %d; want range error rather than truncated %d", widerThanInt32, got, int(wrappedInt32))
		}
		return
	}
	if err != nil {
		t.Fatalf("parse(%d): %v", widerThanInt32, err)
	}
	if n != widerThanInt32 || int64(got) != widerThanInt32 {
		t.Fatalf("parse = %d, %d; want %d (int32 truncation would be %d)", got, n, widerThanInt32, wrappedInt32)
	}
}

func TestSchtasksScheduledStatus_last_result_wider_than_int32_is_preserved(t *testing.T) {
	widerThanInt32 := int64(math.MaxInt32) + 1
	text := strconv.FormatInt(widerThanInt32, 10)
	got := parseSchtasksScheduledStatus("genv-updates", schtasksListOutput("Ready", "8/27/2026 6:22:00 PM", text))

	if widerThanInt32 > math.MaxInt {
		if got.ExitCode != nil || !strings.Contains(got.Detail, "invalid Last Result") {
			t.Fatalf("status = %#v, want malformed Last Result (int32 wrap is %d)", got, int32(widerThanInt32))
		}
		return
	}
	if got.LastRun != ScheduledRunFailure || got.ExitCode == nil || int64(*got.ExitCode) != widerThanInt32 {
		t.Fatalf("status = %#v, want failure exit %d (int32 wrap is %d)", got, widerThanInt32, int32(widerThanInt32))
	}
}

func TestSchtasksScheduledStatus_last_result_rejects_int64_overflow(t *testing.T) {
	got := parseSchtasksScheduledStatus("genv-updates", schtasksListOutput("Ready", "8/27/2026 6:22:00 PM", "9223372036854775808"))
	if got.ExitCode != nil || !strings.Contains(got.Detail, "invalid Last Result") {
		t.Fatalf("status = %#v, want malformed Last Result for int64 overflow", got)
	}
}

func TestSchtasksCreate_failure_surfaces_schtasks_output(t *testing.T) {
	home := t.TempDir()
	testutil.SetHome(t, home)
	withSchtasksRun(t, func(_ context.Context, args ...string) ([]byte, error) {
		if len(args) > 0 && args[0] == "/Create" {
			return []byte("ERROR: Access is denied.\r\n"), errors.New("exit status 1")
		}
		return []byte("SUCCESS\n"), nil
	})
	err := startSchtasksScheduledJob(context.Background(), ScheduledJob{
		Name:     "updates",
		Command:  []string{`C:\genv.exe`, "updates", "__run-once"},
		Interval: time.Hour,
	})
	if err == nil || !strings.Contains(err.Error(), "Access is denied") {
		t.Fatalf("start error = %v, want schtasks output", err)
	}
}

func TestSchtasksStop_missing_task_is_ok(t *testing.T) {
	home := t.TempDir()
	testutil.SetHome(t, home)
	withSchtasksRun(t, func(context.Context, ...string) ([]byte, error) {
		return []byte("ERROR: The system cannot find the file specified.\r\n"), errors.New("exit status 1")
	})
	if err := stopSchtasksScheduledJob(context.Background(), "updates"); err != nil {
		t.Fatalf("stop missing task: %v", err)
	}
}

func TestSchtasksXML_clamps_sub_minute_interval(t *testing.T) {
	xml := SchtasksScheduledTaskXML("updates", `C:\Windows\System32\wscript.exe`, `C:\genv-updates.vbs`, 15*time.Second)
	if !strings.Contains(xml, "<Interval>PT1M</Interval>") {
		t.Fatalf("XML = %q, want 1m clamp", xml)
	}
}

func TestEncodeUTF16LE_roundTrip(t *testing.T) {
	encoded := encodeUTF16LE("genv-updates")
	if encoded[0] != 0xFF || encoded[1] != 0xFE {
		t.Fatalf("missing BOM: %v", encoded[:2])
	}
	if decodeSchtasksOutput(encoded) != "genv-updates" {
		t.Fatalf("decode = %q", decodeSchtasksOutput(encoded))
	}
}

func TestSchtasksBackend_realWindowsRegisterStatusStop(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("real schtasks is Windows-only")
	}
	if !IsSchtasksAvailable() {
		t.Skip("schtasks not on PATH")
	}

	home := t.TempDir()
	testutil.SetHome(t, home)
	ctx := context.Background()
	name := fmt.Sprintf("ci-%d", os.Getpid())
	t.Cleanup(func() {
		_ = stopSchtasksScheduledJob(context.Background(), name)
	})

	job := ScheduledJob{
		Name:     name,
		Command:  []string{schtasksCmdExe(), "/c", "exit", "0"},
		Interval: time.Hour,
		Environment: map[string]string{
			"PATH": ScheduledWindowsPath(os.Getenv("PATH"), home, os.Getenv("LOCALAPPDATA"), os.Getenv("SCOOP")),
		},
	}
	if err := startSchtasksScheduledJob(ctx, job); err != nil {
		t.Fatalf("startSchtasksScheduledJob: %v", err)
	}

	status, err := schtasksScheduledStatus(ctx, schtasksTaskName(name))
	if err != nil {
		t.Fatalf("status after start: %v", err)
	}
	if !status.Supported || !status.Registered {
		t.Fatalf("status = %#v, want registered", status)
	}

	if err := stopSchtasksScheduledJob(ctx, name); err != nil {
		t.Fatalf("stopSchtasksScheduledJob: %v", err)
	}
	status, err = schtasksScheduledStatus(ctx, schtasksTaskName(name))
	if err != nil {
		t.Fatalf("status after stop: %v", err)
	}
	if status.Registered {
		t.Fatalf("status after stop = %#v, want unregistered", status)
	}
}

func TestRemoveScheduledArtifact_retriesBusyThenSucceeds(t *testing.T) {
	origDelay := scheduledRemoveRetryDelay
	scheduledRemoveRetryDelay = 0
	t.Cleanup(func() { scheduledRemoveRetryDelay = origDelay })

	n := 0
	err := removeScheduledArtifactWith(func(string) error {
		n++
		if n < 3 {
			return &os.PathError{
				Op:   "remove",
				Path: "genv-updates.vbs",
				Err:  errors.New("The process cannot access the file because it is being used by another process."),
			}
		}
		return nil
	}, "genv-updates.vbs", time.Second)
	if err != nil {
		t.Fatalf("remove after retries: %v", err)
	}
	if n != 3 {
		t.Fatalf("attempts = %d, want 3", n)
	}
}

func TestRemoveScheduledArtifact_nonBusyFailsImmediately(t *testing.T) {
	n := 0
	err := removeScheduledArtifactWith(func(string) error {
		n++
		return &os.PathError{Op: "remove", Path: "genv-updates.vbs", Err: errors.New("access is denied")}
	}, "genv-updates.vbs", time.Second)
	if err == nil || !strings.Contains(err.Error(), "access is denied") {
		t.Fatalf("error = %v, want access is denied", err)
	}
	if n != 1 {
		t.Fatalf("attempts = %d, want 1 (no retry)", n)
	}
}

func assertSchtasksWindowlessXML(t *testing.T, xml, vbsPath string) {
	t.Helper()
	if !strings.Contains(xml, `<Command>`) || !strings.Contains(strings.ToLower(xml), `wscript.exe</command>`) {
		t.Fatalf("XML = %q, want wscript.exe as the scheduled action", xml)
	}
	if strings.Contains(strings.ToLower(xml), `cmd.exe</command>`) {
		t.Fatalf("XML = %q, cmd.exe as Exec Command allocates a visible console", xml)
	}
	if strings.Contains(xml, `/d /c call`) {
		t.Fatalf("XML = %q, /d /c call is a visible-window argv; it belongs in the hidden VBS host", xml)
	}
	if !strings.Contains(xml, "//B") || !strings.Contains(xml, "//Nologo") {
		t.Fatalf("XML = %q, want wscript //B //Nologo so script dialogs stay off", xml)
	}
	if !strings.Contains(xml, vbsPath) && !strings.Contains(xml, strings.ReplaceAll(vbsPath, `/`, `\`)) {
		t.Fatalf("XML = %q, want vbs path %q", xml, vbsPath)
	}
}

func assertSchtasksWindowlessVbs(t *testing.T, vbs, scriptPath string) {
	t.Helper()
	if !strings.Contains(vbs, `CreateObject("WScript.Shell")`) {
		t.Fatalf("vbs = %q, want WScript.Shell host", vbs)
	}
	if !strings.Contains(vbs, ", 0, True") {
		t.Fatalf("vbs = %q, want WshShell.Run window style 0 (hidden) and wait", vbs)
	}
	if strings.Contains(vbs, ", 1,") || strings.Contains(vbs, ", 5,") {
		t.Fatalf("vbs = %q, window style 1/5 shows a console", vbs)
	}
	if !strings.Contains(vbs, scriptPath) && !strings.Contains(vbs, strings.ReplaceAll(scriptPath, `/`, `\`)) {
		t.Fatalf("vbs = %q, want cmd wrapper path %q", vbs, scriptPath)
	}
	if !strings.Contains(strings.ToLower(vbs), "cmd.exe") {
		t.Fatalf("vbs = %q, want hidden cmd.exe invocation of the existing wrapper", vbs)
	}
}

func withSchtasksRun(t *testing.T, fn func(ctx context.Context, args ...string) ([]byte, error)) {
	t.Helper()
	original := schtasksRun
	schtasksRun = fn
	t.Cleanup(func() { schtasksRun = original })
}

func schtasksListOutput(status, lastRunTime, lastResult string) string {
	return fmt.Sprintf("Folder: \\\r\nHostName:                             QA\r\nTaskName:                             \\genv-updates\r\nNext Run Time:                        8/27/2026 7:22:00 PM\r\nStatus:                               %s\r\nLogon Mode:                           Interactive only\r\nLast Run Time:                        %s\r\nLast Result:                          %s\r\nAuthor:                               QA\\runner\r\n", status, lastRunTime, lastResult)
}

func schtasksCallHas(calls [][]string, want ...string) bool {
	for _, call := range calls {
		if len(call) < len(want) {
			continue
		}
		matched := true
		wi := 0
		for _, arg := range call {
			if wi < len(want) && arg == want[wi] {
				wi++
			}
		}
		if wi != len(want) {
			matched = false
		}
		if matched {
			return true
		}
	}
	return false
}

func bytesToUTF16(b []byte) []uint16 {
	out := make([]uint16, 0, len(b)/2)
	for i := 0; i+1 < len(b); i += 2 {
		out = append(out, binary.LittleEndian.Uint16(b[i:i+2]))
	}
	return out
}

func scheduledStatusEqual(got, want ScheduledJobStatus) bool {
	if got.Supported != want.Supported || got.Registered != want.Registered || got.Executing != want.Executing || got.LastRun != want.LastRun || got.LastRunDetail != want.LastRunDetail || got.Detail != want.Detail {
		return false
	}
	if (got.ExitCode == nil) != (want.ExitCode == nil) {
		return false
	}
	if got.ExitCode != nil && *got.ExitCode != *want.ExitCode {
		return false
	}
	return true
}
