package adapter

import (
	"errors"
	"os/exec"
)

// MacPorts is the adapter for MacPorts (macOS).
type MacPorts struct{}

func (MacPorts) Name() string { return "macports" }

func (MacPorts) Available() bool {
	_, err := lookPath("port")
	return err == nil
}

func (MacPorts) NormalizeID(id string, managers map[string]string) (string, bool) {
	if name, ok := managers["macports"]; ok {
		return name, true
	}
	return id, false
}

func (MacPorts) PlanInstall(pkgName string) []string {
	return []string{"sudo", "port", "install", pkgName}
}

// Query checks whether pkgName is installed as an active MacPorts port.
func (MacPorts) Query(pkgName string) (bool, error) {
	err := exec.Command("port", "installed", pkgName).Run()
	if err == nil {
		return true, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return false, nil
	}
	return false, err
}
