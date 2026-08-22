package adapter

import (
	"errors"
	"os/exec"
)

// Poetry is the adapter for poetry self plugins.
type Poetry struct{}

func (Poetry) Name() string { return "poetry" }

func (Poetry) Available() bool {
	_, err := lookPath("poetry")
	return err == nil
}

func (Poetry) NormalizeID(id string, managers map[string]string) (string, bool) {
	return normalizeID("poetry", id, managers)
}

func (Poetry) PlanInstall(pkgName string) []string {
	return []string{"poetry", "self", "add", pkgName}
}

func (Poetry) PlanUninstall(pkgName string) []string {
	return []string{"poetry", "self", "remove", PythonBasePackageName(pkgName)}
}

func (Poetry) PlanUpgrade(pkgName string) []string {
	return []string{"poetry", "self", "add", pkgName}
}

func (Poetry) PlanClean() [][]string {
	return [][]string{{"poetry", "cache", "clear", "--all", "pypi"}}
}

func (Poetry) Query(pkgName string) (bool, error) {
	name := PythonBasePackageName(pkgName)
	entries, err := Poetry{}.listEntries()
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

func (Poetry) ListInstalled() ([]string, error) {
	entries, err := Poetry{}.listEntries()
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.name)
	}
	return names, nil
}

func (Poetry) QueryVersion(pkgName string) (string, error) {
	name := PythonBasePackageName(pkgName)
	entries, err := Poetry{}.listEntries()
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

func (Poetry) ListInstalledVersions() (map[string]string, error) {
	entries, err := Poetry{}.listEntries()
	if err != nil {
		return nil, err
	}
	versions := make(map[string]string, len(entries))
	for _, entry := range entries {
		versions[entry.name] = entry.version
	}
	return versions, nil
}

func (Poetry) listEntries() ([]pythonEntry, error) {
	out, err := runProbe("poetry", "self", "show", "plugins")
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return nil, nil
		}
		return nil, err
	}
	return parsePoetryPluginsText(string(out))
}
