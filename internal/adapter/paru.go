package adapter

import (
	"errors"
	"os/exec"
)

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
	if name, ok := managers["paru"]; ok {
		return name, true
	}
	return id, false
}

func (Paru) PlanInstall(pkgName string) []string {
	return []string{"paru", "-S", "--noconfirm", pkgName}
}

// Query checks whether pkgName is installed in the local package database.
func (Paru) Query(pkgName string) (bool, error) {
	err := exec.Command("paru", "-Qi", pkgName).Run()
	if err == nil {
		return true, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return false, nil
	}
	return false, err
}
