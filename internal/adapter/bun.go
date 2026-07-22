package adapter

import (
	"strings"
	"unicode/utf8"
)

// Bun is the adapter for Bun's global package installs.
// It manages only globally-installed packages (`bun add --global`); local
// project installs are intentionally out of scope.
type Bun struct{}

func (Bun) Name() string { return "bun" }

func (Bun) Available() bool {
	_, err := lookPath("bun")
	return err == nil
}

func (Bun) NormalizeID(id string, managers map[string]string) (string, bool) {
	return normalizeID("bun", id, managers)
}

func (Bun) PlanInstall(pkgName string) []string {
	return []string{"bun", "add", "--global", pkgName}
}

func (Bun) PlanUninstall(pkgName string) []string {
	return []string{"bun", "remove", "--global", bunBaseName(pkgName)}
}

func (Bun) PlanUpgrade(pkgName string) []string {
	// `bun update --global <pkg>` looks for a local package.json and is a no-op
	// for globally-installed packages ("No package.json, so nothing to update"),
	// which breaks scheduled upgrades that run without a project directory.
	// `bun add --global <pkg>` reinstalls the latest version, which is the
	// correct, package.json-independent way to upgrade a global package.
	return []string{"bun", "add", "--global", bunBaseName(pkgName)}
}

func (Bun) PlanClean() [][]string {
	return [][]string{{"bun", "pm", "cache", "rm", "--global"}}
}

func (b Bun) Query(pkgName string) (bool, error) {
	entries, err := b.listEntries()
	if err != nil {
		return false, err
	}
	base := bunBaseName(pkgName)
	for _, entry := range entries {
		if entry.name == base {
			return true, nil
		}
	}
	return false, nil
}

func (Bun) ListInstalled() ([]string, error) {
	entries, err := Bun{}.listEntries()
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.name)
	}
	return names, nil
}

func (Bun) QueryVersion(pkgName string) (string, error) {
	entries, err := Bun{}.listEntries()
	if err != nil {
		return "", err
	}
	base := bunBaseName(pkgName)
	for _, entry := range entries {
		if entry.name == base {
			return entry.version, nil
		}
	}
	return "", nil
}

func (Bun) ListInstalledVersions() (map[string]string, error) {
	entries, err := Bun{}.listEntries()
	if err != nil {
		return nil, err
	}
	versions := make(map[string]string, len(entries))
	for _, entry := range entries {
		versions[entry.name] = entry.version
	}
	return versions, nil
}

// ListOutdated reports globally-installed bun packages with a newer version on
// the npm registry, keyed by package base name -> latest version, restricted to
// pkgNames when provided. bun has no reliable global "outdated" command, so genv
// compares each installed version against the registry "latest" dist-tag. A
// package whose latest version can't be resolved due to a transport error is
// conservatively reported as outdated so a real update is never silently missed;
// a 404 (unpublished) is treated as up to date.
func (Bun) ListOutdated(pkgNames []string) (map[string]string, error) {
	entries, err := Bun{}.listEntries()
	if err != nil {
		return nil, err
	}
	installed := make(map[string]string, len(entries))
	for _, e := range entries {
		installed[e.name] = e.version
	}
	return listJSOutdated(installed, pkgNames)
}

type bunEntry struct {
	name    string
	version string
}

func (Bun) listEntries() ([]bunEntry, error) {
	lines, err := runListOutput("bun", "pm", "ls", "--global")
	if err != nil {
		return nil, err
	}
	return parseBunEntries(lines), nil
}

// bunBaseName strips an @version suffix from a package spec, returning the
// canonical package name used for matching installed packages.
// e.g. "cf@latest" -> "cf", "@scope/pkg@1.0.0" -> "@scope/pkg".
func bunBaseName(pkgName string) string {
	return jsBasePackageName(pkgName)
}

func parseBunEntries(lines []string) []bunEntry {
	var entries []bunEntry
	for _, line := range lines {
		if name, version, ok := parseBunListLine(line); ok {
			entries = append(entries, bunEntry{name: name, version: version})
		}
	}
	return entries
}

// parseBunListLine parses a single line of `bun pm ls --global` output.
// It returns the package base name and version. Lines that are not package
// entries (e.g. the header path line) return ok == false.
func parseBunListLine(line string) (name, version string, ok bool) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return "", "", false
	}
	r, _ := utf8.DecodeRuneInString(trimmed)
	if r != '│' && r != '├' && r != '└' {
		return "", "", false
	}

	body := strings.TrimLeft(trimmed, " │├└─")
	if body == "" {
		return "", "", false
	}

	if strings.HasPrefix(body, "@") {
		base := jsBasePackageName(body)
		if base != body {
			return base, body[len(base)+1:], true
		}
		return body, "", true
	}

	if idx := strings.Index(body, "@"); idx > 0 {
		return body[:idx], body[idx+1:], true
	}
	return body, "", true
}
