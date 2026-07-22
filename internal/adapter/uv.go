package adapter

import (
	"errors"
	"os/exec"
	"strings"
)

// Uv is the adapter for uv global Python tool installs.
// It only operates on uv's tool environment ("uv tool install/uninstall"),
// never on project-level Python packages.
type Uv struct{}

func (Uv) Name() string { return "uv" }

func (Uv) Available() bool {
	_, err := lookPath("uv")
	return err == nil
}

func (Uv) NormalizeID(id string, managers map[string]string) (string, bool) {
	return normalizeID("uv", id, managers)
}

func (Uv) PlanInstall(pkgName string) []string {
	return []string{"uv", "tool", "install", pkgName}
}

func (Uv) PlanUninstall(pkgName string) []string {
	return []string{"uv", "tool", "uninstall", uvToolName(pkgName)}
}

// PlanUpgrade reuses "uv tool install --upgrade", which upgrades an installed
// tool to the latest version matching the requested specifier.
func (Uv) PlanUpgrade(pkgName string) []string {
	return []string{"uv", "tool", "install", "--upgrade", pkgName}
}

// PlanClean runs uv's global cache clean. There is no tool-only variant;
// this clears uv's shared wheel/build cache, which can grow unbounded.
func (Uv) PlanClean() [][]string {
	return [][]string{{"uv", "cache", "clean"}}
}

func (Uv) Query(pkgName string) (bool, error) {
	name := uvToolName(pkgName)
	entries, err := Uv{}.listEntries()
	if err != nil {
		return false, err
	}
	for _, entry := range entries {
		if entry.name == name {
			return true, nil
		}
	}
	return false, nil
}

// ListInstalled parses "uv tool list" output. Top-level lines name the tool
// (e.g. "black v24.2.0"); indented lines are entrypoints and are skipped.
func (Uv) ListInstalled() ([]string, error) {
	entries, err := Uv{}.listEntries()
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.name)
	}
	return names, nil
}

// QueryVersion parses the installed version from "uv tool list" output.
// Returns "", nil when the tool is not installed or has no reported version.
func (Uv) QueryVersion(pkgName string) (string, error) {
	name := uvToolName(pkgName)
	entries, err := Uv{}.listEntries()
	if err != nil {
		return "", err
	}
	for _, entry := range entries {
		if entry.name == name {
			return entry.version, nil
		}
	}
	return "", nil
}

func (Uv) ListInstalledVersions() (map[string]string, error) {
	entries, err := Uv{}.listEntries()
	if err != nil {
		return nil, err
	}
	versions := make(map[string]string, len(entries))
	for _, entry := range entries {
		versions[entry.name] = entry.version
	}
	return versions, nil
}

// ListOutdated reports installed uv tools with a newer version on PyPI, keyed
// by bare tool name -> latest version, restricted to pkgNames when provided.
func (Uv) ListOutdated(pkgNames []string) (map[string]string, error) {
	entries, err := Uv{}.listEntries()
	if err != nil {
		return nil, err
	}
	installed := make(map[string]string, len(entries))
	for _, entry := range entries {
		installed[entry.name] = entry.version
	}
	return listRegistryOutdated(installed, pkgNames, uvToolName, pypiLatestVersion)
}

type uvEntry struct {
	name    string
	version string
}

func (Uv) listEntries() ([]uvEntry, error) {
	lines, err := runUvToolList()
	if err != nil {
		return nil, err
	}
	entries := make([]uvEntry, 0, len(lines))
	for _, line := range lines {
		if line == "" || isIndented(line) {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		version := ""
		if len(fields) > 1 {
			// Output is "<tool> v<version>" or "<tool> <version>".
			version = strings.TrimPrefix(fields[1], "v")
		}
		entries = append(entries, uvEntry{name: fields[0], version: version})
	}
	return entries, nil
}

// runUvToolList runs "uv tool list" and returns stdout split into lines,
// preserving leading whitespace so indented entrypoint lines can be detected.
// A non-zero exit code is treated as "no tools" (nil, nil), not an error.
func runUvToolList() ([]string, error) {
	out, err := exec.Command("uv", "tool", "list").Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return nil, nil
		}
		return nil, err
	}
	return nonEmptyLines(string(out)), nil
}

// uvToolName strips an optional @version suffix so that "ruff@0.6.0" is
// treated as the tool "ruff". uv tool uninstall and status checks expect the
// bare tool name, while uv tool install accepts the full specifier.
func uvToolName(s string) string {
	return atVersionBaseName(s)
}

// isIndented reports whether line starts with horizontal whitespace.
func isIndented(line string) bool {
	return strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t")
}
