// Package hooks executes lifecycle shell commands declared in genv.json v5.
package hooks

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/ks1686/genv/internal/host"
	"github.com/ks1686/genv/internal/schema"
)

// Executor runs lifecycle hooks from a genv.json spec.
//
// Output produced during hook execution is written to Stdout and Stderr.
// In dry-run mode the command that would run is written to Stdout instead of
// being executed.
type Executor struct {
	Stdout io.Writer
	Stderr io.Writer

	// runner abstracts command execution so tests can verify behavior without
	// spawning real subprocesses.
	runner commandRunner

	// goos selects the shell used to run hook commands. It defaults to
	// runtime.GOOS (set by NewExecutor); tests override it to exercise the
	// per-OS shell selection without spawning real subprocesses.
	goos string
}

type commandRunner interface {
	Run(ctx context.Context, args []string, env []string, stdout, stderr io.Writer) error
}

type execRunner struct{}

func (execRunner) Run(ctx context.Context, args []string, env []string, stdout, stderr io.Writer) error {
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	if len(env) > 0 {
		cmd.Env = append(os.Environ(), env...)
	}
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	return cmd.Run()
}

// RunOptions configures one hook phase without changing the command string.
type RunOptions struct {
	Host    string
	DryRun  bool
	Env     []string
	Timeout time.Duration
}

func fprintf(w io.Writer, format string, a ...any) { _, _ = fmt.Fprintf(w, format, a...) }

// shellFor returns the shell binary and its command flag for the given GOOS.
// Native Windows has no "sh", so hooks run via "cmd /C"; every other OS uses
// the POSIX "sh -c".
func shellFor(goos string) (bin string, flag string) {
	if goos == "windows" {
		return "cmd", "/C"
	}
	return "sh", "-c"
}

func scriptRunnerFor(goos string) []string {
	if goos == "windows" {
		return []string{"cmd", "/C"}
	}
	return []string{"sh"}
}

// NewExecutor returns an Executor that writes subprocess output to stdout and stderr.
func NewExecutor(stdout, stderr io.Writer) *Executor {
	if stdout == nil {
		stdout = io.Discard
	}
	if stderr == nil {
		stderr = io.Discard
	}
	return &Executor{
		Stdout: stdout,
		Stderr: stderr,
		runner: execRunner{},
		goos:   runtime.GOOS,
	}
}

// PreUpgrade runs the pre-upgrade hooks.
func (e *Executor) PreUpgrade(ctx context.Context, hooks []schema.Hook, host string, dryRun bool) error {
	return e.PreUpgradeWithOptions(ctx, hooks, RunOptions{Host: host, DryRun: dryRun})
}

func (e *Executor) PreApplyWithOptions(ctx context.Context, hooks []schema.Hook, opts RunOptions) error {
	return e.runPhase(ctx, "pre-apply", hooks, opts)
}

// PreUpgradeWithOptions runs the pre-upgrade hooks with explicit execution options.
func (e *Executor) PreUpgradeWithOptions(ctx context.Context, hooks []schema.Hook, opts RunOptions) error {
	return e.runPhase(ctx, "pre-upgrade", hooks, opts)
}

func (e *Executor) PreAddWithOptions(ctx context.Context, hooks []schema.Hook, opts RunOptions) error {
	return e.runPhase(ctx, "pre-add", hooks, opts)
}

func (e *Executor) PostAddWithOptions(ctx context.Context, hooks []schema.Hook, opts RunOptions) error {
	return e.runPhase(ctx, "post-add", hooks, opts)
}

func (e *Executor) PreRemoveWithOptions(ctx context.Context, hooks []schema.Hook, opts RunOptions) error {
	return e.runPhase(ctx, "pre-remove", hooks, opts)
}

func (e *Executor) PostRemoveWithOptions(ctx context.Context, hooks []schema.Hook, opts RunOptions) error {
	return e.runPhase(ctx, "post-remove", hooks, opts)
}

// PostApply runs the post-apply hooks.
func (e *Executor) PostApply(ctx context.Context, hooks []schema.Hook, host string, dryRun bool) error {
	return e.PostApplyWithOptions(ctx, hooks, RunOptions{Host: host, DryRun: dryRun})
}

func (e *Executor) PostApplyWithOptions(ctx context.Context, hooks []schema.Hook, opts RunOptions) error {
	return e.runPhase(ctx, "post-apply", hooks, opts)
}

// PostUpgrade runs the post-upgrade hooks.
func (e *Executor) PostUpgrade(ctx context.Context, hooks []schema.Hook, host string, dryRun bool) error {
	return e.PostUpgradeWithOptions(ctx, hooks, RunOptions{Host: host, DryRun: dryRun})
}

// PostUpgradeWithOptions runs the post-upgrade hooks with explicit execution options.
func (e *Executor) PostUpgradeWithOptions(ctx context.Context, hooks []schema.Hook, opts RunOptions) error {
	return e.runPhase(ctx, "post-upgrade", hooks, opts)
}

func (e *Executor) runPhase(ctx context.Context, phase string, hooks []schema.Hook, opts RunOptions) error {
	if e.runner == nil {
		e.runner = execRunner{}
	}
	if e.goos == "" {
		e.goos = runtime.GOOS
	}
	if e.Stdout == nil {
		e.Stdout = io.Discard
	}
	if e.Stderr == nil {
		e.Stderr = io.Discard
	}

	for i, h := range hooks {
		if !host.Match(h.Host, opts.Host) {
			slog.Debug("skipping hook for host", "phase", phase, "index", i, "command", h.Command, "host", opts.Host)
			continue
		}

		desc := hookDesc(h)
		if opts.DryRun {
			fprintf(e.Stdout, "[%s] [dry-run] %s\n", phase, desc)
			slog.Debug("would run hook", "phase", phase, "index", i, "command", h.Command)
			continue
		}

		slog.Debug("running hook", "phase", phase, "index", i, "hook", desc)
		args, err := e.hookArgs(h)
		if err != nil {
			return fmt.Errorf("%s hook %s: %w", phase, desc, err)
		}
		runCtx := ctx
		cancel := func() {}
		if opts.Timeout > 0 {
			runCtx, cancel = context.WithTimeout(ctx, opts.Timeout)
		}
		err = e.runner.Run(runCtx, args, opts.Env, e.Stdout, e.Stderr)
		cancel()
		if errors.Is(runCtx.Err(), context.DeadlineExceeded) {
			return fmt.Errorf("%s hook timed out after %s %s: %w", phase, opts.Timeout, desc, context.DeadlineExceeded)
		}
		if err != nil {
			return fmt.Errorf("%s hook %s: %w", phase, desc, err)
		}
	}

	return nil
}

func (e *Executor) hookArgs(h schema.Hook) ([]string, error) {
	if h.File != "" {
		path, err := expandPath(h.File)
		if err != nil {
			return nil, err
		}
		if info, err := os.Stat(path); err != nil {
			return nil, fmt.Errorf("hook script %s: %w", path, err)
		} else if info.IsDir() {
			return nil, fmt.Errorf("hook script %s is a directory", path)
		}
		args := scriptRunnerFor(e.goos)
		return append(args, path), nil
	}
	bin, flag := shellFor(e.goos)
	return []string{bin, flag, h.Command}, nil
}

func hookDesc(h schema.Hook) string {
	if h.File != "" {
		return "file " + h.File
	}
	return fmt.Sprintf("%q", h.Command)
}

func expandPath(raw string) (string, error) {
	path := raw
	if strings.HasPrefix(path, "~") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("expand home: %w", err)
		}
		path = filepath.Join(home, strings.TrimPrefix(path, "~"))
	}
	return os.Expand(path, os.Getenv), nil
}
