package adapter

import "strings"

// Apk is the adapter for the APK package manager (Alpine Linux).
type Apk struct{}

func (Apk) Name() string { return "apk" }

func (Apk) Available() bool {
	_, err := lookPath("apk")
	return err == nil
}

func (Apk) NormalizeID(id string, managers map[string]string) (string, bool) {
	return normalizeID("apk", id, managers)
}

func (Apk) PlanInstall(pkgName string) []string {
	return []string{"sudo", "apk", "add", pkgName}
}

func (Apk) PlanUninstall(pkgName string) []string {
	return []string{"sudo", "apk", "del", pkgName}
}

// PlanUpgrade delegates to PlanInstall: apk add upgrades a package to the
// latest available version when it is already installed.
func (Apk) PlanUpgrade(pkgName string) []string { return Apk{}.PlanInstall(pkgName) }

func (Apk) PlanClean() [][]string {
	return [][]string{
		{"sudo", "apk", "cache", "clean"},
	}
}

func (Apk) Query(pkgName string) (bool, error) {
	// apk info -e exits 0 when the package is installed, 1 when absent.
	return runQuery("apk", "info", "-e", pkgName)
}

// Search returns package names from apk repos whose name contains query.
// "apk search" outputs "pkgname-version" per line; we strip the version suffix
// and filter to names containing query (case-insensitive).
func (Apk) Search(query string) ([]string, error) {
	lines, err := runListOutput("apk", "search", query)
	if err != nil || len(lines) == 0 {
		return lines, err
	}
	q := strings.ToLower(query)
	seen := make(map[string]bool)
	var names []string
	for _, line := range lines {
		name := trimVersionSuffix(line)
		if name != "" && strings.Contains(strings.ToLower(name), q) && !seen[name] {
			seen[name] = true
			names = append(names, name)
		}
	}
	return names, nil
}

// ListInstalled returns the names of all installed packages.
// "apk info" lists every installed package as "pkgname-version"; we strip the
// version suffix so callers receive plain package names.
func (Apk) ListInstalled() ([]string, error) {
	lines, err := runListOutput("apk", "info")
	if err != nil || len(lines) == 0 {
		return lines, err
	}
	names := make([]string, 0, len(lines))
	for _, line := range lines {
		if name := trimVersionSuffix(line); name != "" {
			names = append(names, name)
		}
	}
	return names, nil
}

// QueryVersion returns the installed version of pkgName.
// "apk info <pkg>" first line is "pkgname-version description:"; we strip the
// package-name prefix to isolate the version string.
func (Apk) QueryVersion(pkgName string) (string, error) {
	lines, err := runListOutput("apk", "info", pkgName)
	if err != nil || len(lines) == 0 {
		return "", err
	}
	// First line: "pkgname-version description:"
	first, _, _ := strings.Cut(lines[0], " description:")
	ver, _ := strings.CutPrefix(first, pkgName+"-")
	return ver, nil
}
