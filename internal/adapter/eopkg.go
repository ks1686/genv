package adapter

import "strings"

// Eopkg is the adapter for the eopkg package manager (Solus Linux).
type Eopkg struct{}

func (Eopkg) Name() string { return "eopkg" }

func (Eopkg) Available() bool {
	_, err := lookPath("eopkg")
	return err == nil
}

func (Eopkg) NormalizeID(id string, managers map[string]string) (string, bool) {
	return normalizeID("eopkg", id, managers)
}

func (Eopkg) PlanInstall(pkgName string) []string {
	return []string{"sudo", "eopkg", "install", "-y", pkgName}
}

func (Eopkg) PlanUninstall(pkgName string) []string {
	return []string{"sudo", "eopkg", "remove", "-y", pkgName}
}

func (Eopkg) PlanUpgrade(pkgName string) []string {
	return []string{"sudo", "eopkg", "upgrade", "-y", pkgName}
}

func (Eopkg) PlanClean() [][]string {
	return [][]string{
		{"sudo", "eopkg", "delete-cache"},
	}
}

// Query reports whether pkgName is installed.
// "eopkg info -i <pkg>" exits 0 when the package is installed, non-zero when absent.
func (Eopkg) Query(pkgName string) (bool, error) {
	return runQuery("eopkg", "info", "-i", pkgName)
}

// eopkgListQuery returns the raw "pkgname - version, release R" lines from
// eopkg list-installed. Shared by ListInstalled and QueryVersion to avoid
// forking eopkg twice.
func eopkgListQuery() ([]string, error) {
	return runListOutput("eopkg", "list-installed")
}

// Search returns package names from Solus repos whose name contains query.
// "eopkg search <query>" outputs "pkgname - description" lines; we extract the
// package name and filter to those containing query (case-insensitive).
func (Eopkg) Search(query string) ([]string, error) {
	lines, err := runListOutput("eopkg", "search", query)
	if err != nil || len(lines) == 0 {
		return lines, err
	}
	q := strings.ToLower(query)
	seen := make(map[string]bool)
	var names []string
	for _, line := range lines {
		name, _, _ := strings.Cut(line, " - ")
		name = strings.TrimSpace(name)
		if name != "" && strings.Contains(strings.ToLower(name), q) && !seen[name] {
			seen[name] = true
			names = append(names, name)
		}
	}
	return names, nil
}

// ListInstalled returns the names of all packages installed via eopkg.
// "eopkg list-installed" outputs "pkgname - version, release R" per line;
// we take the package name from the first field.
func (Eopkg) ListInstalled() ([]string, error) {
	lines, err := eopkgListQuery()
	if err != nil || len(lines) == 0 {
		return lines, err
	}
	var names []string
	for _, line := range lines {
		name, _, _ := strings.Cut(line, " - ")
		name = strings.TrimSpace(name)
		if name != "" {
			names = append(names, name)
		}
	}
	return names, nil
}

// QueryVersion returns the installed version of pkgName.
// "eopkg list-installed" outputs "pkgname - version, release R" per line;
// we scan for the line matching pkgName and extract the version before the comma.
func (Eopkg) QueryVersion(pkgName string) (string, error) {
	lines, err := eopkgListQuery()
	if err != nil || len(lines) == 0 {
		return "", err
	}
	for _, line := range lines {
		name, rest, ok := strings.Cut(line, " - ")
		if !ok || strings.TrimSpace(name) != pkgName {
			continue
		}
		ver, _, _ := strings.Cut(rest, ",")
		return strings.TrimSpace(ver), nil
	}
	return "", nil
}
