package adapter

import "strings"

// Snap is the adapter for the Snap package manager (Ubuntu/Canonical).
type Snap struct{}

func (Snap) Name() string { return "snap" }

func (Snap) Available() bool {
	_, err := lookPath("snap")
	return err == nil
}

func (Snap) NormalizeID(id string, managers map[string]string) (string, bool) {
	return normalizeID("snap", id, managers)
}

func (Snap) PlanInstall(pkgName string) []string {
	return []string{"sudo", "snap", "install", pkgName}
}

func (Snap) PlanUninstall(pkgName string) []string {
	return []string{"sudo", "snap", "remove", "--purge", pkgName}
}

func (Snap) PlanUpgrade(pkgName string) []string {
	return []string{"sudo", "snap", "refresh", pkgName}
}

// PlanClean returns nil: snap has no standard cache-clean command.
func (Snap) PlanClean() [][]string { return nil }

func (Snap) Query(pkgName string) (bool, error) { return runQuery("snap", "list", pkgName) }

// Search returns snap package names containing query.
// "snap find" output: header line then data lines of "name version publisher notes summary".
func (Snap) Search(query string) ([]string, error) {
	lines, err := runListOutput("snap", "find", query)
	if err != nil || len(lines) == 0 {
		return lines, err
	}
	q := strings.ToLower(query)
	var names []string
	for i, line := range lines {
		if i == 0 {
			continue // skip header
		}
		if fields := strings.Fields(line); len(fields) > 0 {
			name := fields[0]
			if strings.Contains(strings.ToLower(name), q) {
				names = append(names, name)
			}
		}
	}
	return names, nil
}

// ListInstalled parses "snap list" output, skipping the header line.
func (Snap) ListInstalled() ([]string, error) {
	lines, err := runListOutput("snap", "list")
	if err != nil {
		return nil, err
	}
	// First line is the header; extract the package name (first field) from data lines.
	var names []string
	for i, line := range lines {
		if i == 0 {
			continue // skip header
		}
		if fields := strings.Fields(line); len(fields) > 0 {
			names = append(names, fields[0])
		}
	}
	return names, nil
}

func (Snap) QueryVersion(pkgName string) (string, error) {
	// "snap list pkgname" → header line then data line with version in column 2.
	out, err := runVersionOutput("snap", "list", pkgName)
	if err != nil || out == "" {
		return out, err
	}
	lines := strings.Split(out, "\n")
	if len(lines) >= 2 {
		if fields := strings.Fields(lines[1]); len(fields) >= 2 {
			return fields[1], nil
		}
	}
	return "", nil
}
