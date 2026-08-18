package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/ks1686/genv/internal/genvfile"
	"github.com/ks1686/genv/internal/output"
	"github.com/ks1686/genv/internal/resolver"
	"github.com/ks1686/genv/internal/schema"
	"github.com/ks1686/genv/internal/service"
	"github.com/ks1686/genv/internal/upgrade"
)

var (
	updatesBuildPlan  = upgrade.BuildUpgradePlan
	updatesRunUpgrade = upgrade.RunUpgrade
	updatesLookPath   = exec.LookPath
)

func updatesRunOnceCmd(args []string) int {
	fs := flag.NewFlagSet("updates __run-once", flag.ContinueOnError)
	file := fs.String("file", defaultSpecPath(), "path to genv.json")
	lockFile := fs.String("lock-file", "", "path to genv lock file")
	hostFlag := fs.String("host", "", "host name for host-specific records")
	targetFlag := fs.String("target", "", "portable target id for schemaVersion 8 specs")
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	logger, closeLog, err := updatesLogger()
	if err != nil {
		fPrintln(os.Stderr, "genv updates: audit log unavailable")
		return exitIO
	}
	defer closeLog()
	f, err := genvfile.Read(*file)
	if err != nil {
		logger.Warn("updates.check.config", slog.Any("err", err))
		return exitValidation
	}
	cfg, interval, cfgErr := parseEnabledUpdatesConfig(f)
	if cfgErr != nil {
		logger.Warn("updates.check.config", slog.Any("err", cfgErr))
		return exitValidation
	}
	ctx, cancel := context.WithTimeout(context.Background(), service.ScheduledJobTimeOut(interval))
	defer cancel()

	done := make(chan int, 1)
	go func() {
		done <- updatesRunOnceBody(ctx, logger, f, cfg, *file, *lockFile, *hostFlag, *targetFlag)
	}()
	select {
	case code := <-done:
		return code
	case <-ctx.Done():
		// Prefer a late completion over a false timeout when both are ready.
		select {
		case code := <-done:
			return code
		default:
			logger.Warn("updates.check.timeout", slog.Duration("timeout", service.ScheduledJobTimeOut(interval)), slog.Any("err", ctx.Err()))
			return exitLogic
		}
	}
}

func updatesRunOnceBody(ctx context.Context, logger *slog.Logger, f *schema.GenvFile, cfg *schema.UpdatesConfig, file, lockFile, hostFlag, targetFlag string) int {
	f, activeTarget, code := materializeSpecForCommand("updates", file, f, hostFlag, targetFlag)
	if code != exitOK {
		logger.Warn("updates.check.target", slog.Int("exit", code))
		return code
	}
	lockPath := lockPathForSpec(file, lockFile)
	lf, err := genvfile.ReadLock(lockPath)
	if err != nil {
		logger.Warn("updates.check.lock", slog.Any("err", err))
		return exitIO
	}
	if f.SchemaVersion == schema.Version8 {
		available := resolver.Detect()
		_, code := applyLockGate("updates", lockPath, lf, activeTarget, available, true, false, true, "")
		if code != exitOK {
			logger.Warn("updates.check.lock", slog.Int("exit", code), slog.String("reason", "foreign lock refused"))
			return code
		}
	}
	filters := output.UpgradeFilters{Only: cfg.Only, Skip: cfg.Skip, OnlyManager: cfg.OnlyManagers, SkipManager: cfg.SkipManagers, HooksSkipped: true}
	planStarted := time.Now()
	plan, err := updatesBuildPlan(upgrade.UpgradeOptions{Spec: f, Lock: lf, Filters: filters})
	planDur := time.Since(planStarted).Round(time.Millisecond)
	if err != nil {
		logger.Warn("updates.check.plan", slog.Any("err", err), slog.Duration("duration", planDur))
		return exitUsage
	}
	for _, warn := range plan.Warnings {
		logger.Info("updates.check.warning", slog.String("warning", warn))
	}
	if err := ctx.Err(); err != nil {
		logger.Warn("updates.check.timeout", slog.Any("err", err), slog.Duration("duration", planDur))
		return exitLogic
	}
	if !cfg.AutoApply {
		outdated := countPlannedPackages(plan)
		logger.Info("updates.check.planned", slog.Int("packages", outdated), slog.Int("batches", len(plan.Actions)), slog.Int("skipped", len(plan.Skipped)), slog.Bool("auto_apply", false), slog.Duration("duration", planDur))
		if outdated > 0 {
			notifyUpdates(ctx, cfg.Notify, "genv updates", fmt.Sprintf("%d package(s) have updates available", outdated), logger)
		}
		return exitOK
	}
	diagnostics := newUpdatesDiagnosticWriter(updatesDiagnosticLimit)
	runResult := updatesRunUpgrade(ctx, upgrade.UpgradeRunOptions{Plan: plan, Lock: lf, LockPath: lockPath, Stdin: strings.NewReader(""), Stdout: io.Discard, Stderr: diagnostics})
	matchedErrors := make([]bool, len(runResult.Errors))
	for _, failure := range runResult.Failures {
		sanitizedIDs := make([]string, len(failure.IDs))
		for i, id := range failure.IDs {
			sanitizedIDs[i] = sanitizeAndBoundUpdatesDiagnostic(id, updatesDiagnosticLimit)
		}
		logger.Warn("updates.apply.failed", slog.Any("ids", sanitizedIDs), slog.String("err", sanitizeAndBoundUpdatesDiagnostic(failure.Err.Error(), updatesDiagnosticLimit)))
		for i, runErr := range runResult.Errors {
			if !matchedErrors[i] && errors.Is(runErr, failure.Err) {
				matchedErrors[i] = true
				break
			}
		}
	}
	for i, runErr := range runResult.Errors {
		if !matchedErrors[i] {
			logger.Warn("updates.apply.failed", slog.Any("ids", []string(nil)), slog.String("err", sanitizeAndBoundUpdatesDiagnostic(runErr.Error(), updatesDiagnosticLimit)))
		}
	}
	if rendered := strings.TrimSpace(sanitizeAndBoundUpdatesDiagnostic(diagnostics.String(), updatesDiagnosticLimit)); rendered != "" {
		logger.Warn("updates.apply.diagnostics", slog.String("diagnostics", rendered))
	}
	logger.Info("updates.apply.completed", slog.Int("upgraded", len(runResult.Upgraded)), slog.Int("errors", len(runResult.Errors)), slog.Bool("auto_apply", true))
	notifyUpdates(ctx, cfg.Notify, "genv updates", fmt.Sprintf("auto-apply completed: %d upgraded, %d error(s)", len(runResult.Upgraded), len(runResult.Errors)), logger)
	if runResult.LockWriteError != nil {
		logger.Warn("updates.apply.lock", slog.Any("err", runResult.LockWriteError))
		return exitIO
	}
	if len(runResult.Errors) > 0 {
		return exitLogic
	}
	return exitOK
}

func updatesLogger() (*slog.Logger, func(), error) {
	path, err := updatesLogPath()
	if err != nil {
		return nil, nil, err
	}
	if err := rotateUpdatesLog(path); err != nil {
		return nil, nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, nil, fmt.Errorf("opening updates log: %w", err)
	}
	return slog.New(slog.NewTextHandler(f, nil)), func() { _ = f.Close() }, nil
}

func updatesLogPath() (string, error) {
	dir, err := genvfile.DefaultDir()
	if err != nil {
		return "", fmt.Errorf("locating updates log directory: %w", err)
	}
	return filepath.Join(dir, "updates.log"), nil
}

func updatesLogPathOrFallback() string {
	path, err := updatesLogPath()
	if err != nil {
		return "updates.log"
	}
	return path
}

func rotateUpdatesLog(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("creating updates log directory: %w", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("stat updates log: %w", err)
	}
	if info.Size() <= 1<<20 {
		return nil
	}
	if err := os.Rename(path, path+".1"); err != nil {
		return fmt.Errorf("rotate updates log: %w", err)
	}
	return nil
}

// countPlannedPackages returns the total number of packages across all planned
// upgrade actions. A batched action (e.g. one `brew upgrade a b c`) holds many
// packages, so counting actions would undercount; the notification reports
// packages, not batches.
func countPlannedPackages(plan upgrade.UpgradePlan) int {
	n := 0
	for _, action := range plan.Actions {
		n += len(action.LPs)
	}
	return n
}

func notifyUpdates(_ context.Context, enabled bool, title, message string, logger *slog.Logger) {
	if !enabled {
		return
	}
	var notifier string
	var args []string
	switch {
	case service.IsLaunchdAvailable():
		notifier = "osascript"
		args = []string{"-e", fmt.Sprintf("display notification %q with title %q", message, title)}
	default:
		notifier = "notify-send"
		args = []string{title, message}
	}
	path, err := updatesLookPath(notifier)
	if err != nil {
		logger.Warn("updates.notify.unavailable", slog.String("notifier", notifier), slog.Any("err", err))
		return
	}
	// Never block the scheduled worker on a desktop notification. Under launchd,
	// osascript has been observed to ignore CommandContext cancel and hold the
	// process until the job deadline (exit 4) even after a successful plan.
	go runNotificationAsync(logger, notifier, path, args...)
}

func runNotificationAsync(logger *slog.Logger, notifier, path string, args ...string) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := exec.CommandContext(ctx, path, args...).Run(); err != nil {
		logger.Warn("updates.notify.failed", slog.String("notifier", notifier), slog.Any("err", err))
	}
}
