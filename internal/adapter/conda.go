package adapter

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// parseCondaEnvPkg splits an env-qualified spec like "env:myenv:mypkg" or "myenv:mypkg"
// into the environment name and the package name.
func parseCondaEnvPkg(spec string) (env string, pkg string, err error) {
	spec = strings.TrimPrefix(spec, "env:")
	parts := strings.SplitN(spec, ":", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("conda/mamba requires env-qualified format <env>:<pkg>, got %q", spec)
	}
	return parts[0], parts[1], nil
}

// Conda is the adapter for conda environments.
type Conda struct{}

func (Conda) Name() string { return "conda" }

func (Conda) Available() bool {
	_, err := lookPath("conda")
	return err == nil
}

func (Conda) NormalizeID(id string, managers map[string]string) (string, bool) {
	return normalizeID("conda", id, managers)
}

func (Conda) PlanInstall(pkgName string) []string {
	env, pkg, err := parseCondaEnvPkg(pkgName)
	if err != nil {
		return condaInvalidCommand(pkgName)
	}
	return []string{"conda", "install", "-y", "-n", env, pkg}
}

func (Conda) PlanUninstall(pkgName string) []string {
	env, pkg, err := parseCondaEnvPkg(pkgName)
	if err != nil {
		return condaInvalidCommand(pkgName)
	}
	return []string{"conda", "remove", "-y", "-n", env, PythonBasePackageName(pkg)}
}

func (Conda) PlanUpgrade(pkgName string) []string {
	env, pkg, err := parseCondaEnvPkg(pkgName)
	if err != nil {
		return condaInvalidCommand(pkgName)
	}
	return []string{"conda", "update", "-y", "-n", env, PythonBasePackageName(pkg)}
}

func condaInvalidCommand(pkgName string) []string {
	return []string{"sh", "-c", "printf '%s\n' 'genv: conda/mamba requires env-qualified format <env>:<pkg>' >&2; exit 1", "genv-conda-invalid", pkgName}
}

func (Conda) PlanClean() [][]string {
	return [][]string{{"conda", "clean", "-y", "--all"}}
}

func (Conda) Query(pkgName string) (bool, error) {
	env, pkg, err := parseCondaEnvPkg(pkgName)
	if err != nil {
		return false, err
	}
	name := PythonBasePackageName(pkg)
	entries, err := listCondaVersions("conda", env)
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

func (Conda) ListInstalled() ([]string, error) {
	return nil, nil
}

func (Conda) QueryVersion(pkgName string) (string, error) {
	env, pkg, err := parseCondaEnvPkg(pkgName)
	if err != nil {
		return "", err
	}
	name := PythonBasePackageName(pkg)
	entries, err := listCondaVersions("conda", env)
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

func listCondaVersions(bin, env string) ([]pythonEntry, error) {
	out, err := exec.Command(bin, "list", "-n", env, "--json").Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return nil, nil
		}
		return nil, err
	}
	return parseCondaListJSON(out)
}
