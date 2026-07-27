package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/ks1686/genv/internal/genvfile"
	"github.com/ks1686/genv/internal/schema"
	"github.com/ks1686/genv/internal/service"
)

const updatesServiceName = "updates"

type updatesSupervisor interface {
	Supported() bool
	Start(ctx context.Context, job service.ScheduledJob) error
	Stop(ctx context.Context, name string) error
	Status(ctx context.Context, name string) (service.ScheduledJobStatus, error)
}

var (
	newUpdatesSupervisor = func() updatesSupervisor { return service.NewScheduledBackend() }
	updatesExecutable    = os.Executable
)

func updatesStartCmd(args []string) int {
	fs := flag.NewFlagSet("updates start", flag.ContinueOnError)
	fs.Usage = func() {
		fPrintln(os.Stderr, "usage: genv updates start [flags]")
		fPrintln(os.Stderr)
		fPrintln(os.Stderr, "flags:")
		fs.PrintDefaults()
	}
	file := fs.String("file", defaultSpecPath(), "path to genv.json")
	lockFile := fs.String("lock-file", "", "path to genv lock file")
	hostFlag := fs.String("host", "", "host name for host-specific records (defaults to $GENV_HOST or os.Hostname())")
	targetFlag := fs.String("target", "", "portable target id for schemaVersion 8 specs (defaults to $GENV_TARGET or host classification)")
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	cfg, interval, code := readUpdatesConfigForLifecycle(*file)
	if code != exitOK {
		return code
	}
	backend := newUpdatesSupervisor()
	if !backend.Supported() {
		fPrintln(os.Stderr, "genv updates start: not supported on this platform (requires systemd --user on Linux or launchd on macOS)")
		return exitLogic
	}
	exe, err := updatesExecutable()
	if err != nil {
		fprintf(os.Stderr, "genv updates start: finding current executable: %v\n", err)
		return exitIO
	}
	absFile, err := filepath.Abs(*file)
	if err != nil {
		fprintf(os.Stderr, "genv updates start: resolving spec path: %v\n", err)
		return exitIO
	}
	lockPath := lockPathForSpec(*file, *lockFile)
	command := []string{exe, "updates", "__run-once", "--file", absFile, "--lock-file", lockPath, "--host", hostForCommand(*hostFlag)}
	if strings.TrimSpace(*targetFlag) != "" {
		command = append(command, "--target", strings.TrimSpace(*targetFlag))
	}
	pathValue := os.Getenv("PATH")
	goos := runtime.GOOS
	environment := map[string]string{"PATH": service.ScheduledPath(pathValue, goos)}
	if target := strings.TrimSpace(os.Getenv("GENV_TARGET")); target != "" && strings.TrimSpace(*targetFlag) == "" {
		environment["GENV_TARGET"] = target
	}
	if err := backend.Start(context.Background(), service.ScheduledJob{Name: updatesServiceName, Command: command, Interval: interval, Environment: environment}); err != nil {
		fprintf(os.Stderr, "genv updates start: %v\n", err)
		return exitIO
	}
	mode := "check/log/notify only"
	if cfg.AutoApply {
		mode = "auto-apply enabled by updates.autoApply:true"
	}
	fprintf(os.Stdout, "updates checker started (%s, interval %s). Log: %s\n", mode, cfg.Interval, updatesLogPathOrFallback())
	return exitOK
}

func updatesStopCmd(args []string) int {
	fs := flag.NewFlagSet("updates stop", flag.ContinueOnError)
	fs.Usage = func() { fPrintln(os.Stderr, "usage: genv updates stop") }
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	backend := newUpdatesSupervisor()
	if !backend.Supported() {
		fPrintln(os.Stdout, "updates checker is not supported on this platform; nothing to stop.")
		return exitOK
	}
	if err := backend.Stop(context.Background(), updatesServiceName); err != nil {
		fprintf(os.Stderr, "genv updates stop: %v\n", err)
		return exitIO
	}
	fPrintln(os.Stdout, "updates checker stopped.")
	return exitOK
}

func updatesStatusCmd(args []string) int {
	fs := flag.NewFlagSet("updates status", flag.ContinueOnError)
	fs.Usage = func() { fPrintln(os.Stderr, "usage: genv updates status") }
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	backend := newUpdatesSupervisor()
	status, err := backend.Status(context.Background(), updatesServiceName)
	if err != nil {
		fprintf(os.Stderr, "genv updates status: %v\n", err)
		return exitIO
	}
	if !status.Supported {
		fPrintln(os.Stdout, "updates checker is not supported on this platform (requires systemd --user on Linux or launchd on macOS); not running.")
		return exitOK
	}
	if !status.Registered {
		fprintf(os.Stdout, "updates checker is not registered (%s).\n", status.Detail)
		return exitOK
	}
	if status.Executing {
		fprintf(os.Stdout, "updates checker is registered and executing (%s).\n", status.Detail)
		return exitOK
	}
	switch status.LastRun {
	case service.ScheduledRunSuccess:
		fprintf(os.Stdout, "updates checker is registered and idle; last run succeeded%s (%s).\n", scheduledRunStatusDetail(status), status.Detail)
	case service.ScheduledRunFailure:
		fprintf(os.Stdout, "updates checker is registered and idle; last run failed%s (%s).\n", scheduledRunStatusDetail(status), status.Detail)
	case service.ScheduledRunUnknown, "":
		fprintf(os.Stdout, "updates checker is registered and idle; no completed run is known (%s).\n", status.Detail)
	default:
		fprintf(os.Stdout, "updates checker is registered and idle; last-run outcome is unknown (%s).\n", status.Detail)
	}
	return exitOK
}

func scheduledRunStatusDetail(status service.ScheduledJobStatus) string {
	parts := make([]string, 0, 2)
	if status.ExitCode != nil {
		parts = append(parts, fmt.Sprintf("status %d", *status.ExitCode))
	}
	if status.LastRunDetail != "" {
		parts = append(parts, "reason "+status.LastRunDetail)
	}
	if len(parts) == 0 {
		return ""
	}
	return " (" + strings.Join(parts, "; ") + ")"
}

func readUpdatesConfigForLifecycle(file string) (*schema.UpdatesConfig, time.Duration, int) {
	f, err := genvfile.Read(file)
	if err != nil {
		if errors.Is(err, genvfile.ErrNotFound) {
			fprintf(os.Stderr, "genv updates start: %s not found — add an updates block to genv.json, for example {\"schemaVersion\":\"6\",\"updates\":{\"enabled\":true,\"interval\":\"24h\"}}\n", file)
			return nil, 0, exitIO
		}
		fprintf(os.Stderr, "genv updates start: %v\n", err)
		if errors.Is(err, genvfile.ErrInvalidFile) {
			fPrintln(os.Stderr, "Hint: add an enabled updates block with a positive interval, e.g. {\"schemaVersion\":\"6\",\"updates\":{\"enabled\":true,\"interval\":\"24h\"}}")
			return nil, 0, exitValidation
		}
		return nil, 0, exitIO
	}
	cfg, interval, cfgErr := parseEnabledUpdatesConfig(f)
	if cfgErr != nil {
		fprintf(os.Stderr, "genv updates start: %v\nHint: add an enabled updates block with a positive interval, e.g. {\"schemaVersion\":\"6\",\"updates\":{\"enabled\":true,\"interval\":\"24h\"}}\n", cfgErr)
		return nil, 0, exitValidation
	}
	return cfg, interval, exitOK
}

func parseEnabledUpdatesConfig(f *schema.GenvFile) (*schema.UpdatesConfig, time.Duration, error) {
	cfg := f.Updates
	if cfg == nil {
		return nil, 0, fmt.Errorf("updates block is missing")
	}
	if !cfg.Enabled {
		return nil, 0, fmt.Errorf("updates.enabled is false")
	}
	interval, err := time.ParseDuration(cfg.Interval)
	if err != nil || interval <= 0 {
		if err != nil {
			return nil, 0, fmt.Errorf("updates.interval %q is invalid: %w", cfg.Interval, err)
		}
		return nil, 0, fmt.Errorf("updates.interval %q must be positive", cfg.Interval)
	}
	return cfg, interval, nil
}
