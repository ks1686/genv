package adapter

import (
	"errors"
	"os/exec"
)

// Snap is the adapter for the Snap package manager (Ubuntu/Canonical).
type Snap struct{}

func (Snap) Name() string { return "snap" }

func (Snap) Available() bool {
	_, err := lookPath("snap")
	return err == nil
}

func (Snap) NormalizeID(id string, managers map[string]string) (string, bool) {
	if name, ok := managers["snap"]; ok {
		return name, true
	}
	return id, false
}

func (Snap) PlanInstall(pkgName string) []string {
	return []string{"sudo", "snap", "install", pkgName}
}

// Query checks whether pkgName is listed as an installed snap.
func (Snap) Query(pkgName string) (bool, error) {
	err := exec.Command("snap", "list", pkgName).Run()
	if err == nil {
		return true, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return false, nil
	}
	return false, err
}
