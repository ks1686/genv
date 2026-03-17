package adapter

// Yay is the adapter for yay (Yet Another Yogurt), an AUR helper for Arch Linux.
// yay wraps pacman and handles AUR packages; it manages privilege escalation
// internally so no sudo prefix is needed.
type Yay struct{}

func (Yay) Name() string { return "yay" }

func (Yay) Available() bool {
	_, err := lookPath("yay")
	return err == nil
}

func (Yay) NormalizeID(id string, managers map[string]string) (string, bool) {
	return normalizeID("yay", id, managers)
}

func (Yay) PlanInstall(pkgName string) []string {
	return []string{"yay", "-S", "--noconfirm", pkgName}
}

func (Yay) Query(pkgName string) (bool, error) { return runQuery("yay", "-Qi", pkgName) }
