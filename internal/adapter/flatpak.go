package adapter

// Flatpak is the adapter for the Flatpak application sandbox runtime.
type Flatpak struct{}

func (Flatpak) Name() string { return "flatpak" }

func (Flatpak) Available() bool {
	_, err := lookPath("flatpak")
	return err == nil
}

func (Flatpak) NormalizeID(id string, managers map[string]string) (string, bool) {
	return normalizeID("flatpak", id, managers)
}

func (Flatpak) PlanInstall(pkgName string) []string {
	return []string{"flatpak", "install", "-y", "--noninteractive", pkgName}
}

func (Flatpak) Query(pkgName string) (bool, error) { return runQuery("flatpak", "info", pkgName) }
