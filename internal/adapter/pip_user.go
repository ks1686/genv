package adapter

import (
	"errors"
	"os/exec"
)

// PipUser is the adapter for pip --user installs.
type PipUser struct{}

func (PipUser) Name() string { return "pip-user" }

func (PipUser) Available() bool {
	_, err := lookPath("python3")
	return err == nil
}

func (PipUser) NormalizeID(id string, managers map[string]string) (string, bool) {
	return normalizeID("pip-user", id, managers)
}

func (PipUser) PlanInstall(pkgName string) []string {
	return []string{"python3", "-m", "pip", "install", "--user", pkgName}
}

func (PipUser) PlanUninstall(pkgName string) []string {
	return []string{"python3", "-m", "pip", "uninstall", "-y", PythonBasePackageName(pkgName)}
}

func (PipUser) PlanUpgrade(pkgName string) []string {
	return []string{"python3", "-m", "pip", "install", "--user", "--upgrade", pkgName}
}

func (PipUser) PlanClean() [][]string {
	return [][]string{{"python3", "-m", "pip", "cache", "purge"}}
}

func (PipUser) Query(pkgName string) (bool, error) {
	name := PythonBasePackageName(pkgName)
	entries, err := PipUser{}.listEntries()
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

func (PipUser) ListInstalled() ([]string, error) {
	entries, err := PipUser{}.listEntries()
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.name)
	}
	return names, nil
}

func (PipUser) QueryVersion(pkgName string) (string, error) {
	name := PythonBasePackageName(pkgName)
	entries, err := PipUser{}.listEntries()
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

func (PipUser) ListInstalledVersions() (map[string]string, error) {
	entries, err := PipUser{}.listEntries()
	if err != nil {
		return nil, err
	}
	versions := make(map[string]string, len(entries))
	for _, entry := range entries {
		versions[entry.name] = entry.version
	}
	return versions, nil
}

func (PipUser) listEntries() ([]pythonEntry, error) {
	out, err := exec.Command("python3", "-m", "pip", "list", "--user", "--format=json").Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return nil, nil
		}
		return nil, err
	}
	return parsePipListJSON(out)
}
