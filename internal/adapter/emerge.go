package adapter

import "strings"

// Emerge is the adapter for the Portage package manager (Gentoo Linux).
type Emerge struct{}

func (Emerge) Name() string { return "emerge" }

func (Emerge) Available() bool {
	_, err := lookPath("emerge")
	return err == nil
}

func (Emerge) NormalizeID(id string, managers map[string]string) (string, bool) {
	return normalizeID("emerge", id, managers)
}

func (Emerge) PlanInstall(pkgName string) []string {
	return []string{"sudo", "emerge", "--ask=n", pkgName}
}

func (Emerge) PlanUninstall(pkgName string) []string {
	return []string{"sudo", "emerge", "--ask=n", "--unmerge", pkgName}
}

func (Emerge) PlanUpgrade(pkgName string) []string {
	return []string{"sudo", "emerge", "--ask=n", "--update", pkgName}
}

func (Emerge) PlanClean() [][]string {
	return [][]string{
		{"sudo", "emerge", "--ask=n", "--depclean"},
	}
}

// Query reports whether pkgName is installed.
// "qlist -I <pkg>" exits 0 when the package is installed, 1 when absent.
// qlist is provided by app-portage/portage-utils, which is standard on Gentoo.
func (Emerge) Query(pkgName string) (bool, error) {
	return runQuery("qlist", "-I", pkgName)
}

// emergeQueryInstalled returns the "pkgname-version" components of all installed
// packages by running qlist -I and stripping the category prefix from each line.
// Shared by ListInstalled and QueryVersion to avoid forking qlist twice.
func emergeQueryInstalled() ([]string, error) {
	lines, err := runListOutput("qlist", "-I")
	if err != nil || len(lines) == 0 {
		return lines, err
	}
	result := make([]string, 0, len(lines))
	for _, line := range lines {
		if _, after, ok := strings.Cut(line, "/"); ok {
			result = append(result, after)
		} else {
			result = append(result, line)
		}
	}
	return result, nil
}

// Search returns package names from Portage repos whose name contains query.
// "emerge --search <query>" outputs blocks beginning with "*  category/name";
// we extract the package name and filter to those containing query (case-insensitive).
func (Emerge) Search(query string) ([]string, error) {
	lines, err := runListOutput("emerge", "--search", query)
	if err != nil || len(lines) == 0 {
		return lines, err
	}
	q := strings.ToLower(query)
	seen := make(map[string]bool)
	var names []string
	for _, line := range lines {
		if !strings.HasPrefix(line, "*") {
			continue
		}
		atom := strings.TrimSpace(strings.TrimPrefix(line, "*"))
		_, name, ok := strings.Cut(atom, "/")
		if !ok || name == "" {
			continue
		}
		if strings.Contains(strings.ToLower(name), q) && !seen[name] {
			seen[name] = true
			names = append(names, name)
		}
	}
	return names, nil
}

// ListInstalled returns the names of all packages installed via Portage.
// "qlist -I" outputs "category/pkgname-version" per line; we strip the
// category prefix and version suffix so callers receive plain package names.
// Deduplication is required because Portage allows multiple slots of the same
// package (e.g. python:3.11 and python:3.12), which share a base name.
func (Emerge) ListInstalled() ([]string, error) {
	nameVers, err := emergeQueryInstalled()
	if err != nil || len(nameVers) == 0 {
		return nameVers, err
	}
	seen := make(map[string]bool)
	var names []string
	for _, nameVer := range nameVers {
		name := trimVersionSuffix(nameVer)
		if name != "" && !seen[name] {
			seen[name] = true
			names = append(names, name)
		}
	}
	return names, nil
}

// QueryVersion returns the installed version of pkgName.
// "qlist -I" outputs "category/pkgname-version" per line; we scan for the
// first entry whose base name equals pkgName and extract the version from it.
func (Emerge) QueryVersion(pkgName string) (string, error) {
	nameVers, err := emergeQueryInstalled()
	if err != nil || len(nameVers) == 0 {
		return "", err
	}
	for _, nameVer := range nameVers {
		if trimVersionSuffix(nameVer) == pkgName {
			return strings.TrimPrefix(nameVer, pkgName+"-"), nil
		}
	}
	return "", nil
}
