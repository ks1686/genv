package adapter

// MacPorts is the adapter for MacPorts (macOS).
type MacPorts struct{}

func (MacPorts) Name() string { return "macports" }

func (MacPorts) Available() bool {
	_, err := lookPath("port")
	return err == nil
}

func (MacPorts) NormalizeID(id string, managers map[string]string) (string, bool) {
	return normalizeID("macports", id, managers)
}

func (MacPorts) PlanInstall(pkgName string) []string {
	return []string{"sudo", "port", "install", pkgName}
}

func (MacPorts) Query(pkgName string) (bool, error) { return runQuery("port", "installed", pkgName) }
