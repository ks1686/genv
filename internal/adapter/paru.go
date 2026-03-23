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

func (Paru) PlanUninstall(pkgName string) []string {
	return []string{"paru", "-Rns", "--noconfirm", pkgName}
}

func (Paru) PlanClean() [][]string {
	return [][]string{{"paru", "-Sc", "--noconfirm"}}
}

func (Paru) Query(pkgName string) (bool, error) { return runQuery("paru", "-Qi", pkgName) }

// ListInstalled delegates to pacman since paru manages the same pacman DB.
func (Paru) ListInstalled() ([]string, error) {
	return runListOutput("pacman", "-Qqe")
}

func (Paru) QueryVersion(pkgName string) (string, error) {
	out, err := runVersionOutput("paru", "-Q", pkgName)
	if err != nil || out == "" {
		return out, err
	}
	return parseMgrQueryVersion(out), nil
}
