package service

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/ks1686/genv/internal/genvfile"
	"github.com/ks1686/genv/internal/schema"
)

// systemdUnitName returns the systemd unit name for a genv-managed service.
func systemdUnitName(name string) string {
	name = strings.ReplaceAll(name, "/", "-")
	name = strings.ReplaceAll(name, "\\", "-")
	name = strings.ReplaceAll(name, "..", "")
	return "genv-" + name + ".service"
}

// launchdPlistName returns the launchd plist filename for a genv-managed service.
func launchdPlistName(name string) string {
	name = strings.ReplaceAll(name, "/", "-")
	name = strings.ReplaceAll(name, "\\", "-")
	name = strings.ReplaceAll(name, "..", "")
	return "genv." + name + ".plist"
}

// SystemdLogsHint returns the journalctl command users should run to view logs for name.
func SystemdLogsHint(name string) string {
	return "journalctl --user -u " + systemdUnitName(name)
}

// ServiceStatusKind classifies a service's state vs the lock.
type ServiceStatusKind string

const (
	// ServiceStatusOK means the service is in both spec and lock with the same config.
	ServiceStatusOK ServiceStatusKind = "ok"

	// ServiceStatusModified means the service is in both spec and lock but the config differs.
	ServiceStatusModified ServiceStatusKind = "modified"

	// ServiceStatusMissing means the service is in the spec but has no lock entry.
	ServiceStatusMissing ServiceStatusKind = "missing"

	// ServiceStatusExtra means the service is in the lock but not in the spec.
	ServiceStatusExtra ServiceStatusKind = "extra"
)

// ServiceStatusEntry is one row in the service status report.
type ServiceStatusEntry struct {
	Name    string
	Kind    ServiceStatusKind
	Running bool // true if the service's status command exits 0
}

// ServiceStatus computes the two-way diff between the spec services block and
// the lock services entries.
func ServiceStatus(specServices map[string]schema.Service, lockServices []genvfile.LockedService) []ServiceStatusEntry {
	lockByName := make(map[string]genvfile.LockedService, len(lockServices))
	for _, ls := range lockServices {
		lockByName[ls.Name] = ls
	}

	// Collect all names and sort for deterministic output.
	nameSet := make(map[string]bool)
	for name := range specServices {
		nameSet[name] = true
	}
	for name := range lockByName {
		nameSet[name] = true
	}
	var names []string
	for name := range nameSet {
		names = append(names, name)
	}
	sort.Strings(names)

	var entries []ServiceStatusEntry
	for _, name := range names {
		svc, inSpec := specServices[name]
		ls, inLock := lockByName[name]

		var kind ServiceStatusKind
		switch {
		case inSpec && !inLock:
			kind = ServiceStatusMissing
		case !inSpec && inLock:
			kind = ServiceStatusExtra
		case inSpec && inLock && !compareServices(svc, ls):
			kind = ServiceStatusModified
		default:
			kind = ServiceStatusOK
		}

		running := false
		if inSpec && len(svc.Status) > 0 {
			cmd := exec.Command(svc.Status[0], svc.Status[1:]...)
			if err := cmd.Run(); err == nil {
				running = true
			}
		} else if inSpec && IsSystemdAvailable() {
			cmd := exec.Command("systemctl", "--user", "is-active", "--quiet", systemdUnitName(name))
			if err := cmd.Run(); err == nil {
				running = true
			}
		}

		entries = append(entries, ServiceStatusEntry{
			Name:    name,
			Kind:    kind,
			Running: running,
		})
	}

	return entries
}

func compareServices(s schema.Service, l genvfile.LockedService) bool {
	return strings.Join(s.Start, " ") == strings.Join(l.Start, " ") &&
		strings.Join(s.Stop, " ") == strings.Join(l.Stop, " ") &&
		strings.Join(s.Restart, " ") == strings.Join(l.Restart, " ") &&
		strings.Join(s.Status, " ") == strings.Join(l.Status, " ")
}

// ApplyServices reconciles the system state with the desired services.
// It starts missing/modified services and stops extra services.
func ApplyServices(ctx context.Context, specServices map[string]schema.Service, lockServices []genvfile.LockedService, verbose bool) (applied, removed []string, errs []error) {
	statusEntries := ServiceStatus(specServices, lockServices)

	for _, e := range statusEntries {
		switch e.Kind {
		case ServiceStatusMissing, ServiceStatusModified:
			svc := specServices[e.Name]
			if IsSystemdAvailable() {
				if err := applySystemd(ctx, e.Name, svc, verbose); err != nil {
					errs = append(errs, err)
				} else {
					applied = append(applied, e.Name)
				}
			} else if IsLaunchdAvailable() {
				if err := applyLaunchd(ctx, e.Name, svc, verbose); err != nil {
					errs = append(errs, err)
				} else {
					applied = append(applied, e.Name)
				}
			} else {
				if verbose {
					fmt.Fprintf(os.Stdout, "  service: starting %s\n", e.Name)
				}
				cmd := exec.CommandContext(ctx, svc.Start[0], svc.Start[1:]...)
				if err := cmd.Run(); err != nil {
					errs = append(errs, fmt.Errorf("starting service %q: %w", e.Name, err))
				} else {
					applied = append(applied, e.Name)
				}
			}
		case ServiceStatusExtra:
			// Find the locked service to get its stop command if possible.
			var ls genvfile.LockedService
			for _, l := range lockServices {
				if l.Name == e.Name {
					ls = l
					break
				}
			}

			if IsSystemdAvailable() {
				if err := removeSystemd(ctx, e.Name, verbose); err != nil {
					errs = append(errs, err)
				} else {
					removed = append(removed, e.Name)
				}
			} else if IsLaunchdAvailable() {
				if err := removeLaunchd(ctx, e.Name, verbose); err != nil {
					errs = append(errs, err)
				} else {
					removed = append(removed, e.Name)
				}
			} else if len(ls.Stop) > 0 {
				if verbose {
					fmt.Fprintf(os.Stdout, "  service: stopping %s\n", e.Name)
				}
				cmd := exec.CommandContext(ctx, ls.Stop[0], ls.Stop[1:]...)
				if err := cmd.Run(); err != nil {
					errs = append(errs, fmt.Errorf("stopping service %q: %w", e.Name, err))
				} else {
					removed = append(removed, e.Name)
				}
			} else {
				if verbose {
					fmt.Fprintf(os.Stdout, "  service: removed %s from spec (no stop command defined)\n", e.Name)
				}
				removed = append(removed, e.Name)
			}
		}
	}
	return applied, removed, errs
}

// SpecToLock converts spec services map to a slice of locked services.
func SpecToLock(spec map[string]schema.Service) []genvfile.LockedService {
	if len(spec) == 0 {
		return nil
	}
	var lock []genvfile.LockedService
	for name, svc := range spec {
		lock = append(lock, genvfile.LockedService{
			Name:    name,
			Start:   svc.Start,
			Stop:    svc.Stop,
			Restart: svc.Restart,
			Status:  svc.Status,
		})
	}
	sort.Slice(lock, func(i, j int) bool {
		return lock[i].Name < lock[j].Name
	})
	return lock
}

// IsSystemdAvailable reports whether systemctl is available on the path.
func IsSystemdAvailable() bool {
	if runtime.GOOS != "linux" {
		return false
	}
	if _, err := exec.LookPath("systemctl"); err != nil {
		return false
	}
	// Ensure systemd is actually running for the user
	return exec.Command("systemctl", "--user", "show-environment").Run() == nil
}

// IsLaunchdAvailable reports whether launchctl is available on the path.
func IsLaunchdAvailable() bool {
	if runtime.GOOS != "darwin" {
		return false
	}
	_, err := exec.LookPath("launchctl")
	return err == nil
}

// SystemdUnitContent returns the systemd unit file content for the given service.
func SystemdUnitContent(name string, svc schema.Service) string {
	content := fmt.Sprintf(`[Unit]
Description=genv managed service: %s

[Service]
ExecStart=%s
`, name, strings.Join(svc.Start, " "))

	if len(svc.Stop) > 0 {
		content += fmt.Sprintf("ExecStop=%s\n", strings.Join(svc.Stop, " "))
	}
	if len(svc.Restart) > 0 {
		content += fmt.Sprintf("ExecReload=%s\n", strings.Join(svc.Restart, " "))
	}

	content += `
[Install]
WantedBy=default.target
`
	return content
}

// LaunchdPlistContent returns the launchd plist file content for the given service.
func LaunchdPlistContent(name string, svc schema.Service) string {
	var b strings.Builder
	for _, arg := range svc.Start {
		fmt.Fprintf(&b, "        <string>%s</string>\n", arg)
	}
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>genv.%s</string>
    <key>ProgramArguments</key>
    <array>
%s    </array>
    <key>RunAtLoad</key>
    <true/>
</dict>
</plist>
`, name, b.String())
}

func applySystemd(ctx context.Context, name string, svc schema.Service, verbose bool) error {
	unitDir := filepath.Join(os.Getenv("HOME"), ".config/systemd/user")
	if err := os.MkdirAll(unitDir, 0o755); err != nil {
		return fmt.Errorf("creating systemd unit directory: %w", err)
	}

	unitName := systemdUnitName(name)
	unitPath := filepath.Join(unitDir, unitName)

	if err := os.WriteFile(unitPath, []byte(SystemdUnitContent(name, svc)), 0o644); err != nil {
		return fmt.Errorf("writing systemd unit file %q: %w", unitPath, err)
	}

	if verbose {
		fmt.Fprintf(os.Stdout, "  service: starting %s via systemd\n", name)
	}

	// reload daemon, enable and start service
	exec.CommandContext(ctx, "systemctl", "--user", "daemon-reload").Run()
	if err := exec.CommandContext(ctx, "systemctl", "--user", "enable", "--now", unitName).Run(); err != nil {
		return fmt.Errorf("enabling systemd service %q: %w\nTip: to view logs run: %s", unitName, err, SystemdLogsHint(name))
	}

	return nil
}

func removeSystemd(ctx context.Context, name string, verbose bool) error {
	unitName := systemdUnitName(name)
	unitPath := filepath.Join(os.Getenv("HOME"), ".config/systemd/user", unitName)

	if verbose {
		fmt.Fprintf(os.Stdout, "  service: stopping and removing %s via systemd\n", name)
	}

	exec.CommandContext(ctx, "systemctl", "--user", "stop", unitName).Run()
	exec.CommandContext(ctx, "systemctl", "--user", "disable", unitName).Run()

	if err := os.Remove(unitPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("removing systemd unit file %q: %w", unitPath, err)
	}

	exec.CommandContext(ctx, "systemctl", "--user", "daemon-reload").Run()
	return nil
}

func applyLaunchd(ctx context.Context, name string, svc schema.Service, verbose bool) error {
	agentDir := filepath.Join(os.Getenv("HOME"), "Library/LaunchAgents")
	if err := os.MkdirAll(agentDir, 0o755); err != nil {
		return fmt.Errorf("creating launchd agent directory: %w", err)
	}

	plistName := launchdPlistName(name)
	plistPath := filepath.Join(agentDir, plistName)

	if err := os.WriteFile(plistPath, []byte(LaunchdPlistContent(name, svc)), 0o644); err != nil {
		return fmt.Errorf("writing launchd plist file %q: %w", plistPath, err)
	}

	if verbose {
		fmt.Fprintf(os.Stdout, "  service: starting %s via launchd\n", name)
	}

	// unload if already loaded, then load
	exec.CommandContext(ctx, "launchctl", "unload", plistPath).Run()
	if err := exec.CommandContext(ctx, "launchctl", "load", plistPath).Run(); err != nil {
		return fmt.Errorf("loading launchd service %q: %w\nTip: to view logs run: log show --predicate 'subsystem == \"genv\"' --last 1h", name, err)
	}

	return nil
}

func removeLaunchd(ctx context.Context, name string, verbose bool) error {
	plistName := launchdPlistName(name)
	plistPath := filepath.Join(os.Getenv("HOME"), "Library/LaunchAgents", plistName)

	if verbose {
		fmt.Fprintf(os.Stdout, "  service: stopping and removing %s via launchd\n", name)
	}

	exec.CommandContext(ctx, "launchctl", "unload", plistPath).Run()

	if err := os.Remove(plistPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("removing launchd plist file %q: %w", plistPath, err)
	}

	return nil
}
