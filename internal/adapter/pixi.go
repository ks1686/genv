package adapter

import (
	"errors"
	"os/exec"
)

// Pixi is the adapter for pixi global tools.
type Pixi struct{}

func (Pixi) Name() string { return "pixi" }

func (Pixi) Available() bool {
	_, err := lookPath("pixi")
	return err == nil
}

func (Pixi) NormalizeID(id string, managers map[string]string) (string, bool) {
	return normalizeID("pixi", id, managers)
}

func (Pixi) PlanInstall(pkgName string) []string {
	return []string{"pixi", "global", "install", pkgName}
}

func (Pixi) PlanUninstall(pkgName string) []string {
	return []string{"pixi", "global", "remove", PythonBasePackageName(pkgName)}
}

func (Pixi) PlanUpgrade(pkgName string) []string {
	return []string{"pixi", "global", "upgrade", pkgName}
}

func (Pixi) PlanClean() [][]string {
	return nil
}

func (Pixi) Query(pkgName string) (bool, error) {
	name := PythonBasePackageName(pkgName)
	entries, err := Pixi{}.listEntries()
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

func (Pixi) ListInstalled() ([]string, error) {
	entries, err := Pixi{}.listEntries()
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.name)
	}
	return names, nil
}

func (Pixi) QueryVersion(pkgName string) (string, error) {
	name := PythonBasePackageName(pkgName)
	entries, err := Pixi{}.listEntries()
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

func (Pixi) ListInstalledVersions() (map[string]string, error) {
	entries, err := Pixi{}.listEntries()
	if err != nil {
		return nil, err
	}
	versions := make(map[string]string, len(entries))
	for _, entry := range entries {
		versions[entry.name] = entry.version
	}
	return versions, nil
}

func (Pixi) listEntries() ([]pythonEntry, error) {
	out, err := runProbe("pixi", "global", "list")
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return nil, nil
		}
		return nil, err
	}
	return parsePixiListText(string(out))
}
