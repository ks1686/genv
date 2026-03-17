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

func (Dnf) Query(pkgName string) (bool, error) { return runQuery("rpm", "-q", pkgName) }
