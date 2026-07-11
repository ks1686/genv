package adapter

import "strings"

// Cargo manages global Rust crates installed with `cargo install`.
type Cargo struct{}

func (Cargo) Name() string { return "cargo" }

func (Cargo) Available() bool {
	_, err := lookPath("cargo")
	return err == nil
}

func (Cargo) NormalizeID(id string, managers map[string]string) (string, bool) {
	return normalizeID("cargo", id, managers)
}

func (Cargo) PlanInstall(pkgName string) []string {
	return []string{"cargo", "install", pkgName}
}

func (Cargo) PlanUninstall(pkgName string) []string {
	return []string{"cargo", "uninstall", cargoBaseName(pkgName)}
}

func (Cargo) PlanUpgrade(pkgName string) []string {
	return []string{"cargo", "install", pkgName}
}

func (Cargo) PlanClean() [][]string { return nil }

func (c Cargo) Query(pkgName string) (bool, error) {
	entries, err := c.listEntries()
	if err != nil {
		return false, err
	}
	base := cargoBaseName(pkgName)
	for _, entry := range entries {
		if entry.name == base {
			return true, nil
		}
	}
	return false, nil
}

func (c Cargo) ListInstalled() ([]string, error) {
	entries, err := c.listEntries()
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.name)
	}
	return names, nil
}

func (c Cargo) QueryVersion(pkgName string) (string, error) {
	entries, err := c.listEntries()
	if err != nil {
		return "", err
	}
	base := cargoBaseName(pkgName)
	for _, entry := range entries {
		if entry.name == base {
			return entry.version, nil
		}
	}
	return "", nil
}

func (c Cargo) ListInstalledVersions() (map[string]string, error) {
	entries, err := c.listEntries()
	if err != nil {
		return nil, err
	}
	versions := make(map[string]string, len(entries))
	for _, entry := range entries {
		versions[entry.name] = entry.version
	}
	return versions, nil
}

type cargoEntry struct {
	name    string
	version string
}

func (Cargo) listEntries() ([]cargoEntry, error) {
	lines, err := runListOutput("cargo", "install", "--list")
	if err != nil {
		return nil, err
	}
	return parseCargoInstallListEntries(lines), nil
}

func cargoBaseName(pkgName string) string {
	return atVersionBaseName(pkgName)
}

func parseCargoInstallListEntries(lines []string) []cargoEntry {
	entries := make([]cargoEntry, 0, len(lines))
	for _, line := range lines {
		if entry, ok := parseCargoInstallListLine(line); ok {
			entries = append(entries, entry)
		}
	}
	return entries
}

func parseCargoInstallListLine(line string) (cargoEntry, bool) {
	if line == "" || isIndented(line) || !strings.HasSuffix(line, ":") {
		return cargoEntry{}, false
	}
	fields := strings.Fields(strings.TrimSuffix(line, ":"))
	if len(fields) < 2 || !strings.HasPrefix(fields[1], "v") {
		return cargoEntry{}, false
	}
	return cargoEntry{name: fields[0], version: strings.TrimPrefix(fields[1], "v")}, true
}
