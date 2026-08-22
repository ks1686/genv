package adapter

import (
	"errors"
	"os/exec"
	"strings"
)

// DotnetTool manages globally installed .NET tools via `dotnet tool ... --global`.
// It never operates on tool manifests or local tools.
type DotnetTool struct{}

func (DotnetTool) Name() string { return "dotnet-tool" }

func (DotnetTool) Available() bool {
	_, err := lookPath("dotnet")
	return err == nil
}

func (DotnetTool) NormalizeID(id string, managers map[string]string) (string, bool) {
	return normalizeID("dotnet-tool", id, managers)
}

func (DotnetTool) PlanInstall(pkgName string) []string {
	return []string{"dotnet", "tool", "install", "--global", pkgName}
}

func (DotnetTool) PlanUninstall(pkgName string) []string {
	return []string{"dotnet", "tool", "uninstall", "--global", atVersionBaseName(pkgName)}
}

func (DotnetTool) PlanUpgrade(pkgName string) []string {
	return []string{"dotnet", "tool", "update", "--global", atVersionBaseName(pkgName)}
}

func (DotnetTool) PlanClean() [][]string { return nil }

func (d DotnetTool) Query(pkgName string) (bool, error) {
	entries, err := d.listEntries()
	if err != nil {
		return false, err
	}
	_, ok := findDotnetEntry(entries, pkgName)
	return ok, nil
}

func (d DotnetTool) ListInstalled() ([]string, error) {
	entries, err := d.listEntries()
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.name)
	}
	return names, nil
}

func (d DotnetTool) QueryVersion(pkgName string) (string, error) {
	entries, err := d.listEntries()
	if err != nil {
		return "", err
	}
	entry, ok := findDotnetEntry(entries, pkgName)
	if !ok {
		return "", nil
	}
	return entry.version, nil
}

func (d DotnetTool) ListInstalledVersions() (map[string]string, error) {
	entries, err := d.listEntries()
	if err != nil {
		return nil, err
	}
	versions := make(map[string]string, len(entries))
	for _, entry := range entries {
		versions[entry.name] = entry.version
	}
	return versions, nil
}

type dotnetEntry struct {
	name    string
	version string
}

func (DotnetTool) listEntries() ([]dotnetEntry, error) {
	out, err := runProbe("dotnet", "tool", "list", "--global")
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return nil, nil
		}
		return nil, err
	}
	return parseDotnetToolList(string(out)), nil
}

func findDotnetEntry(entries []dotnetEntry, pkgName string) (dotnetEntry, bool) {
	base := strings.ToLower(atVersionBaseName(pkgName))
	for _, entry := range entries {
		if entry.name == base {
			return entry, true
		}
	}
	return dotnetEntry{}, false
}

// parseDotnetToolList parses `dotnet tool list --global` output. The output is
// a table: a header row ("Package Id  Version  Commands"), a separator row of
// dashes, then one row per tool. Package Ids are compared case-insensitively
// because NuGet package ids are case-insensitive.
func parseDotnetToolList(out string) []dotnetEntry {
	var entries []dotnetEntry
	for line := range strings.SplitSeq(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		if strings.EqualFold(fields[0], "package") && strings.EqualFold(fields[1], "id") {
			continue
		}
		if strings.HasPrefix(fields[0], "---") {
			continue
		}
		entries = append(entries, dotnetEntry{name: strings.ToLower(fields[0]), version: fields[1]})
	}
	return entries
}
