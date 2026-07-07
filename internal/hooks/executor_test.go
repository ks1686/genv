package hooks

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/ks1686/genv/internal/schema"
)

// fakeRunner records command invocations and returns a programmed error.
type fakeRunner struct {
	calls [][]string
	err   error
}

func (f *fakeRunner) Run(_ context.Context, args []string, _, _ io.Writer) error {
	f.calls = append(f.calls, args)
	return f.err
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
		name string
		goos string
		bin  string
		flag string
	}{
		{name: "windows uses cmd /C", goos: "windows", bin: "cmd", flag: "/C"},
		{name: "linux uses sh -c", goos: "linux", bin: "sh", flag: "-c"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e, fr := newTestExecutor(nil, nil)
			e.goos = tc.goos
			hooks := []schema.Hook{{Command: "echo hi"}}

			if err := e.PreUpgrade(ctx, hooks, "any", false); err != nil {
				t.Fatalf("PreUpgrade() error = %v, want nil", err)
			}

			if len(fr.calls) != 1 {
				t.Fatalf("got %d calls, want 1", len(fr.calls))
			}
			want := []string{tc.bin, tc.flag, "echo hi"}
			if !slicesEqual(fr.calls[0], want) {
				t.Errorf("call 0 = %v, want %v", fr.calls[0], want)
			}
		})
	}
}

func TestShellFor(t *testing.T) {
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
