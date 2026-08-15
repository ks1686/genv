package adapter

import (
	"os/exec"
	"slices"
	"strings"
)

// Krew manages kubectl plugins via krew. Every operation targets a single
// tracked plugin; genv never runs the broad `kubectl krew upgrade` (all
// plugins) form.
type Krew struct{}

func (Krew) Name() string { return "krew" }

var krewProbe = func() error {
	return exec.Command("kubectl", "krew", "version").Run()
}

func (Krew) Available() bool {
	if _, err := lookPath("kubectl"); err != nil {
		return false
	}
	return krewProbe() == nil
}

func (Krew) NormalizeID(id string, managers map[string]string) (string, bool) {
	return normalizeID("krew", id, managers)
}

func (Krew) PlanInstall(pkgName string) []string {
	return []string{"kubectl", "krew", "install", krewBaseName(pkgName)}
}

func (Krew) PlanUninstall(pkgName string) []string {
	return []string{"kubectl", "krew", "uninstall", krewBaseName(pkgName)}
}

// PlanUpgrade upgrades a single tracked plugin by name; it never issues the
// broad `kubectl krew upgrade` form that updates every installed plugin.
func (Krew) PlanUpgrade(pkgName string) []string {
	return []string{"kubectl", "krew", "upgrade", krewBaseName(pkgName)}
}

func (Krew) PlanClean() [][]string { return nil }

func (k Krew) Query(pkgName string) (bool, error) {
	entries, err := k.listEntries()
	if err != nil {
		return false, err
	}
	return slices.Contains(entries, krewBaseName(pkgName)), nil
}

func (k Krew) ListInstalled() ([]string, error) {
	return k.listEntries()
}

func (Krew) QueryVersion(string) (string, error) { return "", nil }

func (Krew) listEntries() ([]string, error) {
	lines, err := runListOutput("kubectl", "krew", "list")
	if err != nil {
		return nil, err
	}
	plugins := make([]string, 0, len(lines))
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		if strings.EqualFold(fields[0], "PLUGIN") {
			continue
		}
		plugins = append(plugins, fields[0])
	}
	return plugins, nil
}

// krewBaseName strips a "krew/" prefix if the plugin was tracked with the
// index-qualified form and any @version suffix, leaving the bare plugin name
// krew expects for install/uninstall/upgrade.
func krewBaseName(pkgName string) string {
	name := atVersionBaseName(pkgName)
	if _, after, ok := strings.Cut(name, "/"); ok {
		return after
	}
	return name
}
