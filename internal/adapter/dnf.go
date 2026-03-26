package adapter

import "strings"

// Dnf is the adapter for the DNF package manager (Fedora/RHEL).
type Dnf struct{}

func (Dnf) Name() string { return "dnf" }

func (Dnf) Available() bool {
	_, err := lookPath("dnf")
	return err == nil
}

func (Dnf) NormalizeID(id string, managers map[string]string) (string, bool) {
	return normalizeID("dnf", id, managers)
}

func (Dnf) PlanInstall(pkgName string) []string {
	return []string{"sudo", "dnf", "install", "-y", pkgName}
}

func (Dnf) PlanUninstall(pkgName string) []string {
	return []string{"sudo", "dnf", "remove", "-y", pkgName}
}

func (Dnf) PlanUpgrade(pkgName string) []string {
	return []string{"sudo", "dnf", "upgrade", "-y", pkgName}
}

func (Dnf) PlanClean() [][]string {
	return [][]string{
		{"sudo", "dnf", "autoremove", "-y"},
		{"sudo", "dnf", "clean", "all"},
	}
}

func (Dnf) Query(pkgName string) (bool, error) { return runQuery("rpm", "-q", pkgName) }

// Search returns package names from dnf repos whose name contains query.
// "dnf search" output mixes metadata lines, section headers (===), and
// package entries of the form "name.arch : description". We extract the name
// part (before the last dot) from package-entry lines only.
func (Dnf) Search(query string) ([]string, error) {
	lines, err := runListOutput("dnf", "search", query)
	if err != nil || len(lines) == 0 {
		return lines, err
	}
	var names []string
	seen := make(map[string]bool)
	for _, line := range lines {
		// Skip section headers and metadata lines.
		if strings.HasPrefix(line, "=") || strings.HasPrefix(line, "Last metadata") || strings.HasPrefix(line, "Error") {
			continue
		}
		// Package lines: "name.arch : description"
		pkgPart, _, ok := strings.Cut(line, " : ")
		if !ok {
			continue
		}
		pkgPart = strings.TrimSpace(pkgPart)
		// Strip arch suffix: "firefox.x86_64" → "firefox"
		if dot := strings.LastIndex(pkgPart, "."); dot > 0 {
			pkgPart = pkgPart[:dot]
		}
		if pkgPart != "" && containsFold(pkgPart, query) && !seen[pkgPart] {
			seen[pkgPart] = true
			names = append(names, pkgPart)
		}
	}
	return names, nil
}

func (Dnf) ListInstalled() ([]string, error) {
	return runListOutput("rpm", "-qa", "--qf", "%{NAME}\\n")
}

func (Dnf) QueryVersion(pkgName string) (string, error) {
	return runVersionOutput("rpm", "-q", "--qf", "%{VERSION}", pkgName)
}
