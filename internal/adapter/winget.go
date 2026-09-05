package adapter

import (
	"context"
	"strings"
)

// Winget is the adapter for winget, the package manager built into Windows
// 10 (1709+) and Windows 11. Packages are addressed by their winget package
// ID (e.g. "Neovim.Neovim"), which never contains whitespace, unlike the
// human-readable Name column winget also prints.
type Winget struct{}

func (Winget) Name() string { return "winget" }

func (Winget) Available() bool {
	_, err := lookPath("winget")
	return err == nil
}

func (Winget) NormalizeID(id string, managers map[string]string) (string, bool) {
	return normalizeID("winget", id, managers)
}

func (Winget) PlanInstall(pkgName string) []string {
	return []string{"winget", "install", "--exact", "--silent", "--disable-interactivity", "--no-upgrade", "--accept-package-agreements", "--accept-source-agreements", "--id", pkgName}
}

func (Winget) PlanUninstall(pkgName string) []string {
	return []string{"winget", "uninstall", "--exact", "--silent", "--disable-interactivity", "--id", pkgName}
}

func (Winget) PlanUpgrade(pkgName string) []string {
	return []string{"winget", "upgrade", "--exact", "--silent", "--disable-interactivity", "--accept-package-agreements", "--accept-source-agreements", "--id", pkgName}
}

func (Winget) PlanRefresh() []string {
	return []string{"winget", "source", "update"}
}

// PlanClean returns nil: winget has no standalone cache-clean command.
func (Winget) PlanClean() [][]string { return nil }

// Query reports whether pkgName is installed. "winget list --id <id> --exact"
// exits non-zero ("No installed package found...") when absent, matching the
// runQuery convention used by every other adapter.
func (Winget) Query(pkgName string) (bool, error) {
	return runQuery("winget", "list", "--id", pkgName, "--exact")
}

// Search returns winget package IDs whose listing contains query.
func (Winget) Search(query string) ([]string, error) {
	return Winget{}.SearchContext(context.Background(), query)
}

func (Winget) SearchContext(ctx context.Context, query string) ([]string, error) {
	lines, err := runListOutputContext(ctx, "winget", "search", "--query", query)
	if err != nil || len(lines) == 0 {
		return nil, err
	}
	rows := parseWingetTable(lines)
	names := make([]string, 0, len(rows))
	for _, r := range rows {
		if r.id != "" {
			names = append(names, r.id)
		}
	}
	return names, nil
}

// ListInstalled returns the winget package IDs of every package winget
// itself can manage (i.e. rows with a non-empty Source column). "winget
// list" with no filter also reports many entries winget only discovered via
// the Windows registry or MSIX (blank Source) that it cannot install,
// upgrade, or uninstall; those are excluded.
func (Winget) ListInstalled() ([]string, error) {
	lines, err := runListOutput("winget", "list")
	if err != nil {
		return nil, err
	}
	rows := parseWingetTable(lines)
	var names []string
	for _, r := range rows {
		if r.id != "" && r.source != "" {
			names = append(names, r.id)
		}
	}
	return names, nil
}

func (Winget) QueryVersion(pkgName string) (string, error) {
	out, err := runListOutput("winget", "list", "--id", pkgName, "--exact")
	if err != nil || len(out) == 0 {
		return "", err
	}
	rows := parseWingetTable(out)
	if len(rows) == 0 {
		return "", nil
	}
	return rows[0].version, nil
}

// ListOutdated reports winget package IDs with an available upgrade, keyed by
// ID -> target version, intersected with pkgNames.
func (Winget) ListOutdated(pkgNames []string) (map[string]string, error) {
	out, err := runProbe("winget", "upgrade")
	if err != nil {
		return nil, err
	}
	outdated := make(map[string]string)
	for _, row := range parseWingetTable(trimmedNonEmptyLines(string(out))) {
		if row.id != "" && row.available != "" {
			outdated[row.id] = row.available
		}
	}
	return intersectNameMap(outdated, pkgNames), nil
}

type wingetRow struct {
	id        string
	version   string
	available string
	source    string
}

// parseWingetTable parses winget's fixed-width table output (used by both
// "winget list" and "winget search"). Column boundaries are derived from the
// header line's field start offsets rather than whitespace-splitting each
// row, because the Name column may itself contain spaces.
func parseWingetTable(lines []string) []wingetRow {
	if len(lines) < 2 {
		return nil
	}
	header := lines[0]
	idStart := strings.Index(header, "Id")
	versionStart := strings.Index(header, "Version")
	if idStart < 0 || versionStart < 0 || versionStart <= idStart {
		return nil
	}
	availableStart := strings.Index(header, "Available")
	// The optional "Available" column (only present in bare "list" output)
	// sits between Version and Source; either way, Source is the last
	// column we care about and everything after Version up to it (or end
	// of line) is irrelevant to us except for locating Source itself.
	sourceStart := strings.LastIndex(header, "Source")

	var rows []wingetRow
	for _, line := range lines[1:] {
		if line == "" || strings.HasPrefix(line, "---") {
			continue
		}
		if len(line) <= idStart {
			continue
		}
		row := wingetRow{}
		end := min(versionStart, len(line))
		row.id = strings.TrimSpace(line[idStart:end])
		if row.id == "" {
			continue
		}
		if versionStart < len(line) {
			verEnd := len(line)
			if availableStart > versionStart && availableStart < len(line) {
				verEnd = availableStart
			}
			if sourceStart > versionStart && sourceStart < len(line) {
				verEnd = min(verEnd, sourceStart)
			}
			row.version = strings.TrimSpace(firstField(line[versionStart:verEnd]))
		}
		if availableStart > versionStart && availableStart < len(line) {
			availableEnd := len(line)
			if sourceStart > availableStart && sourceStart < len(line) {
				availableEnd = sourceStart
			}
			row.available = strings.TrimSpace(firstField(line[availableStart:availableEnd]))
		}
		if sourceStart > 0 && sourceStart < len(line) {
			row.source = strings.TrimSpace(line[sourceStart:])
		}
		rows = append(rows, row)
	}
	return rows
}

// firstField returns the first whitespace-delimited token in s, used to pull
// just the Version value out of a slice that may also contain the
// Available-upgrade column when both are present.
func firstField(s string) string {
	fields := strings.Fields(s)
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}
