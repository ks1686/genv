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

func (Yay) PlanUninstall(pkgName string) []string {
	return []string{"yay", "-Rns", "--noconfirm", pkgName}
}

// PlanUpgrade reuses PlanInstall: yay -S upgrades to the latest version.
func (Yay) PlanUpgrade(pkgName string) []string {
	return []string{"yay", "-S", "--noconfirm", pkgName}
}

func (Yay) PlanClean() [][]string {
	return [][]string{{"yay", "-Sc", "--noconfirm"}}
}

func (Yay) Query(pkgName string) (bool, error) { return runQuery("yay", "-Qi", pkgName) }

// ListInstalled delegates to pacman since yay manages the same pacman DB.
func (Yay) ListInstalled() ([]string, error) {
	return runListOutput("pacman", "-Qqe")
}

func (Yay) QueryVersion(pkgName string) (string, error) {
	out, err := runVersionOutput("yay", "-Q", pkgName)
	if err != nil || out == "" {
		return out, err
	}
	return parseMgrQueryVersion(out), nil
}
