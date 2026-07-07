package adapter

import "strings"

// Scoop is the adapter for scoop, a CLI-focused Windows package manager that
// installs into the user profile (no admin/UAC required for most packages).
type Scoop struct{}

func (Scoop) Name() string { return "scoop" }

func (Scoop) Available() bool {
	_, err := lookPath("scoop")
	return err == nil
}

func (Scoop) NormalizeID(id string, managers map[string]string) (string, bool) {
	return normalizeID("scoop", id, managers)
}

func (Scoop) PlanInstall(pkgName string) []string {
	return []string{"scoop", "install", pkgName}
}

func (Scoop) PlanUninstall(pkgName string) []string {
	return []string{"scoop", "uninstall", pkgName}
}

func (Scoop) PlanUpgrade(pkgName string) []string {
	return []string{"scoop", "update", pkgName}
}

// PlanClean clears scoop's downloaded-installer cache for every app.
func (Scoop) PlanClean() [][]string {
	return [][]string{{"scoop", "cache", "rm", "*"}}
}

// Query reports whether pkgName is installed, by checking scoop's own
// installed-apps list rather than relying on a per-package exit code (scoop
// list <app> does not reliably signal absence via exit status).
func (Scoop) Query(pkgName string) (bool, error) {
	installed, err := Scoop{}.ListInstalled()
	if err != nil {
		return false, err
	}
	for _, name := range installed {
		if name == pkgName {
			return true, nil
		}
	}
	return false, nil
}

// Search returns scoop app names containing query.
func (Scoop) Search(query string) ([]string, error) {
	lines, err := runListOutput("scoop", "search", query)
	if err != nil || len(lines) == 0 {
		return lines, err
	}
	var names []string
	for _, name := range parseScoopList(lines) {
		if containsFold(name.name, query) {
			names = append(names, name.name)
		}
	}
	return names, nil
}

// ListInstalled parses "scoop list" output. The table is colorized (ANSI
// escape codes) and column-aligned; app names never contain spaces, so a
// simple first-field split per data row is sufficient once escape codes and
// header/separator/blank lines are stripped.
func (Scoop) ListInstalled() ([]string, error) {
	lines, err := runListOutput("scoop", "list")
	if err != nil {
		return nil, err
	}
	rows := parseScoopList(lines)
	names := make([]string, 0, len(rows))
	for _, r := range rows {
		names = append(names, r.name)
	}
	return names, nil
}

func (Scoop) QueryVersion(pkgName string) (string, error) {
	lines, err := runListOutput("scoop", "list")
	if err != nil {
		return "", err
	}
	for _, r := range parseScoopList(lines) {
		if r.name == pkgName {
			return r.version, nil
		}
	}
	return "", nil
}

type scoopEntry struct {
	name    string
	version string
}

// parseScoopList strips ANSI color escapes and parses "scoop list" /
// "scoop search" output, skipping the "Installed apps:" banner, the header
// row, the "----" separator row, and blank lines.
func parseScoopList(lines []string) []scoopEntry {
	var entries []scoopEntry
	for _, line := range lines {
		clean := stripANSI(line)
		trimmed := strings.TrimSpace(clean)
		if trimmed == "" || strings.HasPrefix(trimmed, "Installed apps") || strings.HasPrefix(trimmed, "----") || strings.HasPrefix(trimmed, "Name") {
			continue
		}
		fields := strings.Fields(trimmed)
		if len(fields) < 2 {
			continue
		}
		entries = append(entries, scoopEntry{name: fields[0], version: fields[1]})
	}
	return entries
}

// stripANSI removes "ESC[...m"-style SGR color escape sequences.
func stripANSI(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == 0x1b && i+1 < len(s) && s[i+1] == '[' {
			j := i + 2
			for j < len(s) && s[j] != 'm' {
				j++
			}
			i = j
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}
