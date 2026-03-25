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

func (Pacman) PlanUninstall(pkgName string) []string {
	return []string{"sudo", "pacman", "-Rns", "--noconfirm", pkgName}
}

// PlanUpgrade reuses PlanInstall: pacman -S upgrades to the latest version.
func (Pacman) PlanUpgrade(pkgName string) []string {
	return []string{"sudo", "pacman", "-S", "--noconfirm", pkgName}
}

func (Pacman) PlanClean() [][]string {
	return [][]string{{"sudo", "pacman", "-Sc", "--noconfirm"}}
}

func (Pacman) Query(pkgName string) (bool, error) { return runQuery("pacman", "-Qi", pkgName) }

// Search returns package names from pacman repos whose name contains query.
func (Pacman) Search(query string) ([]string, error) {
	lines, err := runListOutput("pacman", "-Ss", query)
	if err != nil || len(lines) == 0 {
		return lines, err
	}
	return parsePacmanSearch(lines, query), nil
}

// ListInstalled returns explicitly-installed packages (not pulled-in deps).
func (Pacman) ListInstalled() ([]string, error) {
	return runListOutput("pacman", "-Qqe")
}

func (Pacman) QueryVersion(pkgName string) (string, error) {
	// "pacman -Q pkgname" outputs "pkgname 1.0.0-1"; return only the version part.
	out, err := runVersionOutput("pacman", "-Q", pkgName)
	if err != nil || out == "" {
		return out, err
	}
	return parseMgrQueryVersion(out), nil
}
