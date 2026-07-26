package hooks

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ks1686/genv/internal/profilebackend"
	"github.com/ks1686/genv/internal/schema"
)

// fakeRunner records command invocations and returns a programmed error.
type fakeRunner struct {
	calls [][]string
	envs  [][]string
	err   error
}

func (f *fakeRunner) Run(_ context.Context, args []string, env []string, _, _ io.Writer) error {
	f.calls = append(f.calls, args)
	f.envs = append(f.envs, env)
	return f.err
}

type blockingRunner struct{}

func (blockingRunner) Run(ctx context.Context, _ []string, _ []string, _, _ io.Writer) error {
	<-ctx.Done()
	return ctx.Err()
}

func newTestExecutor(stdout, stderr io.Writer) (*Executor, *fakeRunner) {
	if stdout == nil {
		stdout = io.Discard
	}
	if stderr == nil {
		stderr = io.Discard
	}
	fr := &fakeRunner{}
	e := &Executor{Stdout: stdout, Stderr: stderr, runner: fr, goos: "linux"}
	return e, fr
}

func TestPreUpgrade_RunsMatchingHooks(t *testing.T) {
	ctx := context.Background()
	e, fr := newTestExecutor(nil, nil)
	hooks := []schema.Hook{
		{Command: "echo hello"},
		{Command: "echo world", Host: schema.HostPredicate{"macos"}},
	}

	if err := e.PreUpgrade(ctx, hooks, "macos", false); err != nil {
		t.Fatalf("PreUpgrade() error = %v, want nil", err)
	}

	if len(fr.calls) != 2 {
		t.Fatalf("got %d calls, want 2", len(fr.calls))
	}
	want0 := []string{"sh", "-c", "echo hello"}
	want1 := []string{"sh", "-c", "echo world"}
	if !slicesEqual(fr.calls[0], want0) {
		t.Errorf("call 0 = %v, want %v", fr.calls[0], want0)
	}
	if !slicesEqual(fr.calls[1], want1) {
		t.Errorf("call 1 = %v, want %v", fr.calls[1], want1)
	}
}

func TestExecutor_ShellArgvPerOS(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name     string
		goos     string
		lookPath func(string) (string, error)
		want     []string
	}{
		{
			name:     "windows uses cmd /C when no PowerShell",
			goos:     "windows",
			lookPath: func(string) (string, error) { return "", os.ErrNotExist },
			want:     []string{"cmd", "/C", "echo hi"},
		},
		{
			name: "windows uses pwsh when available",
			goos: "windows",
			lookPath: func(file string) (string, error) {
				if file == "pwsh" {
					return "/usr/bin/pwsh", nil
				}
				return "", os.ErrNotExist
			},
			want: []string{"/usr/bin/pwsh", "-NoProfile", "-Command", "echo hi"},
		},
		{
			name:     "linux uses sh -c",
			goos:     "linux",
			lookPath: nil, // unused for non-windows
			want:     []string{"sh", "-c", "echo hi"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.lookPath != nil {
				restore := profilebackend.SetLookPathForTest(tc.lookPath)
				t.Cleanup(restore)
			}
			e, fr := newTestExecutor(nil, nil)
			e.goos = tc.goos
			hooks := []schema.Hook{{Command: "echo hi"}}

			if err := e.PreUpgrade(ctx, hooks, "any", false); err != nil {
				t.Fatalf("PreUpgrade() error = %v, want nil", err)
			}

			if len(fr.calls) != 1 {
				t.Fatalf("got %d calls, want 1", len(fr.calls))
			}
			if !slicesEqual(fr.calls[0], tc.want) {
				t.Errorf("call 0 = %v, want %v", fr.calls[0], tc.want)
			}
		})
	}
}

func TestShellFor(t *testing.T) {
	restore := profilebackend.SetLookPathForTest(func(string) (string, error) {
		return "", os.ErrNotExist
	})
	t.Cleanup(restore)

	cases := []struct {
		goos string
		bin  string
		flag string
	}{
		{goos: "windows", bin: "cmd", flag: "/C"},
		{goos: "linux", bin: "sh", flag: "-c"},
		{goos: "darwin", bin: "sh", flag: "-c"},
	}
	for _, tc := range cases {
		bin, flag := shellFor(tc.goos)
		if bin != tc.bin || flag != tc.flag {
			t.Errorf("shellFor(%q) = (%q, %q), want (%q, %q)", tc.goos, bin, flag, tc.bin, tc.flag)
		}
	}
}

func TestHookArgs_WindowsPrefersPwsh(t *testing.T) {
	restore := profilebackend.SetLookPathForTest(func(file string) (string, error) {
		if file == "pwsh" {
			return `/fake/pwsh`, nil
		}
		return "", os.ErrNotExist
	})
	t.Cleanup(restore)

	e, _ := newTestExecutor(nil, nil)
	e.goos = "windows"
	args, err := e.hookArgs(schema.Hook{Command: "Write-Host hi"})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{`/fake/pwsh`, "-NoProfile", "-Command", "Write-Host hi"}
	if len(args) != len(want) {
		t.Fatalf("args = %v, want %v", args, want)
	}
	for i := range want {
		if args[i] != want[i] {
			t.Fatalf("args = %v, want %v", args, want)
		}
	}

	script := filepath.Join(t.TempDir(), "hook.ps1")
	if err := os.WriteFile(script, []byte("Write-Host ok"), 0o644); err != nil {
		t.Fatal(err)
	}
	args, err = e.hookArgs(schema.Hook{File: script})
	if err != nil {
		t.Fatal(err)
	}
	want = []string{`/fake/pwsh`, "-NoProfile", "-File", script}
	if len(args) != len(want) {
		t.Fatalf("script args = %v, want %v", args, want)
	}
	for i := range want {
		if args[i] != want[i] {
			t.Fatalf("script args = %v, want %v", args, want)
		}
	}
}

func TestPostApply_SkipsHooksForOtherHosts(t *testing.T) {
	ctx := context.Background()
	e, fr := newTestExecutor(nil, nil)
	hooks := []schema.Hook{
		{Command: "echo arch", Host: schema.HostPredicate{"arch"}},
		{Command: "echo macos", Host: schema.HostPredicate{"macos"}},
		{Command: "echo all"},
	}

	if err := e.PostApply(ctx, hooks, "arch", false); err != nil {
		t.Fatalf("PostApply() error = %v, want nil", err)
	}

	if len(fr.calls) != 2 {
		t.Fatalf("got %d calls, want 2", len(fr.calls))
	}
	if !strings.Contains(fr.calls[0][2], "arch") {
		t.Errorf("call 0 = %v, want arch hook", fr.calls[0])
	}
	if !strings.Contains(fr.calls[1][2], "all") {
		t.Errorf("call 1 = %v, want all hook", fr.calls[1])
	}
}

func TestPostUpgrade_DryRunPrintsWithoutExecuting(t *testing.T) {
	ctx := context.Background()
	var out bytes.Buffer
	e, fr := newTestExecutor(&out, nil)
	hooks := []schema.Hook{
		{Command: "echo one"},
		{Command: "echo two", Host: schema.HostPredicate{"macos"}},
	}

	if err := e.PostUpgrade(ctx, hooks, "macos", true); err != nil {
		t.Fatalf("PostUpgrade() error = %v, want nil", err)
	}

	if len(fr.calls) != 0 {
		t.Fatalf("dry-run executed %d commands, want 0", len(fr.calls))
	}
	got := out.String()
	if !strings.Contains(got, "echo one") {
		t.Errorf("dry-run output missing echo one: %q", got)
	}
	if !strings.Contains(got, "echo two") {
		t.Errorf("dry-run output missing echo two: %q", got)
	}
}

func TestExecutor_AbortsOnFirstFailure(t *testing.T) {
	ctx := context.Background()
	e, fr := newTestExecutor(nil, nil)
	fr.err = errors.New("boom")
	hooks := []schema.Hook{
		{Command: "echo first"},
		{Command: "echo second"},
	}

	err := e.PreUpgrade(ctx, hooks, "any", false)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "pre-upgrade") {
		t.Errorf("error %q does not mention phase", err.Error())
	}
	if len(fr.calls) != 1 {
		t.Fatalf("got %d calls, want 1 (aborted after failure)", len(fr.calls))
	}
}

func TestExecutor_EmptyHooksIsNoOp(t *testing.T) {
	ctx := context.Background()
	e, fr := newTestExecutor(nil, nil)

	if err := e.PostApply(ctx, nil, "any", false); err != nil {
		t.Fatalf("PostApply(nil) error = %v, want nil", err)
	}
	if len(fr.calls) != 0 {
		t.Fatalf("got %d calls, want 0", len(fr.calls))
	}
}

func TestExecutor_PhaseIncludedInError(t *testing.T) {
	ctx := context.Background()
	e, fr := newTestExecutor(nil, nil)
	fr.err = errors.New("failed")

	err := e.PostApply(ctx, []schema.Hook{{Command: "x"}}, "any", false)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "post-apply") {
		t.Errorf("error %q does not contain post-apply", err.Error())
	}
}

func TestPreUpgrade_WithEnv_passes_additions_without_changing_argv(t *testing.T) {
	ctx := context.Background()
	e, fr := newTestExecutor(nil, nil)
	hooks := []schema.Hook{{Command: "printf '%s' \"$GENV_HOST\""}}
	env := []string{
		"GENV_EVENT=upgrade",
		"GENV_PHASE=pre-upgrade",
		"GENV_HOST=ci-host",
		"GENV_UPGRADE_MANAGERS=brew,bun",
	}

	err := e.PreUpgradeWithOptions(ctx, hooks, RunOptions{Host: "ci-host", Env: env})

	if err != nil {
		t.Fatalf("PreUpgradeWithOptions() error = %v, want nil", err)
	}
	if len(fr.calls) != 1 {
		t.Fatalf("got %d calls, want 1", len(fr.calls))
	}
	wantArgs := []string{"sh", "-c", "printf '%s' \"$GENV_HOST\""}
	if !slicesEqual(fr.calls[0], wantArgs) {
		t.Fatalf("call args = %v, want %v", fr.calls[0], wantArgs)
	}
	if len(fr.envs) != 1 || !slicesEqual(fr.envs[0], env) {
		t.Fatalf("env = %v, want %v", fr.envs, env)
	}
}

func TestPostUpgrade_WithTimeout_returns_actionable_deadline_error(t *testing.T) {
	ctx := context.Background()
	e := &Executor{Stdout: io.Discard, Stderr: io.Discard, runner: blockingRunner{}, goos: "linux"}
	hooks := []schema.Hook{{Command: "sleep 60"}}

	err := e.PostUpgradeWithOptions(ctx, hooks, RunOptions{Host: "any", Timeout: time.Nanosecond})

	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !strings.Contains(err.Error(), "post-upgrade hook timed out after") {
		t.Fatalf("error = %q, want timeout context", err.Error())
	}
}

func TestLifecyclePhaseRunners_use_expected_phase_names(t *testing.T) {
	tests := []struct {
		name    string
		run     func(context.Context, *Executor, []schema.Hook) error
		wantArg string
	}{
		{name: "pre apply", run: func(ctx context.Context, e *Executor, hs []schema.Hook) error {
			return e.PreApplyWithOptions(ctx, hs, RunOptions{Host: "any"})
		}, wantArg: "pre-apply"},
		{name: "post apply", run: func(ctx context.Context, e *Executor, hs []schema.Hook) error {
			return e.PostApplyWithOptions(ctx, hs, RunOptions{Host: "any"})
		}, wantArg: "post-apply"},
		{name: "pre add", run: func(ctx context.Context, e *Executor, hs []schema.Hook) error {
			return e.PreAddWithOptions(ctx, hs, RunOptions{Host: "any"})
		}, wantArg: "pre-add"},
		{name: "post add", run: func(ctx context.Context, e *Executor, hs []schema.Hook) error {
			return e.PostAddWithOptions(ctx, hs, RunOptions{Host: "any"})
		}, wantArg: "post-add"},
		{name: "pre remove", run: func(ctx context.Context, e *Executor, hs []schema.Hook) error {
			return e.PreRemoveWithOptions(ctx, hs, RunOptions{Host: "any"})
		}, wantArg: "pre-remove"},
		{name: "post remove", run: func(ctx context.Context, e *Executor, hs []schema.Hook) error {
			return e.PostRemoveWithOptions(ctx, hs, RunOptions{Host: "any"})
		}, wantArg: "post-remove"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Given
			ctx := context.Background()
			e, fr := newTestExecutor(nil, nil)

			// When
			err := tc.run(ctx, e, []schema.Hook{{Command: "echo ok"}})

			// Then
			if err != nil {
				t.Fatalf("runner returned error: %v", err)
			}
			if len(fr.calls) != 1 || !slicesEqual(fr.calls[0], []string{"sh", "-c", "echo ok"}) {
				t.Fatalf("call = %v, want inline shell argv", fr.calls)
			}
		})
	}
}

func TestHookFile_executes_resolved_script_as_argv_element(t *testing.T) {
	// Given
	ctx := context.Background()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	scriptPath := filepath.Join(dir, "hook.sh")
	if err := os.WriteFile(scriptPath, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write script: %v", err)
	}
	e, fr := newTestExecutor(nil, nil)

	// When
	err := e.PostApplyWithOptions(ctx, []schema.Hook{{File: "~/hook.sh"}}, RunOptions{Host: "any"})

	// Then
	if err != nil {
		t.Fatalf("PostApplyWithOptions(file) error = %v, want nil", err)
	}
	if len(fr.calls) != 1 {
		t.Fatalf("got %d calls, want 1", len(fr.calls))
	}
	want := []string{"sh", scriptPath}
	if !slicesEqual(fr.calls[0], want) {
		t.Fatalf("call args = %v, want %v", fr.calls[0], want)
	}
}

func TestHookFile_missing_script_returns_actionable_error(t *testing.T) {
	// Given
	ctx := context.Background()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	e, fr := newTestExecutor(nil, nil)

	// When
	err := e.PostApplyWithOptions(ctx, []schema.Hook{{File: "~/missing.sh"}}, RunOptions{Host: "any"})

	// Then
	if err == nil {
		t.Fatal("expected missing script error")
	}
	if !strings.Contains(err.Error(), "hook script") || !strings.Contains(err.Error(), "missing.sh") {
		t.Fatalf("error = %q, want actionable missing script", err.Error())
	}
	if len(fr.calls) != 0 {
		t.Fatalf("runner calls = %v, want none for missing script", fr.calls)
	}
}

func slicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
