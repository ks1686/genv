package adapter

// Pacman is the adapter for pacman, the Arch Linux official-repository package
// manager. It supports only packages from Arch official repositories; AUR
// packages are handled by the Paru and Yay adapters.
//
// Unlike paru/yay (which self-elevate), pacman has no built-in privilege
// escalation, so mutating commands are prefixed with sudo — the same
// approach used by Snap.
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
	return []string{"sudo", "pacman", "-S", "--needed", "--noconfirm", pkgName}
}

func (Pacman) PlanUninstall(pkgName string) []string {
	return []string{"sudo", "pacman", "-Rcs", "--noconfirm", pkgName}
}

// PlanUpgrade reuses PlanInstall: pacman -S upgrades to the latest version.
func (Pacman) PlanUpgrade(pkgName string) []string {
	return []string{"sudo", "pacman", "-S", "--needed", "--noconfirm", pkgName}
}

func (Pacman) PlanClean() [][]string {
	return [][]string{{"sudo", "pacman", "-Sc", "--noconfirm"}}
}

func (Pacman) Query(pkgName string) (bool, error) { return runQuery("pacman", "-Qi", pkgName) }

// Search returns package names from official pacman repos whose name contains query.
func (Pacman) Search(query string) ([]string, error) {
	lines, err := runListOutput("pacman", "-Ss", query)
	if err != nil || len(lines) == 0 {
		return lines, err
	}
	return parsePacmanSearch(lines, query), nil
}

func (Pacman) ListInstalled() ([]string, error) {
	return runListOutput("pacman", "-Qq")
}

func (Pacman) QueryVersion(pkgName string) (string, error) {
	out, err := runVersionOutput("pacman", "-Q", pkgName)
	if err != nil || out == "" {
		return out, err
	}
	return parseMgrQueryVersion(out), nil
}
