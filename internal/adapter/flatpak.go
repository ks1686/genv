package adapter

import (
	"errors"
	"os/exec"
)

// Flatpak is the adapter for the Flatpak application sandbox runtime.
type Flatpak struct{}

func (Flatpak) Name() string { return "flatpak" }

func (Flatpak) Available() bool {
	_, err := lookPath("flatpak")
	return err == nil
}

func (Flatpak) NormalizeID(id string, managers map[string]string) (string, bool) {
	if name, ok := managers["flatpak"]; ok {
		return name, true
	}
	return id, false
}

func (Flatpak) PlanInstall(pkgName string) []string {
	return []string{"flatpak", "install", "-y", "--noninteractive", pkgName}
}

// Query checks whether the Flatpak app/runtime ref pkgName is installed.
func (Flatpak) Query(pkgName string) (bool, error) {
	err := exec.Command("flatpak", "info", pkgName).Run()
	if err == nil {
		return true, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return false, nil
	}
	return false, err
}
