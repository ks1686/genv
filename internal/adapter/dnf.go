package adapter

import (
	"errors"
	"os/exec"
)

// Dnf is the adapter for the DNF package manager (Fedora/RHEL).
type Dnf struct{}

func (Dnf) Name() string { return "dnf" }

func (Dnf) Available() bool {
	_, err := lookPath("dnf")
	return err == nil
}

func (Dnf) NormalizeID(id string, managers map[string]string) (string, bool) {
	if name, ok := managers["dnf"]; ok {
		return name, true
	}
	return id, false
}

func (Dnf) PlanInstall(pkgName string) []string {
	return []string{"sudo", "dnf", "install", "-y", pkgName}
}

// Query checks whether pkgName is installed via rpm.
func (Dnf) Query(pkgName string) (bool, error) {
	err := exec.Command("rpm", "-q", pkgName).Run()
	if err == nil {
		return true, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return false, nil
	}
	return false, err
}
