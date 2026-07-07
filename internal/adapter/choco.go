package adapter

import "strings"

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

// PlanClean clears Chocolatey's HTTP/package download cache.
func (Choco) PlanClean() [][]string {
	return [][]string{{"choco", "cache", "remove", "--all"}}
}

// Query reports whether pkgName is installed, checked against choco's own
// installed-packages list rather than a per-package exit code.
func (Choco) Query(pkgName string) (bool, error) {
	installed, err := Choco{}.ListInstalled()
	if err != nil {
		return false, err
	}
	for _, name := range installed {
		if name == pkgName {
			return true, nil
		}
	}
	return false, nil
}

// Search returns Chocolatey package names containing query.
func (Choco) Search(query string) ([]string, error) {
	lines, err := runListOutput("choco", "search", query)
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
	lines, err := runListOutput("choco", "list")
	if err != nil {
		return nil, err
	}
	rows := parseChocoList(lines)
	names := make([]string, 0, len(rows))
	for _, r := range rows {
		names = append(names, r.name)
	}
	return names, nil
}

func (Choco) QueryVersion(pkgName string) (string, error) {
	lines, err := runListOutput("choco", "list")
	if err != nil {
		return "", err
	}
	for _, r := range parseChocoList(lines) {
		if r.name == pkgName {
			return r.version, nil
		}
	}
	return "", nil
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
