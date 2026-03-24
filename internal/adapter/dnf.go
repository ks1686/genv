package adapter

// Dnf is the adapter for the DNF package manager (Fedora/RHEL).
type Dnf struct{}

func (Dnf) Name() string { return "dnf" }

func (Dnf) Available() bool {
	_, err := lookPath("dnf")
	return err == nil
}

func (Dnf) NormalizeID(id string, managers map[string]string) (string, bool) {
	return normalizeID("dnf", id, managers)
}

func (Dnf) PlanInstall(pkgName string) []string {
	return []string{"sudo", "dnf", "install", "-y", pkgName}
}

func (Dnf) PlanUninstall(pkgName string) []string {
	return []string{"sudo", "dnf", "remove", "-y", pkgName}
}

func (Dnf) PlanUpgrade(pkgName string) []string {
	return []string{"sudo", "dnf", "upgrade", "-y", pkgName}
}

func (Dnf) PlanClean() [][]string {
	return [][]string{
		{"sudo", "dnf", "autoremove", "-y"},
		{"sudo", "dnf", "clean", "all"},
	}
}

func (Dnf) Query(pkgName string) (bool, error) { return runQuery("rpm", "-q", pkgName) }

func (Dnf) ListInstalled() ([]string, error) {
	return runListOutput("rpm", "-qa", "--qf", "%{NAME}\\n")
}

func (Dnf) QueryVersion(pkgName string) (string, error) {
	return runVersionOutput("rpm", "-q", "--qf", "%{VERSION}", pkgName)
}
