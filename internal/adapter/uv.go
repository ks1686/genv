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

// PlanClean returns nil: uv has no standard tool-only cache-clean command.
func (Uv) PlanClean() [][]string { return nil }

func (Uv) Query(pkgName string) (bool, error) {
	name := uvToolName(pkgName)
	tools, err := Uv{}.ListInstalled()
	if err != nil {
		return false, err
	}
	for _, tool := range tools {
		if tool == name {
			return true, nil
		}
	}
	return false, nil
}

// ListInstalled parses "uv tool list" output. Top-level lines name the tool
// (e.g. "black v24.2.0"); indented lines are entrypoints and are skipped.
func (Uv) ListInstalled() ([]string, error) {
	lines, err := runUvToolList()
	if err != nil {
		return nil, err
	}
	var names []string
	for _, line := range lines {
		if line == "" || isIndented(line) {
			continue
		}
		if fields := strings.Fields(line); len(fields) > 0 {
			names = append(names, fields[0])
		}
	}
	return names, nil
}

// QueryVersion parses the installed version from "uv tool list" output.
// Returns "", nil when the tool is not installed or has no reported version.
func (Uv) QueryVersion(pkgName string) (string, error) {
	name := uvToolName(pkgName)
	lines, err := runUvToolList()
	if err != nil {
		return "", err
	}
	for _, line := range lines {
		if line == "" || isIndented(line) {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		if fields[0] != name {
			continue
		}
		// Output is "<tool> v<version>" or "<tool> <version>".
		return strings.TrimPrefix(fields[1], "v"), nil
	}
	return "", nil
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
	var result []string
	for _, line := range strings.Split(string(out), "\n") {
		if line != "" {
			result = append(result, line)
		}
	}
	return result, nil
}

// uvToolName strips an optional @version suffix so that "ruff@0.6.0" is
// treated as the tool "ruff". uv tool uninstall and status checks expect the
// bare tool name, while uv tool install accepts the full specifier.
func uvToolName(s string) string {
	if before, _, ok := strings.Cut(s, "@"); ok {
		return before
	}
	return s
}

// isIndented reports whether line starts with horizontal whitespace.
func isIndented(line string) bool {
	return strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t")
}
