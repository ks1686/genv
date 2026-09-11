//go:build unix

package resolver

import (
	"os"
	"os/exec"
)

func processElevated() bool {
	return os.Geteuid() == 0
}

func sudoNoninteractiveOK() bool {
	sudo, err := exec.LookPath("sudo")
	if err != nil {
		return false
	}
	return exec.Command(sudo, "-n", "true").Run() == nil
}
