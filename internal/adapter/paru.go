package adapter

// Paru is the adapter for paru, an AUR helper for Arch Linux.
// paru wraps pacman and handles AUR packages; it manages privilege escalation
// internally so no sudo prefix is needed.
type Paru struct{}

func (Paru) Name() string { return "paru" }

func (Paru) Available() bool {
	_, err := lookPath("paru")
	return err == nil
}

func (Paru) NormalizeID(id string, managers map[string]string) (string, bool) {
	return normalizeID("paru", id, managers)
}

func (Paru) PlanInstall(pkgName string) []string {
	return []string{"paru", "-S", "--noconfirm", pkgName}
}

func (Paru) Query(pkgName string) (bool, error) { return runQuery("paru", "-Qi", pkgName) }
