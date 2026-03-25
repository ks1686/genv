package adapter

import "strings"

// Zypper is the adapter for the zypper package manager (openSUSE / SLES).
// zypper uses RPM as its backend, so Query, ListInstalled, and QueryVersion
// delegate to rpm commands directly.
type Zypper struct{}

func (Zypper) Name() string { return "zypper" }

func (Zypper) Available() bool {
	_, err := lookPath("zypper")
	return err == nil
}

func (Zypper) NormalizeID(id string, managers map[string]string) (string, bool) {
	return normalizeID("zypper", id, managers)
}

// PlanInstall uses --non-interactive as a global flag (before the subcommand),
// which is zypper's canonical way to suppress all prompts.
func (Zypper) PlanInstall(pkgName string) []string {
	return []string{"sudo", "zypper", "--non-interactive", "install", pkgName}
}

func (Zypper) PlanUninstall(pkgName string) []string {
	return []string{"sudo", "zypper", "--non-interactive", "remove", pkgName}
}

func (Zypper) PlanUpgrade(pkgName string) []string {
	return []string{"sudo", "zypper", "--non-interactive", "update", pkgName}
}

func (Zypper) PlanClean() [][]string {
	// -a cleans all repository caches, not just metadata.
	return [][]string{
		{"sudo", "zypper", "clean", "-a"},
	}
}

func (Zypper) Query(pkgName string) (bool, error) { return rpmQuery(pkgName) }

// Search returns package names from zypper repos whose name contains query.
// "zypper search" outputs a pipe-delimited table; we parse the Name column
// and skip separator lines, the header row, and non-matching names.
func (Zypper) Search(query string) ([]string, error) {
	lines, err := runListOutput("zypper", "search", query)
	if err != nil || len(lines) == 0 {
		return lines, err
	}
	return parseZypperSearch(lines, query), nil
}

func (Zypper) ListInstalled() ([]string, error)            { return rpmListInstalled() }
func (Zypper) QueryVersion(pkgName string) (string, error) { return rpmQueryVersion(pkgName) }

// parseZypperSearch parses the pipe-delimited table produced by zypper search
// and returns package names whose name contains query (case-insensitive).
// Table format: "S | Name | Summary | Type" with "---+..." separator rows.
func parseZypperSearch(lines []string, query string) []string {
	q := strings.ToLower(query)
	seen := make(map[string]bool)
	var names []string
	for _, line := range lines {
		// Skip separator rows ("---+...").
		if strings.HasPrefix(line, "-") {
			continue
		}
		parts := strings.SplitN(line, "|", 4)
		if len(parts) < 3 {
			continue
		}
		name := strings.TrimSpace(parts[1])
		// Skip the header row.
		if name == "Name" {
			continue
		}
		if name != "" && strings.Contains(strings.ToLower(name), q) && !seen[name] {
			seen[name] = true
			names = append(names, name)
		}
	}
	return names
}
