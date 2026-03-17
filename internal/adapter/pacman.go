package adapter

import (
	"errors"
	"os/exec"
)

// Pacman is the adapter for the pacman package manager (Arch Linux).
type Pacman struct{}

func (Pacman) Name() string { return "pacman" }

func (Pacman) Available() bool {
	_, err := lookPath("pacman")
	return err == nil
}

func (Pacman) NormalizeID(id string, managers map[string]string) (string, bool) {
	if name, ok := managers["pacman"]; ok {
		return name, true
	}
	return id, false
}

func (Pacman) PlanInstall(pkgName string) []string {
	return []string{"sudo", "pacman", "-S", "--noconfirm", pkgName}
}

// Query checks whether pkgName is installed in the local pacman database.
func (Pacman) Query(pkgName string) (bool, error) {
	err := exec.Command("pacman", "-Qi", pkgName).Run()
	if err == nil {
		return true, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return false, nil
	}
	return false, err
}
