package adapter

// Apt is the adapter for the APT package manager (Debian/Ubuntu).
type Apt struct{}

func (Apt) Name() string { return "apt" }

func (Apt) Available() bool {
	_, err := lookPath("apt-get")
	return err == nil
}

func (Apt) NormalizeID(id string, managers map[string]string) (string, bool) {
	return normalizeID("apt", id, managers)
}

func (Apt) PlanInstall(pkgName string) []string {
	return []string{"sudo", "apt-get", "install", "-y", pkgName}
}

func (Apt) Query(pkgName string) (bool, error) { return runQuery("dpkg", "-s", pkgName) }
