package adapter

import (
	"context"
	"errors"
	"os/exec"
	"strings"
)

// Choco is the adapter for Chocolatey, a Windows package manager that
// typically requires an elevated (admin) shell for install/uninstall/upgrade.
type Choco struct{}

func (Choco) Name() string { return "choco" }

func (Choco) Available() bool {
	_, err := lookPath("choco")
	return err == nil
}

func (Choco) NormalizeID(id string, managers map[string]string) (string, bool) {
	return normalizeID("choco", id, managers)
}

func (Choco) PlanInstall(pkgName string) []string {
	return []string{"choco", "install", "-y", pkgName}
}

func (Choco) PlanUninstall(pkgName string) []string {
	return []string{"choco", "uninstall", "-y", pkgName}
}

func (Choco) PlanUpgrade(pkgName string) []string {
	return []string{"choco", "upgrade", "-y", pkgName}
}

// PlanUpgradeBatch upgrades multiple packages in one choco invocation.
func (Choco) PlanUpgradeBatch(pkgNames []string) []string {
	args := []string{"choco", "upgrade", "-y"}
	return append(args, pkgNames...)
}

// PlanClean clears Chocolatey's HTTP/package download cache.
func (Choco) PlanClean() [][]string {
	return [][]string{{"choco", "cache", "remove", "--all"}}
}

// Query reports whether pkgName is installed, checked against choco's own
// installed-packages list rather than a per-package exit code.
func (Choco) Query(pkgName string) (bool, error) {
	entries, err := Choco{}.listEntries()
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

// Search returns Chocolatey package names containing query.
func (Choco) Search(query string) ([]string, error) {
	return Choco{}.SearchContext(context.Background(), query)
}

func (Choco) SearchContext(ctx context.Context, query string) ([]string, error) {
	lines, err := runListOutputContext(ctx, "choco", "search", query)
	if err != nil || len(lines) == 0 {
		return lines, err
	}
	var names []string
	for _, name := range parseChocoList(lines) {
		if containsFold(name.name, query) {
			names = append(names, name.name)
		}
	}
	return names, nil
}

// ListInstalled parses "choco list" output. Since Chocolatey CLI v2, bare
// "choco list" (no --local-only, which was removed) lists locally installed
// packages by default, one "<name> <version>" pair per line, preceded by a
// "Chocolatey vX.Y.Z" banner line.
func (Choco) ListInstalled() ([]string, error) {
	entries, err := Choco{}.listEntries()
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.name)
	}
	return names, nil
}

func (Choco) QueryVersion(pkgName string) (string, error) {
	entries, err := Choco{}.listEntries()
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

func (Choco) ListInstalledVersions() (map[string]string, error) {
	entries, err := Choco{}.listEntries()
	if err != nil {
		return nil, err
	}
	versions := make(map[string]string, len(entries))
	for _, entry := range entries {
		versions[entry.name] = entry.version
	}
	return versions, nil
}

// ListOutdated reports Chocolatey packages whose available version differs
// from the installed one, keyed by package name -> target version.
func (Choco) ListOutdated(pkgNames []string) (map[string]string, error) {
	out, err := exec.Command("choco", "outdated", "-r").Output()
	if err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) || exitErr.ExitCode() != 2 {
			return nil, err
		}
		// Chocolatey enhanced exit code 2 means outdated packages were found.
	}
	outdated := make(map[string]string)
	for _, line := range trimmedNonEmptyLines(string(out)) {
		fields := strings.Split(line, "|")
		if len(fields) >= 3 && fields[0] != "" && fields[1] != fields[2] {
			outdated[fields[0]] = fields[2]
		}
	}
	return intersectNameMap(outdated, pkgNames), nil
}

func (Choco) listEntries() ([]chocoEntry, error) {
	lines, err := runListOutput("choco", "list")
	if err != nil {
		return nil, err
	}
	return parseChocoList(lines), nil
}

type chocoEntry struct {
	name    string
	version string
}

// parseChocoList skips the "Chocolatey vX.Y.Z" banner and any trailing
// summary line ("N packages installed."), returning "<name> <version>"
// pairs from the remaining lines.
func parseChocoList(lines []string) []chocoEntry {
	var entries []chocoEntry
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "Chocolatey v") || strings.HasSuffix(trimmed, "packages installed.") || strings.HasSuffix(trimmed, "package installed.") {
			continue
		}
		fields := strings.Fields(trimmed)
		if len(fields) < 2 {
			continue
		}
		entries = append(entries, chocoEntry{name: fields[0], version: fields[1]})
	}
	return entries
}
