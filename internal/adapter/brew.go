package adapter

import (
	"errors"
	"os/exec"
)

// Brew is the adapter for Homebrew (macOS and Linux).
type Brew struct{}

func (Brew) Name() string { return "brew" }

func (Brew) Available() bool {
	_, err := lookPath("brew")
	return err == nil
}

func (Brew) NormalizeID(id string, managers map[string]string) (string, bool) {
	if name, ok := managers["brew"]; ok {
		return name, true
	}
	return id, false
}

func (Brew) PlanInstall(pkgName string) []string {
	return []string{"brew", "install", pkgName}
}

// Query checks whether pkgName is installed as a Homebrew formula.
func (Brew) Query(pkgName string) (bool, error) {
	err := exec.Command("brew", "list", "--formula", pkgName).Run()
	if err == nil {
		return true, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return false, nil
	}
	return false, err
}

// Linuxbrew is the adapter for Homebrew on Linux (distinct manager ID so
// gpm.json can target it explicitly, but uses the same brew binary).
type Linuxbrew struct{}

func (Linuxbrew) Name() string { return "linuxbrew" }

func (Linuxbrew) Available() bool {
	_, err := lookPath("brew")
	return err == nil
}

func (Linuxbrew) NormalizeID(id string, managers map[string]string) (string, bool) {
	if name, ok := managers["linuxbrew"]; ok {
		return name, true
	}
	return id, false
}

func (Linuxbrew) PlanInstall(pkgName string) []string {
	return []string{"brew", "install", pkgName}
}

// Query checks whether pkgName is installed via the brew binary.
func (Linuxbrew) Query(pkgName string) (bool, error) {
	err := exec.Command("brew", "list", "--formula", pkgName).Run()
	if err == nil {
		return true, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return false, nil
	}
	return false, err
}
