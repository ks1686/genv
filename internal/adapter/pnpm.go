package adapter

// Pnpm manages globally installed pnpm packages only.
type Pnpm struct{}

func (Pnpm) Name() string { return "pnpm" }

func (Pnpm) Available() bool {
	_, err := lookPath("pnpm")
	return err == nil
}

func (Pnpm) NormalizeID(id string, managers map[string]string) (string, bool) {
	return normalizeID("pnpm", id, managers)
}

func (Pnpm) PlanInstall(pkgName string) []string {
	return []string{"pnpm", "add", "--global", pkgName}
}

func (Pnpm) PlanUninstall(pkgName string) []string {
	return []string{"pnpm", "remove", "--global", jsBasePackageName(pkgName)}
}

func (Pnpm) PlanUpgrade(pkgName string) []string {
	return []string{"pnpm", "add", "--global", pkgName}
}

func (Pnpm) PlanClean() [][]string { return nil }

func (p Pnpm) Query(pkgName string) (bool, error) {
	entries, err := p.listEntries()
	if err != nil {
		return false, err
	}
	_, ok := findEntry(entries, pkgName)
	return ok, nil
}

func (p Pnpm) ListInstalled() ([]string, error) {
	entries, err := p.listEntries()
	if err != nil {
		return nil, err
	}
	return entriesNames(entries), nil
}

func (p Pnpm) QueryVersion(pkgName string) (string, error) {
	entries, err := p.listEntries()
	if err != nil {
		return "", err
	}
	entry, ok := findEntry(entries, pkgName)
	if !ok {
		return "", nil
	}
	return entry.version, nil
}

func (p Pnpm) ListInstalledVersions() (map[string]string, error) {
	entries, err := p.listEntries()
	if err != nil {
		return nil, err
	}
	return entriesVersions(entries), nil
}

func (p Pnpm) ListOutdated(pkgNames []string) (map[string]string, error) {
	entries, err := p.listEntries()
	if err != nil {
		return nil, err
	}
	return listJSOutdated(entriesVersions(entries), pkgNames)
}

func (Pnpm) listEntries() ([]jsPackageEntry, error) {
	return runJSONPackageList("pnpm", "list", "--global", "--depth=0", "--json")
}
