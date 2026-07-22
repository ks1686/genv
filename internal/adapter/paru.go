package adapter

import "strings"

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

// PlanUpgrade reuses PlanInstall: paru -S upgrades to the latest version.
func (Paru) PlanUpgrade(pkgName string) []string {
	return []string{"paru", "-S", "--noconfirm", pkgName}
}

// PlanUpgradeBatch upgrades multiple packages in one paru invocation.
func (Paru) PlanUpgradeBatch(pkgNames []string) []string {
	args := []string{"paru", "-S", "--noconfirm"}
	return append(args, pkgNames...)
}

func (Paru) PlanClean() [][]string {
	return [][]string{{"paru", "-Sc", "--noconfirm"}}
}

func (Paru) Query(pkgName string) (bool, error) { return runQuery("paru", "-Qi", pkgName) }

// Search returns package names from pacman/AUR repos whose name contains query.
func (Paru) Search(query string) ([]string, error) {
	lines, err := runListOutput("paru", "-Ss", query)
	if err != nil || len(lines) == 0 {
		return lines, err
	}
	return parsePacmanSearch(lines, query), nil
}

// ListInstalled delegates to pacman since paru manages the same pacman DB.
func (Paru) ListInstalled() ([]string, error) {
	return runListOutput("pacman", "-Qqe")
}

// ListInstalledVersions returns installed versions from the pacman database,
// which paru/yay share. This satisfies VersionLister for batch upgrade version
// refresh.
func (Paru) ListInstalledVersions() (map[string]string, error) {
	lines, err := runListOutput("pacman", "-Q")
	if err != nil {
		return nil, err
	}
	versions := make(map[string]string, len(lines))
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) >= 2 {
			versions[fields[0]] = fields[1]
		}
	}
	return versions, nil
}

func (Paru) QueryVersion(pkgName string) (string, error) {
	out, err := runVersionOutput("paru", "-Q", pkgName)
	if err != nil || out == "" {
		return out, err
	}
	return parseMgrQueryVersion(out), nil
}

// ListOutdated reports installed packages with a newer version available via
// paru, keyed by package name -> target version, intersected with pkgNames.
func (Paru) ListOutdated(pkgNames []string) (map[string]string, error) {
	return listPacmanQuOutdated("paru", pkgNames)
}
