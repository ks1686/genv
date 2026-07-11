package adapter

import "strings"

// Volta manages globally installed JavaScript tools through volta install.
type Volta struct{}

func (Volta) Name() string { return "volta" }

func (Volta) Available() bool {
	_, err := lookPath("volta")
	return err == nil
}

func (Volta) NormalizeID(id string, managers map[string]string) (string, bool) {
	return normalizeID("volta", id, managers)
}

func (Volta) PlanInstall(pkgName string) []string {
	return []string{"volta", "install", pkgName}
}

func (Volta) PlanUninstall(pkgName string) []string {
	return []string{"volta", "uninstall", jsBasePackageName(pkgName)}
}

func (Volta) PlanUpgrade(pkgName string) []string {
	return []string{"volta", "install", pkgName}
}

func (Volta) PlanClean() [][]string { return nil }

func (v Volta) Query(pkgName string) (bool, error) {
	entries, err := v.listEntries()
	if err != nil {
		return false, err
	}
	_, ok := findEntry(entries, pkgName)
	return ok, nil
}

func (v Volta) ListInstalled() ([]string, error) {
	entries, err := v.listEntries()
	if err != nil {
		return nil, err
	}
	return entriesNames(entries), nil
}

func (v Volta) QueryVersion(pkgName string) (string, error) {
	entries, err := v.listEntries()
	if err != nil {
		return "", err
	}
	entry, ok := findEntry(entries, pkgName)
	if !ok {
		return "", nil
	}
	return entry.version, nil
}

func (v Volta) ListInstalledVersions() (map[string]string, error) {
	entries, err := v.listEntries()
	if err != nil {
		return nil, err
	}
	return entriesVersions(entries), nil
}

func (Volta) listEntries() ([]jsPackageEntry, error) {
	lines, err := runListOutput("volta", "list", "all")
	if err != nil {
		return nil, err
	}
	return parseVoltaListAllEntries(lines), nil
}

func parseVoltaListAllEntries(lines []string) []jsPackageEntry {
	entries := make([]jsPackageEntry, 0, len(lines))
	for _, line := range lines {
		if entry, ok := parseVoltaListAllLine(line); ok {
			entries = append(entries, entry)
		}
	}
	return entries
}

func parseVoltaListAllLine(line string) (jsPackageEntry, bool) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || strings.HasPrefix(trimmed, "⚡") || strings.HasPrefix(trimmed, "Node") || strings.HasPrefix(trimmed, "Package") {
		return jsPackageEntry{}, false
	}
	fields := strings.Fields(trimmed)
	if len(fields) == 0 || !strings.Contains(fields[0], "@") {
		return jsPackageEntry{}, false
	}
	base := jsBasePackageName(fields[0])
	if base == "node" || base == "npm" || base == "yarn" || base == fields[0] {
		return jsPackageEntry{}, false
	}
	return jsPackageEntry{name: base, version: fields[0][len(base)+1:]}, true
}
