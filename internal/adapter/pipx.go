package adapter

import (
	"errors"
	"os/exec"
)

// Pipx is the adapter for pipx global Python tool installs.
type Pipx struct{}

func (Pipx) Name() string { return "pipx" }

func (Pipx) Available() bool {
	_, err := lookPath("pipx")
	return err == nil
}

func (Pipx) NormalizeID(id string, managers map[string]string) (string, bool) {
	return normalizeID("pipx", id, managers)
}

func (Pipx) PlanInstall(pkgName string) []string {
	return []string{"pipx", "install", pkgName}
}

func (Pipx) PlanUninstall(pkgName string) []string {
	return []string{"pipx", "uninstall", PythonBasePackageName(pkgName)}
}

func (Pipx) PlanUpgrade(pkgName string) []string {
	return []string{"pipx", "install", "--force", pkgName}
}

func (Pipx) PlanClean() [][]string {
	return nil
}

func (Pipx) Query(pkgName string) (bool, error) {
	name := PythonBasePackageName(pkgName)
	entries, err := Pipx{}.listEntries()
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

func (Pipx) ListInstalled() ([]string, error) {
	entries, err := Pipx{}.listEntries()
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.name)
	}
	return names, nil
}

func (Pipx) QueryVersion(pkgName string) (string, error) {
	name := PythonBasePackageName(pkgName)
	entries, err := Pipx{}.listEntries()
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

func (Pipx) ListInstalledVersions() (map[string]string, error) {
	entries, err := Pipx{}.listEntries()
	if err != nil {
		return nil, err
	}
	versions := make(map[string]string, len(entries))
	for _, entry := range entries {
		versions[entry.name] = entry.version
	}
	return versions, nil
}

// ListOutdated reports installed pipx tools with a newer version on PyPI,
// keyed by bare package name -> latest version, restricted to pkgNames when
// provided.
func (Pipx) ListOutdated(pkgNames []string) (map[string]string, error) {
	entries, err := Pipx{}.listEntries()
	if err != nil {
		return nil, err
	}
	return listRegistryOutdated(versionMapOf(entries, func(e pythonEntry) (string, string) {
		return e.name, e.version
	}), pkgNames, PythonBasePackageName, pypiLatestVersion)
}

func (Pipx) listEntries() ([]pythonEntry, error) {
	out, err := runProbe("pipx", "list", "--json")
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return nil, nil
		}
		return nil, err
	}
	return parsePipxListJSON(out)
}
