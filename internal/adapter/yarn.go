package adapter

import "strings"

// Yarn manages Yarn Classic global packages only. Yarn Berry removed the
// classic global command model, so Berry project-scoped installs are out of scope.
type Yarn struct{}

func (Yarn) Name() string { return "yarn" }

func (Yarn) Available() bool {
	_, err := lookPath("yarn")
	return err == nil
}

func (Yarn) NormalizeID(id string, managers map[string]string) (string, bool) {
	return normalizeID("yarn", id, managers)
}

func (Yarn) PlanInstall(pkgName string) []string {
	return []string{"yarn", "global", "add", pkgName}
}

func (Yarn) PlanUninstall(pkgName string) []string {
	return []string{"yarn", "global", "remove", jsBasePackageName(pkgName)}
}

func (Yarn) PlanUpgrade(pkgName string) []string {
	return []string{"yarn", "global", "add", pkgName}
}

func (Yarn) PlanClean() [][]string { return nil }

func (y Yarn) Query(pkgName string) (bool, error) {
	entries, err := y.listEntries()
	if err != nil {
		return false, err
	}
	_, ok := findEntry(entries, pkgName)
	return ok, nil
}

func (y Yarn) ListInstalled() ([]string, error) {
	entries, err := y.listEntries()
	if err != nil {
		return nil, err
	}
	return entriesNames(entries), nil
}

func (y Yarn) QueryVersion(pkgName string) (string, error) {
	entries, err := y.listEntries()
	if err != nil {
		return "", err
	}
	entry, ok := findEntry(entries, pkgName)
	if !ok {
		return "", nil
	}
	return entry.version, nil
}

func (y Yarn) ListInstalledVersions() (map[string]string, error) {
	entries, err := y.listEntries()
	if err != nil {
		return nil, err
	}
	return entriesVersions(entries), nil
}

func (Yarn) listEntries() ([]jsPackageEntry, error) {
	lines, err := runListOutput("yarn", "global", "list", "--depth=0")
	if err != nil {
		return nil, err
	}
	return parseYarnGlobalListEntries(lines), nil
}

func parseYarnGlobalListEntries(lines []string) []jsPackageEntry {
	entries := make([]jsPackageEntry, 0, len(lines))
	for _, line := range lines {
		if entry, ok := parseYarnGlobalListLine(line); ok {
			entries = append(entries, entry)
		}
	}
	return entries
}

func parseYarnGlobalListLine(line string) (jsPackageEntry, bool) {
	spec, ok := quotedYarnSpec(line)
	if !ok {
		spec = treeYarnSpec(line)
	}
	if spec == "" {
		return jsPackageEntry{}, false
	}
	base := jsBasePackageName(spec)
	version := ""
	if len(spec) > len(base) && strings.HasPrefix(spec[len(base):], "@") {
		version = spec[len(base)+1:]
	}
	return jsPackageEntry{name: base, version: version}, true
}

func quotedYarnSpec(line string) (string, bool) {
	start := strings.Index(line, "\"")
	if start < 0 {
		return "", false
	}
	end := strings.Index(line[start+1:], "\"")
	if end < 0 {
		return "", false
	}
	return line[start+1 : start+1+end], true
}

func treeYarnSpec(line string) string {
	trimmed := strings.TrimSpace(line)
	trimmed = strings.TrimLeft(trimmed, "├└─ ")
	if strings.Contains(trimmed, "@") {
		return strings.Fields(trimmed)[0]
	}
	return ""
}
