package adapter

// Mamba is the adapter for mamba environments.
type Mamba struct{}

func (Mamba) Name() string { return "mamba" }

func (Mamba) Available() bool {
	_, err := lookPath("mamba")
	return err == nil
}

func (Mamba) NormalizeID(id string, managers map[string]string) (string, bool) {
	return normalizeID("mamba", id, managers)
}

func (Mamba) PlanInstall(pkgName string) []string {
	env, pkg, err := parseCondaEnvPkg(pkgName)
	if err != nil {
		return condaInvalidCommand(pkgName)
	}
	return []string{"mamba", "install", "-y", "-n", env, pkg}
}

func (Mamba) PlanUninstall(pkgName string) []string {
	env, pkg, err := parseCondaEnvPkg(pkgName)
	if err != nil {
		return condaInvalidCommand(pkgName)
	}
	return []string{"mamba", "remove", "-y", "-n", env, PythonBasePackageName(pkg)}
}

func (Mamba) PlanUpgrade(pkgName string) []string {
	env, pkg, err := parseCondaEnvPkg(pkgName)
	if err != nil {
		return condaInvalidCommand(pkgName)
	}
	return []string{"mamba", "update", "-y", "-n", env, PythonBasePackageName(pkg)}
}

func (Mamba) PlanClean() [][]string {
	return [][]string{{"mamba", "clean", "-y", "--all"}}
}

func (Mamba) Query(pkgName string) (bool, error) {
	env, pkg, err := parseCondaEnvPkg(pkgName)
	if err != nil {
		return false, err
	}
	name := PythonBasePackageName(pkg)
	entries, err := listCondaVersions("mamba", env)
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

func (Mamba) ListInstalled() ([]string, error) {
	return nil, nil
}

func (Mamba) QueryVersion(pkgName string) (string, error) {
	env, pkg, err := parseCondaEnvPkg(pkgName)
	if err != nil {
		return "", err
	}
	name := PythonBasePackageName(pkg)
	entries, err := listCondaVersions("mamba", env)
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
