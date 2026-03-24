package adapter

import "strings"

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

func (Flatpak) PlanUninstall(pkgName string) []string {
	return []string{"flatpak", "uninstall", "-y", pkgName}
}

func (Flatpak) PlanUpgrade(pkgName string) []string {
	return []string{"flatpak", "update", "-y", pkgName}
}

func (Flatpak) PlanClean() [][]string {
	return [][]string{{"flatpak", "uninstall", "--unused", "-y"}}
}

func (Flatpak) Query(pkgName string) (bool, error) { return runQuery("flatpak", "info", pkgName) }

// ListInstalled returns application IDs of installed Flatpak apps (not runtimes).
func (Flatpak) ListInstalled() ([]string, error) {
	return runListOutput("flatpak", "list", "--app", "--columns=application")
}

func (Flatpak) QueryVersion(pkgName string) (string, error) {
	// Parse "Version:" from "flatpak info <pkg>" output.
	out, err := runVersionOutput("flatpak", "info", pkgName)
	if err != nil || out == "" {
		return out, err
	}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if after, ok := strings.CutPrefix(line, "Version:"); ok {
			return strings.TrimSpace(after), nil
		}
	}
	return "", nil
}
