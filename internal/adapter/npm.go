package adapter

import "strings"

// Npm manages globally installed npm packages only.
type Npm struct{}

func (Npm) Name() string { return "npm" }

func (Npm) Available() bool {
	_, err := lookPath("npm")
	return err == nil
}

func (Npm) NormalizeID(id string, managers map[string]string) (string, bool) {
	return normalizeID("npm", id, managers)
}

func (Npm) PlanInstall(pkgName string) []string {
	return []string{"npm", "install", "--global", pkgName}
}

func (Npm) PlanUninstall(pkgName string) []string {
	return []string{"npm", "uninstall", "--global", jsBasePackageName(pkgName)}
}

func (Npm) PlanUpgrade(pkgName string) []string {
	return []string{"npm", "install", "--global", pkgName}
}

func (Npm) PlanClean() [][]string { return nil }

func (Npm) Search(query string) ([]string, error) {
	return searchNpmRegistry(query)
}

func (n Npm) Query(pkgName string) (bool, error) {
	entries, err := n.listEntries()
	if err != nil {
		return false, err
	}
	_, ok := findEntry(entries, pkgName)
	return ok, nil
}

func (n Npm) ListInstalled() ([]string, error) {
	entries, err := n.listEntries()
	if err != nil {
		return nil, err
	}
	return entriesNames(entries), nil
}

func (n Npm) QueryVersion(pkgName string) (string, error) {
	entries, err := n.listEntries()
	if err != nil {
		return "", err
	}
	entry, ok := findEntry(entries, pkgName)
	if !ok {
		return "", nil
	}
	return entry.version, nil
}

func (n Npm) ListInstalledVersions() (map[string]string, error) {
	entries, err := n.listEntries()
	if err != nil {
		return nil, err
	}
	return entriesVersions(entries), nil
}

func (n Npm) ListOutdated(pkgNames []string) (map[string]string, error) {
	entries, err := n.listEntries()
	if err != nil {
		return nil, err
	}
	return listJSOutdated(entriesVersions(entries), pkgNames)
}

func (Npm) listEntries() ([]jsPackageEntry, error) {
	return runJSONPackageList("npm", "list", "--global", "--depth=0", "--json")
}

func searchNpmRegistry(query string) ([]string, error) {
	lines, err := runListOutput("npm", "search", "--parseable", query)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(lines))
	for _, line := range lines {
		name, _, _ := strings.Cut(line, "\t")
		if name != "" {
			names = append(names, name)
		}
	}
	return names, nil
}
