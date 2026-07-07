// Package hooks executes lifecycle shell commands declared in genv.json v5.
package hooks

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os/exec"
	"runtime"

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
	Run(ctx context.Context, args []string, stdout, stderr io.Writer) error
}

type execRunner struct{}

func (execRunner) Run(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	return cmd.Run()
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
	return e.runPhase(ctx, "pre-upgrade", hooks, host, dryRun)
}

// PostApply runs the post-apply hooks.
func (e *Executor) PostApply(ctx context.Context, hooks []schema.Hook, host string, dryRun bool) error {
	return e.runPhase(ctx, "post-apply", hooks, host, dryRun)
}

// PostUpgrade runs the post-upgrade hooks.
func (e *Executor) PostUpgrade(ctx context.Context, hooks []schema.Hook, host string, dryRun bool) error {
	return e.runPhase(ctx, "post-upgrade", hooks, host, dryRun)
}

func (e *Executor) runPhase(ctx context.Context, phase string, hooks []schema.Hook, hostName string, dryRun bool) error {
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
		if !host.Match(h.Host, hostName) {
			slog.Debug("skipping hook for host", "phase", phase, "index", i, "command", h.Command, "host", hostName)
			continue
		}

		if dryRun {
			fprintf(e.Stdout, "[%s] [dry-run] %s\n", phase, h.Command)
			slog.Debug("would run hook", "phase", phase, "index", i, "command", h.Command)
			continue
		}

		slog.Info("running hook", "phase", phase, "index", i, "command", h.Command)
		bin, flag := shellFor(e.goos)
		if err := e.runner.Run(ctx, []string{bin, flag, h.Command}, e.Stdout, e.Stderr); err != nil {
			return fmt.Errorf("%s hook %q: %w", phase, h.Command, err)
		}
	}

	return nil
}
