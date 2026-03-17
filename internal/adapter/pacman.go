package adapter

// Pacman is the adapter for the pacman package manager (Arch Linux).
type Pacman struct{}

func (Pacman) Name() string { return "pacman" }

func (Pacman) Available() bool {
	_, err := lookPath("pacman")
	return err == nil
}

func (Pacman) NormalizeID(id string, managers map[string]string) (string, bool) {
	return normalizeID("pacman", id, managers)
}

func (Pacman) PlanInstall(pkgName string) []string {
	return []string{"sudo", "pacman", "-S", "--noconfirm", pkgName}
}

func (Pacman) Query(pkgName string) (bool, error) { return runQuery("pacman", "-Qi", pkgName) }
