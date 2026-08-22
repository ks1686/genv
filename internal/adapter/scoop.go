package adapter

import (
	"context"
	"os"
	"path/filepath"
	"strings"
)

// Scoop is the adapter for scoop, a CLI-focused Windows package manager that
// installs into the user profile (no admin/UAC required for most packages).
type Scoop struct{}

func (Scoop) Name() string { return "scoop" }

func (Scoop) Available() bool {
	_, err := lookPath("scoop")
	return err == nil
}

func (Scoop) NormalizeID(id string, managers map[string]string) (string, bool) {
	return normalizeID("scoop", id, managers)
}

func (Scoop) PlanInstall(pkgName string) []string {
	return []string{"scoop", "install", pkgName}
}

func (Scoop) PlanUninstall(pkgName string) []string {
	return []string{"scoop", "uninstall", pkgName}
}

func (Scoop) PlanUpgrade(pkgName string) []string {
	return []string{"scoop", "update", pkgName}
}

// PlanUpgradeBatch updates multiple apps in one scoop invocation.
func (Scoop) PlanUpgradeBatch(pkgNames []string) []string {
	args := []string{"scoop", "update"}
	return append(args, pkgNames...)
}

// PlanClean clears scoop's downloaded-installer cache for every app.
func (Scoop) PlanClean() [][]string {
	return [][]string{{"scoop", "cache", "rm", "*"}}
}

// Query reports whether pkgName is installed, by checking scoop's own
// installed-apps list rather than relying on a per-package exit code (scoop
// list <app> does not reliably signal absence via exit status).
func (Scoop) Query(pkgName string) (bool, error) {
	entries, err := Scoop{}.listEntries()
	if err != nil {
		return false, err
	}
	for _, entry := range entries {
		if entry.name == pkgName {
			return true, nil
		}
	}
	return false, nil
}

// Search returns scoop app names containing query.
func (Scoop) Search(query string) ([]string, error) {
	return Scoop{}.SearchContext(context.Background(), query)
}

func (Scoop) SearchContext(ctx context.Context, query string) ([]string, error) {
	lines, err := runListOutputContext(ctx, "scoop", "search", query)
	if err != nil || len(lines) == 0 {
		return lines, err
	}
	var names []string
	for _, name := range parseScoopList(lines) {
		if containsFold(name.name, query) {
			names = append(names, name.name)
		}
	}
	return names, nil
}

// ListInstalled parses "scoop list" output. The table is colorized (ANSI
// escape codes) and column-aligned; app names never contain spaces, so a
// simple first-field split per data row is sufficient once escape codes and
// header/separator/blank lines are stripped.
func (Scoop) ListInstalled() ([]string, error) {
	entries, err := Scoop{}.listEntries()
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.name)
	}
	return names, nil
}

func (Scoop) QueryVersion(pkgName string) (string, error) {
	entries, err := Scoop{}.listEntries()
	if err != nil {
		return "", err
	}
	for _, entry := range entries {
		if entry.name == pkgName {
			return entry.version, nil
		}
	}
	return "", nil
}

func (Scoop) ListInstalledVersions() (map[string]string, error) {
	entries, err := Scoop{}.listEntries()
	if err != nil {
		return nil, err
	}
	versions := make(map[string]string, len(entries))
	for _, entry := range entries {
		versions[entry.name] = entry.version
	}
	return versions, nil
}

// ListOutdated reports Scoop apps with an available update, keyed by app name
// -> target version, intersected with pkgNames.
func (Scoop) ListOutdated(pkgNames []string) (map[string]string, error) {
	out, err := runProbe("scoop", "status")
	if err != nil {
		return nil, err
	}
	outdated := make(map[string]string)
	for _, entry := range parseScoopStatus(trimmedNonEmptyLines(string(out))) {
		outdated[entry.name] = entry.version
	}
	return intersectNameMap(outdated, pkgNames), nil
}

func (Scoop) listEntries() ([]scoopEntry, error) {
	lines, err := runListOutput("scoop", "list")
	if err != nil {
		return nil, err
	}
	return parseScoopList(lines), nil
}

type scoopEntry struct {
	name    string
	version string
}

// parseScoopList strips ANSI color escapes and parses "scoop list" /
// "scoop search" output, skipping the "Installed apps:" banner, the header
// row, the "----" separator row, and blank lines.
func parseScoopList(lines []string) []scoopEntry {
	var entries []scoopEntry
	for _, line := range lines {
		clean := stripANSI(line)
		trimmed := strings.TrimSpace(clean)
		if trimmed == "" || strings.HasPrefix(trimmed, "Installed apps") || strings.HasPrefix(trimmed, "----") || strings.HasPrefix(trimmed, "Name") {
			continue
		}
		fields := strings.Fields(trimmed)
		if len(fields) < 2 {
			continue
		}
		entries = append(entries, scoopEntry{name: fields[0], version: fields[1]})
	}
	return entries
}

// parseScoopStatus returns the package name and available version from Scoop's
// "scoop status" table, skipping its header, separator, and no-update banner.
func parseScoopStatus(lines []string) []scoopEntry {
	var entries []scoopEntry
	for _, line := range lines {
		trimmed := strings.TrimSpace(stripANSI(line))
		if trimmed == "" || strings.HasPrefix(trimmed, "Name") || strings.HasPrefix(trimmed, "----") || strings.HasPrefix(trimmed, "Scoop is up to date") {
			continue
		}
		fields := strings.Fields(trimmed)
		if len(fields) >= 3 {
			entries = append(entries, scoopEntry{name: fields[0], version: fields[2]})
		}
	}
	return entries
}

// stripANSI removes "ESC[...m"-style SGR color escape sequences.
func stripANSI(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == 0x1b && i+1 < len(s) && s[i+1] == '[' {
			j := i + 2
			for j < len(s) && s[j] != 'm' {
				j++
			}
			i = j
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// ScoopGitCmdDir returns the versioned scoop git cmd directory, skipping the
// "current" junction which OpenSSH sessions often cannot follow.
func ScoopGitCmdDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	if v := os.Getenv("USERPROFILE"); v != "" {
		home = v
	}
	root := filepath.Join(home, "scoop", "apps", "git")
	entries, err := os.ReadDir(root)
	if err != nil {
		return ""
	}
	var best string
	for _, e := range entries {
		name := e.Name()
		if name == "current" || !e.IsDir() {
			continue
		}
		cmd := filepath.Join(root, name, "cmd")
		if _, err := os.Stat(filepath.Join(cmd, "git.exe")); err != nil {
			if _, err2 := os.Stat(filepath.Join(cmd, "git")); err2 != nil {
				continue
			}
		}
		if best == "" || name > best {
			best = name
		}
	}
	if best == "" {
		return ""
	}
	return filepath.Join(root, best, "cmd")
}
