package adapter

import (
	"errors"
	"os/exec"
)

// Apt is the adapter for the APT package manager (Debian/Ubuntu).
type Apt struct{}

func (Apt) Name() string { return "apt" }

func (Apt) Available() bool {
	_, err := lookPath("apt-get")
	return err == nil
}

func (Apt) NormalizeID(id string, managers map[string]string) (string, bool) {
	if name, ok := managers["apt"]; ok {
		return name, true
	}
	return id, false
}

func (Apt) PlanInstall(pkgName string) []string {
	return []string{"sudo", "apt-get", "install", "-y", pkgName}
}

// Query checks whether pkgName is recorded as installed in the dpkg database.
func (Apt) Query(pkgName string) (bool, error) {
	err := exec.Command("dpkg", "-s", pkgName).Run()
	if err == nil {
		return true, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return false, nil
	}
	return false, err
}
