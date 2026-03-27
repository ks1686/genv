package adapter

import "strings"

// Xbps is the adapter for the XBPS package manager (Void Linux).
type Xbps struct{}

func (Xbps) Name() string { return "xbps" }

func (Xbps) Available() bool {
	_, err := lookPath("xbps-install")
	return err == nil
}

func (Xbps) NormalizeID(id string, managers map[string]string) (string, bool) {
	return normalizeID("xbps", id, managers)
}

func (Xbps) PlanInstall(pkgName string) []string {
	return []string{"sudo", "xbps-install", "-Sy", pkgName}
}

func (Xbps) PlanUninstall(pkgName string) []string {
	return []string{"sudo", "xbps-remove", "-Ry", pkgName}
}

func (Xbps) PlanUpgrade(pkgName string) []string {
	return Xbps{}.PlanInstall(pkgName)
}

func (Xbps) PlanClean() [][]string {
	return [][]string{
		{"sudo", "xbps-remove", "-O"},
	}
}

func (Xbps) Query(pkgName string) (bool, error) {
	return runQuery("xbps-query", pkgName)
}

func (Xbps) Search(query string) ([]string, error) {
	lines, err := runListOutput("xbps-query", "-Rs", query)
	if err != nil || len(lines) == 0 {
		return lines, err
	}
	q := strings.ToLower(query)
	seen := make(map[string]bool)
	var names []string
	for _, line := range lines {
		// Example line: "[*] gimp-2.10.32_2          GNU Image Manipulation Program"
		parts := strings.Fields(line)
		if len(parts) >= 2 {
			name := parts[1]
			if name != "" && strings.Contains(strings.ToLower(name), q) && !seen[name] {
				seen[name] = true
				names = append(names, name)
			}
		}
	}
	return names, nil
}

func (Xbps) ListInstalled() ([]string, error) {
	lines, err := runListOutput("xbps-query", "-l")
	if err != nil || len(lines) == 0 {
		return lines, err
	}
	var names []string
	for _, line := range lines {
		// Example line: "ii gimp-2.10.32_2          GNU Image Manipulation Program"
		parts := strings.Fields(line)
		if len(parts) >= 2 {
			nameVersion := parts[1]
			name := trimVersionSuffix(nameVersion)
			if name != "" {
				names = append(names, name)
			}
		}
	}
	return names, nil
}

func (Xbps) QueryVersion(pkgName string) (string, error) {
	// Use pkgver (e.g. "curl-8.1.2_1") which is stable across xbps versions.
	// The -p version property was unreliable after xbps self-updates.
	lines, err := runListOutput("xbps-query", "-p", "pkgver", pkgName)
	if err != nil || len(lines) == 0 {
		return "", err
	}
	// Strip "pkgname-" prefix, leaving "version_revision".
	pkgver := lines[0]
	if idx := strings.LastIndex(pkgver, "-"); idx >= 0 {
		pkgver = pkgver[idx+1:]
	}
	// Strip "_revision" suffix.
	if idx := strings.Index(pkgver, "_"); idx >= 0 {
		pkgver = pkgver[:idx]
	}
	return pkgver, nil
}
