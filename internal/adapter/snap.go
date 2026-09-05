package adapter

import (
	"context"
	"strings"
)

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

// PlanUpgradeBatch refreshes multiple snaps in one snap invocation.
func (Snap) PlanUpgradeBatch(pkgNames []string) []string {
	args := []string{"sudo", "snap", "refresh"}
	return append(args, pkgNames...)
}

// PlanClean returns nil: snap has no standard cache-clean command.
func (Snap) PlanClean() [][]string { return nil }

func (Snap) Query(pkgName string) (bool, error) { return runQuery("snap", "list", pkgName) }

// Search returns snap package names containing query.
// "snap find" output: header line then data lines of "name version publisher notes summary".
func (Snap) Search(query string) ([]string, error) {
	return Snap{}.SearchContext(context.Background(), query)
}

func (Snap) SearchContext(ctx context.Context, query string) ([]string, error) {
	lines, err := runListOutputContext(ctx, "snap", "find", query)
	if err != nil || len(lines) == 0 {
		return lines, err
	}
	var names []string
	for i, line := range lines {
		if i == 0 {
			continue // skip header
		}
		if fields := strings.Fields(line); len(fields) > 0 {
			name := fields[0]
			if containsFold(name, query) {
				names = append(names, name)
			}
		}
	}
	return names, nil
}

// ListInstalled parses "snap list" output, skipping the header line.
func (Snap) ListInstalled() ([]string, error) {
	return snapList()
}

// ListInstalledVersions returns the installed version of every snap. This
// satisfies VersionLister for batch upgrade version refresh.
func (Snap) ListInstalledVersions() (map[string]string, error) {
	return snapListVersions()
}

func snapList() ([]string, error) {
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

func snapListVersions() (map[string]string, error) {
	lines, err := runListOutput("snap", "list")
	if err != nil {
		return nil, err
	}
	versions := make(map[string]string, len(lines))
	for i, line := range lines {
		if i == 0 {
			continue // skip header
		}
		if fields := strings.Fields(line); len(fields) >= 2 {
			versions[fields[0]] = fields[1]
		}
	}
	return versions, nil
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

// ListOutdated reports snaps with an available refresh, keyed by snap name
// -> target version, intersected with pkgNames. `snap refresh --list` is
// store-live, so Snap does not implement IndexRefresher.
func (Snap) ListOutdated(pkgNames []string) (map[string]string, error) {
	out, err := runProbe("snap", "refresh", "--list")
	if err != nil {
		return nil, err
	}
	return intersectNameMap(parseSnapRefreshList(trimmedNonEmptyLines(string(out))), pkgNames), nil
}

// parseSnapRefreshList parses "snap refresh --list" output, skipping the header
// line. The first field is the snap name; the second is the target version.
func parseSnapRefreshList(lines []string) map[string]string {
	out := map[string]string{}
	for i, line := range lines {
		if i == 0 {
			continue // skip header
		}
		if fields := strings.Fields(line); len(fields) >= 2 {
			out[fields[0]] = fields[1]
		}
	}
	return out
}
