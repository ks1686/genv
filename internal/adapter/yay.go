package adapter

import (
	"errors"
	"os/exec"
)

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
	if name, ok := managers["yay"]; ok {
		return name, true
	}
	return id, false
}

func (Yay) PlanInstall(pkgName string) []string {
	return []string{"yay", "-S", "--noconfirm", pkgName}
}

// Query checks whether pkgName is installed in the local package database.
func (Yay) Query(pkgName string) (bool, error) {
	err := exec.Command("yay", "-Qi", pkgName).Run()
	if err == nil {
		return true, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return false, nil
	}
	return false, err
}
