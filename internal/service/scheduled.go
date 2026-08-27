package service

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// ScheduledJob is a one-shot command managed by the host user supervisor on a fixed interval.
type ScheduledJob struct {
	Name        string
	Command     []string
	Interval    time.Duration
	Environment map[string]string
}

// ScheduledJobStatus reports supervisor registration, current activity, and the last known completed run.
type ScheduledJobStatus struct {
	Supported     bool
	Registered    bool
	Executing     bool
	LastRun       ScheduledRunOutcome
	ExitCode      *int
	LastRunDetail string
	Detail        string
}

// ScheduledBackend manages one-shot scheduled jobs via systemd --user, launchd, or Windows Task Scheduler.
type ScheduledBackend interface {
	Supported() bool
	Start(ctx context.Context, job ScheduledJob) error
	Stop(ctx context.Context, name string) error
	Status(ctx context.Context, name string) (ScheduledJobStatus, error)
}

type hostScheduledBackend struct{}

// NewScheduledBackend returns the platform supervisor backend for scheduled user jobs.
func NewScheduledBackend() ScheduledBackend {
	return hostScheduledBackend{}
}

func (hostScheduledBackend) Supported() bool {
	return IsSystemdAvailable() || IsLaunchdAvailable() || IsSchtasksAvailable()
}

func (hostScheduledBackend) Start(ctx context.Context, job ScheduledJob) error {
	switch {
	case IsSystemdAvailable():
		return startSystemdScheduledJob(ctx, job)
	case IsLaunchdAvailable():
		return startLaunchdScheduledJob(ctx, job)
	case IsSchtasksAvailable():
		return startSchtasksScheduledJob(ctx, job)
	default:
		return fmt.Errorf("scheduled jobs are not supported on this platform: systemd --user, launchd, or Task Scheduler (schtasks) is required")
	}
}

func (hostScheduledBackend) Stop(ctx context.Context, name string) error {
	switch {
	case IsSystemdAvailable():
		return stopSystemdScheduledJob(ctx, name)
	case IsLaunchdAvailable():
		return stopLaunchdScheduledJob(ctx, name)
	case IsSchtasksAvailable():
		return stopSchtasksScheduledJob(ctx, name)
	default:
		return nil
	}
}

func (hostScheduledBackend) Status(ctx context.Context, name string) (ScheduledJobStatus, error) {
	switch {
	case IsSystemdAvailable():
		return systemdScheduledStatus(ctx, systemdTimerName(name), systemdScheduledUnitName(name))
	case IsLaunchdAvailable():
		return launchdScheduledStatus(ctx, launchdScheduledLabel(name))
	case IsSchtasksAvailable():
		return schtasksScheduledStatus(ctx, schtasksTaskName(name))
	default:
		return ScheduledJobStatus{Supported: false, LastRun: ScheduledRunUnknown, Detail: "systemd --user, launchd, or Task Scheduler (schtasks) is required"}, nil
	}
}

func SystemdScheduledUnitContent(name string, command []string, environment map[string]string) string {
	name = stripLineBreaks(name)
	return fmt.Sprintf(`[Unit]
Description=genv scheduled job: %s

[Service]
Type=oneshot
TimeoutStartSec=%d
%sExecStart=%s
`, name, scheduledJobTimeOutSeconds(24*time.Hour), renderSystemdEnvironment(environment), renderSystemdCommand(command))
}

func SystemdScheduledTimerContent(name string, interval time.Duration) string {
	name = stripLineBreaks(name)
	seconds := max(int(interval.Seconds()), 1)
	return fmt.Sprintf(`[Unit]
Description=genv scheduled job timer: %s

[Timer]
OnBootSec=1m
OnUnitActiveSec=%ds
Unit=%s

[Install]
WantedBy=timers.target
`, name, seconds, systemdScheduledUnitName(name))
}

func LaunchdScheduledPlistContent(name string, command []string, interval time.Duration, environment map[string]string) string {
	name = stripLineBreaks(name)
	seconds := max(int(interval.Seconds()), 1)
	var b strings.Builder
	for _, arg := range command {
		fmt.Fprintf(&b, "        <string>%s</string>\n", xmlEscape(stripLineBreaks(arg)))
	}
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>%s</string>
    <key>ProgramArguments</key>
    <array>
%s    </array>
%s    <key>RunAtLoad</key>
    <true/>
    <key>StartInterval</key>
    <integer>%d</integer>
    <key>TimeOut</key>
    <integer>%d</integer>
</dict>
</plist>
`, xmlEscape(launchdScheduledLabel(name)), b.String(), renderLaunchdEnvironment(environment), seconds, scheduledJobTimeOutSeconds(interval))
}

// ScheduledJobTimeOut is the wall-clock budget for a scheduled updates run.
// Matches launchd TimeOut / systemd TimeoutStartSec / Task Scheduler
// ExecutionTimeLimit so Go can exit before the supervisor kills a wedged process.
func ScheduledJobTimeOut(interval time.Duration) time.Duration {
	return time.Duration(scheduledJobTimeOutSeconds(interval)) * time.Second
}

func scheduledJobTimeOutSeconds(interval time.Duration) int {
	const minTimeout = 60
	const maxTimeout = 300 // 5 minutes
	sec := int(interval.Seconds()) / 2
	if sec < minTimeout {
		sec = minTimeout
	}
	if sec > maxTimeout {
		sec = maxTimeout
	}
	return sec
}

func systemdScheduledUnitName(name string) string {
	return systemdUnitName(name)
}

func systemdTimerName(name string) string {
	base := strings.TrimSuffix(systemdUnitName(name), ".service")
	return base + ".timer"
}

func launchdScheduledLabel(name string) string {
	return strings.TrimSuffix(launchdPlistName(name), ".plist")
}

func startSystemdScheduledJob(ctx context.Context, job ScheduledJob) error {
	home, err := homeDir()
	if err != nil {
		return err
	}
	unitDir := filepath.Join(home, ".config/systemd/user")
	if err := os.MkdirAll(unitDir, 0o755); err != nil {
		return fmt.Errorf("creating systemd unit directory: %w", err)
	}
	unitName := systemdScheduledUnitName(job.Name)
	timerName := systemdTimerName(job.Name)
	unitPath := filepath.Join(unitDir, unitName)
	timerPath := filepath.Join(unitDir, timerName)
	if err := os.WriteFile(unitPath, []byte(SystemdScheduledUnitContent(job.Name, job.Command, job.Environment)), 0o644); err != nil {
		return fmt.Errorf("writing systemd unit file %q: %w", unitPath, err)
	}
	if err := os.WriteFile(timerPath, []byte(SystemdScheduledTimerContent(job.Name, job.Interval)), 0o644); err != nil {
		return fmt.Errorf("writing systemd timer file %q: %w", timerPath, err)
	}
	_ = exec.CommandContext(ctx, "systemctl", "--user", "daemon-reload").Run()
	if err := exec.CommandContext(ctx, "systemctl", "--user", "enable", "--now", timerName).Run(); err != nil {
		return fmt.Errorf("enabling systemd timer %q: %w", timerName, err)
	}
	return nil
}

func stopSystemdScheduledJob(ctx context.Context, name string) error {
	home, err := homeDir()
	if err != nil {
		return err
	}
	unitDir := filepath.Join(home, ".config/systemd/user")
	timerName := systemdTimerName(name)
	unitName := systemdScheduledUnitName(name)
	_ = exec.CommandContext(ctx, "systemctl", "--user", "stop", timerName).Run()
	_ = exec.CommandContext(ctx, "systemctl", "--user", "disable", timerName).Run()
	for _, path := range []string{filepath.Join(unitDir, timerName), filepath.Join(unitDir, unitName)} {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("removing systemd scheduled job file %q: %w", path, err)
		}
	}
	_ = exec.CommandContext(ctx, "systemctl", "--user", "daemon-reload").Run()
	return nil
}

func startLaunchdScheduledJob(ctx context.Context, job ScheduledJob) error {
	home, err := homeDir()
	if err != nil {
		return err
	}
	agentDir := filepath.Join(home, "Library/LaunchAgents")
	if err := os.MkdirAll(agentDir, 0o755); err != nil {
		return fmt.Errorf("creating launchd agent directory: %w", err)
	}
	plistPath := filepath.Join(agentDir, launchdPlistName(job.Name))
	if err := os.WriteFile(plistPath, []byte(LaunchdScheduledPlistContent(job.Name, job.Command, job.Interval, job.Environment)), 0o644); err != nil {
		return fmt.Errorf("writing launchd plist file %q: %w", plistPath, err)
	}
	_ = exec.CommandContext(ctx, "launchctl", "unload", plistPath).Run()
	if err := exec.CommandContext(ctx, "launchctl", "load", plistPath).Run(); err != nil {
		return fmt.Errorf("loading launchd scheduled job %q: %w", job.Name, err)
	}
	return nil
}

func stopLaunchdScheduledJob(ctx context.Context, name string) error {
	home, err := homeDir()
	if err != nil {
		return err
	}
	plistPath := filepath.Join(home, "Library/LaunchAgents", launchdPlistName(name))
	_ = exec.CommandContext(ctx, "launchctl", "unload", plistPath).Run()
	if err := os.Remove(plistPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("removing launchd scheduled job plist %q: %w", plistPath, err)
	}
	return nil
}
