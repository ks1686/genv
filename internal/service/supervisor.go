package service

import (
	"context"
	"fmt"
	"html"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/ks1686/genv/internal/files"
	"github.com/ks1686/genv/internal/genvfile"
	"github.com/ks1686/genv/internal/schema"
)

var (
	probeSystemd = IsSystemdAvailable
	probeLaunchd = IsLaunchdAvailable
	launchctlRun = func(ctx context.Context, args ...string) error {
		return exec.CommandContext(ctx, "launchctl", args...).Run()
	}
	systemctlRun = func(ctx context.Context, args ...string) error {
		return exec.CommandContext(ctx, "systemctl", args...).Run()
	}
)

var launchdLabelRE = regexp.MustCompile(`(?is)<key>\s*Label\s*</key>\s*<string>([^<]*)</string>`)

func launchdGUIDomain() string {
	return "gui/" + strconv.Itoa(os.Getuid())
}

func launchdPrintTarget(label string) string {
	return launchdGUIDomain() + "/" + label
}

func bootstrapLaunchd(ctx context.Context, plistPath string) error {
	return launchctlRun(ctx, "bootstrap", launchdGUIDomain(), plistPath)
}

func bootoutLaunchd(ctx context.Context, label string) error {
	return launchctlRun(ctx, "bootout", launchdPrintTarget(label))
}

func printLaunchd(ctx context.Context, label string) ([]byte, error) {
	return scheduledCommandOutput(ctx, "launchctl", "print", launchdPrintTarget(label))
}

func launchdJobLoaded(ctx context.Context, label string) bool {
	_, err := printLaunchd(ctx, label)
	return err == nil
}

func launchdJobRunning(ctx context.Context, label string) bool {
	output, err := printLaunchd(ctx, label)
	if err != nil {
		return false
	}
	fields := parseLaunchdFields(string(output))
	return strings.EqualFold(fields["state"], "running")
}

func systemdUnitActive(ctx context.Context, unitName string) bool {
	return systemctlRun(ctx, "--user", "is-active", "--quiet", unitName) == nil
}

// ParseLaunchdLabel returns the Label string from a rendered plist.
func ParseLaunchdLabel(plist string) (string, error) {
	m := launchdLabelRE.FindStringSubmatch(plist)
	if len(m) < 2 {
		return "", fmt.Errorf("plist is missing a Label string")
	}
	label, err := sanitizeSupervisorName(html.UnescapeString(strings.TrimSpace(m[1])))
	if err != nil {
		return "", fmt.Errorf("plist Label: %w", err)
	}
	return label, nil
}

func sanitizeSupervisorName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", fmt.Errorf("must not be empty")
	}
	if strings.ContainsAny(name, "/\\\x00\r\n") {
		return "", fmt.Errorf("%q contains an illegal character", name)
	}
	if name == "." || name == ".." {
		return "", fmt.Errorf("%q is not a valid name", name)
	}
	return name, nil
}

func systemdTemplateUnitName(serviceName, unitPath string) (string, error) {
	base := filepath.Base(unitPath)
	if strings.HasSuffix(base, ".service") && base != ".service" {
		return sanitizeSupervisorName(strings.TrimSuffix(base, ".service"))
	}
	return strings.TrimSuffix(systemdUnitName(serviceName), ".service"), nil
}

func launchdAgentPath(label string) (string, error) {
	home, err := homeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Library", "LaunchAgents", label+".plist"), nil
}

func systemdUserUnitPath(unitName string) (string, error) {
	home, err := homeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "systemd", "user", unitName+".service"), nil
}

func resolveServiceSource(sourceRoot, source string) (string, error) {
	return files.ResolveSource(sourceRoot, source)
}

func renderServiceTemplate(sourceRoot, source string) (string, error) {
	path, err := resolveServiceSource(sourceRoot, source)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read template %s: %w", path, err)
	}
	rendered, err := files.RenderString(string(data))
	if err != nil {
		return "", fmt.Errorf("render template %s: %w", path, err)
	}
	return rendered, nil
}

func writeIfChanged(path, content string) (changed bool, err error) {
	existing, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return false, err
	}
	if err == nil && string(existing) == content {
		return false, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, err
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return false, err
	}
	return true, nil
}

func applyLaunchdTemplate(ctx context.Context, name string, svc schema.Service, sourceRoot string, verbose bool) (bool, error) {
	rendered, err := renderServiceTemplate(sourceRoot, svc.Launchd.Plist)
	if err != nil {
		return false, fmt.Errorf("service %q launchd plist: %w", name, err)
	}
	label, err := ParseLaunchdLabel(rendered)
	if err != nil {
		return false, fmt.Errorf("service %q launchd plist: %w", name, err)
	}
	dest, err := launchdAgentPath(label)
	if err != nil {
		return false, err
	}
	changed, err := writeIfChanged(dest, rendered)
	if err != nil {
		return false, fmt.Errorf("writing launchd plist %q: %w", dest, err)
	}
	loaded := launchdJobLoaded(ctx, label)
	if verbose {
		_, _ = fmt.Fprintf(os.Stdout, "  service: reconciling %s via launchd (%s)\n", name, label)
	}
	if changed && loaded {
		_ = bootoutLaunchd(ctx, label)
		if err := bootstrapLaunchd(ctx, dest); err != nil {
			return false, fmt.Errorf("re-bootstrapping launchd service %q: %w", name, err)
		}
		return true, nil
	}
	if !loaded {
		if err := bootstrapLaunchd(ctx, dest); err != nil {
			return false, fmt.Errorf("bootstrapping launchd service %q: %w", name, err)
		}
		return true, nil
	}
	return changed, nil
}

func applySystemdTemplate(ctx context.Context, name string, svc schema.Service, sourceRoot string, verbose bool) (bool, error) {
	rendered, err := renderServiceTemplate(sourceRoot, svc.Systemd.Unit)
	if err != nil {
		return false, fmt.Errorf("service %q systemd unit: %w", name, err)
	}
	unitBase, err := systemdTemplateUnitName(name, svc.Systemd.Unit)
	if err != nil {
		return false, fmt.Errorf("service %q systemd unit: %w", name, err)
	}
	dest, err := systemdUserUnitPath(unitBase)
	if err != nil {
		return false, err
	}
	changed, err := writeIfChanged(dest, rendered)
	if err != nil {
		return false, fmt.Errorf("writing systemd unit %q: %w", dest, err)
	}
	unitName := unitBase + ".service"
	wasActive := systemdUnitActive(ctx, unitName)
	if verbose {
		_, _ = fmt.Fprintf(os.Stdout, "  service: reconciling %s via systemd (%s)\n", name, unitName)
	}
	_ = systemctlRun(ctx, "--user", "daemon-reload")
	if err := systemctlRun(ctx, "--user", "enable", "--now", unitName); err != nil {
		return false, fmt.Errorf("enabling systemd service %q: %w\nTip: to view logs run: %s", unitName, err, SystemdLogsHint(name))
	}
	if changed && wasActive {
		_ = systemctlRun(ctx, "--user", "restart", unitName)
	}
	return changed || !wasActive, nil
}

func removeLaunchdTemplate(ctx context.Context, name, label string, verbose bool) error {
	if label == "" {
		label = strings.TrimSuffix(launchdPlistName(name), ".plist")
	}
	dest, err := launchdAgentPath(label)
	if err != nil {
		return err
	}
	if verbose {
		_, _ = fmt.Fprintf(os.Stdout, "  service: stopping and removing %s via launchd (%s)\n", name, label)
	}
	_ = bootoutLaunchd(ctx, label)
	if err := os.Remove(dest); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("removing launchd plist %q: %w", dest, err)
	}
	return nil
}

func removeSystemdTemplate(ctx context.Context, name, unitBase string, verbose bool) error {
	if unitBase == "" {
		unitBase = strings.TrimSuffix(systemdUnitName(name), ".service")
	}
	unitName := unitBase + ".service"
	dest, err := systemdUserUnitPath(unitBase)
	if err != nil {
		return err
	}
	if verbose {
		_, _ = fmt.Fprintf(os.Stdout, "  service: stopping and removing %s via systemd (%s)\n", name, unitName)
	}
	_ = systemctlRun(ctx, "--user", "stop", unitName)
	_ = systemctlRun(ctx, "--user", "disable", unitName)
	if err := os.Remove(dest); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("removing systemd unit %q: %w", dest, err)
	}
	_ = systemctlRun(ctx, "--user", "daemon-reload")
	return nil
}

func launchdLabelFor(svc schema.Service, sourceRoot string) (string, error) {
	if !svc.DeclaresLaunchd() {
		return "", fmt.Errorf("service does not declare launchd")
	}
	rendered, err := renderServiceTemplate(sourceRoot, svc.Launchd.Plist)
	if err != nil {
		return "", err
	}
	return ParseLaunchdLabel(rendered)
}

func systemdNameFor(name string, svc schema.Service) (string, error) {
	if !svc.DeclaresSystemd() {
		return "", fmt.Errorf("service does not declare systemd")
	}
	return systemdTemplateUnitName(name, svc.Systemd.Unit)
}

// StartDeclared starts a spec-declared service using brew, a supervisor template,
// a generated unit, or the raw start command.
func StartDeclared(ctx context.Context, name string, svc schema.Service, sourceRoot string) error {
	if svc.BrewFormula != "" {
		return BrewServicesStart(ctx, svc.BrewFormula)
	}
	if svc.DeclaresLaunchd() {
		if !probeLaunchd() {
			return fmt.Errorf("service %q requires launchd, which is not available on this host", name)
		}
		_, err := applyLaunchdTemplate(ctx, name, svc, sourceRoot, false)
		return err
	}
	if svc.DeclaresSystemd() {
		if !probeSystemd() {
			return fmt.Errorf("service %q requires systemd --user, which is not available on this host", name)
		}
		_, err := applySystemdTemplate(ctx, name, svc, sourceRoot, false)
		return err
	}
	if probeSystemd() {
		return applySystemd(ctx, name, svc, false)
	}
	if probeLaunchd() {
		return applyLaunchd(ctx, name, svc, false)
	}
	if len(svc.Start) == 0 {
		return fmt.Errorf("service %q has no start command", name)
	}
	return exec.CommandContext(ctx, svc.Start[0], svc.Start[1:]...).Run()
}

// StopDeclared stops a spec-declared service.
func StopDeclared(ctx context.Context, name string, svc schema.Service, sourceRoot string) error {
	if svc.BrewFormula != "" {
		return BrewServicesStop(ctx, svc.BrewFormula)
	}
	if svc.DeclaresLaunchd() {
		if !probeLaunchd() {
			return fmt.Errorf("service %q requires launchd, which is not available on this host", name)
		}
		label, err := launchdLabelFor(svc, sourceRoot)
		if err != nil {
			return fmt.Errorf("service %q: %w", name, err)
		}
		if err := bootoutLaunchd(ctx, label); err != nil {
			return fmt.Errorf("stopping launchd service %q: %w", name, err)
		}
		return nil
	}
	if svc.DeclaresSystemd() {
		if !probeSystemd() {
			return fmt.Errorf("service %q requires systemd --user, which is not available on this host", name)
		}
		unitBase, err := systemdNameFor(name, svc)
		if err != nil {
			return err
		}
		if err := systemctlRun(ctx, "--user", "stop", unitBase+".service"); err != nil {
			return fmt.Errorf("stopping systemd service %q: %w", name, err)
		}
		return nil
	}
	if len(svc.Stop) == 0 {
		return fmt.Errorf("no stop command defined for service %q", name)
	}
	return exec.CommandContext(ctx, svc.Stop[0], svc.Stop[1:]...).Run()
}

// ProbeRunning reports whether the declared service looks live.
func ProbeRunning(ctx context.Context, name string, svc schema.Service, sourceRoot string) bool {
	if svc.BrewFormula != "" {
		return BrewServicesRunning(svc.BrewFormula)
	}
	if svc.DeclaresLaunchd() && probeLaunchd() {
		label, err := launchdLabelFor(svc, sourceRoot)
		if err != nil {
			return false
		}
		return launchdJobRunning(ctx, label)
	}
	if svc.DeclaresSystemd() && probeSystemd() {
		unitBase, err := systemdNameFor(name, svc)
		if err != nil {
			return false
		}
		return systemdUnitActive(ctx, unitBase+".service")
	}
	if len(svc.Status) > 0 {
		if err := exec.CommandContext(ctx, svc.Status[0], svc.Status[1:]...).Run(); err == nil {
			return true
		}
		return false
	}
	if probeSystemd() {
		return systemdUnitActive(ctx, systemdUnitName(name))
	}
	if probeLaunchd() {
		return launchdJobRunning(ctx, strings.TrimSuffix(launchdPlistName(name), ".plist"))
	}
	return false
}

func applyDeclared(ctx context.Context, name string, svc schema.Service, sourceRoot string, verbose bool) (bool, error) {
	if svc.BrewFormula != "" {
		if !IsBrewServicesAvailable() {
			return false, fmt.Errorf("service %q requires brew services but brew is not available on this platform", name)
		}
		if verbose {
			_, _ = fmt.Fprintf(os.Stdout, "  service: starting %s via brew services\n", name)
		}
		return true, BrewServicesStart(ctx, svc.BrewFormula)
	}
	if svc.DeclaresLaunchd() && probeLaunchd() {
		return applyLaunchdTemplate(ctx, name, svc, sourceRoot, verbose)
	}
	if svc.DeclaresSystemd() && probeSystemd() {
		return applySystemdTemplate(ctx, name, svc, sourceRoot, verbose)
	}
	if svc.DeclaresLaunchd() || svc.DeclaresSystemd() {
		if verbose {
			_, _ = fmt.Fprintf(os.Stdout, "  service: skipping %s (supervisor not available on this host)\n", name)
		}
		return false, nil
	}
	if probeSystemd() {
		return true, applySystemd(ctx, name, svc, verbose)
	}
	if probeLaunchd() {
		return true, applyLaunchd(ctx, name, svc, verbose)
	}
	if verbose {
		_, _ = fmt.Fprintf(os.Stdout, "  service: starting %s\n", name)
	}
	if len(svc.Start) == 0 {
		return false, fmt.Errorf("starting service %q: no start command", name)
	}
	if err := exec.CommandContext(ctx, svc.Start[0], svc.Start[1:]...).Run(); err != nil {
		return false, fmt.Errorf("starting service %q: %w", name, err)
	}
	return true, nil
}

func removeLocked(ctx context.Context, name string, ls genvfile.LockedService, verbose bool) error {
	if ls.BrewFormula != "" {
		if !IsBrewServicesAvailable() {
			return fmt.Errorf("service %q requires brew services but brew is not available on this platform", name)
		}
		if verbose {
			_, _ = fmt.Fprintf(os.Stdout, "  service: stopping %s via brew services\n", name)
		}
		return BrewServicesStop(ctx, ls.BrewFormula)
	}
	if ls.LaunchdPlist != "" || ls.LaunchdLabel != "" {
		if !probeLaunchd() {
			return nil
		}
		return removeLaunchdTemplate(ctx, name, ls.LaunchdLabel, verbose)
	}
	if ls.SystemdUnit != "" || ls.SystemdName != "" {
		if !probeSystemd() {
			return nil
		}
		return removeSystemdTemplate(ctx, name, ls.SystemdName, verbose)
	}
	if probeSystemd() {
		return removeSystemd(ctx, name, verbose)
	}
	if probeLaunchd() {
		return removeLaunchd(ctx, name, verbose)
	}
	if len(ls.Stop) > 0 {
		if verbose {
			_, _ = fmt.Fprintf(os.Stdout, "  service: stopping %s\n", name)
		}
		return exec.CommandContext(ctx, ls.Stop[0], ls.Stop[1:]...).Run()
	}
	if verbose {
		_, _ = fmt.Fprintf(os.Stdout, "  service: removed %s from spec (no stop command defined)\n", name)
	}
	return nil
}
