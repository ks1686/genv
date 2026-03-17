package adapter

// Snap is the adapter for the Snap package manager (Ubuntu/Canonical).
type Snap struct{}

func (Snap) Name() string { return "snap" }

func (Snap) Available() bool {
	_, err := lookPath("snap")
	return err == nil
}

func (Snap) NormalizeID(id string, managers map[string]string) (string, bool) {
	return normalizeID("snap", id, managers)
}

func (Snap) PlanInstall(pkgName string) []string {
	return []string{"sudo", "snap", "install", pkgName}
}

func (Snap) Query(pkgName string) (bool, error) { return runQuery("snap", "list", pkgName) }
