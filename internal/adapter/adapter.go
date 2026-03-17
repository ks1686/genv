// Package adapter defines the Adapter interface that every package manager
// must implement, along with the ordered registry of all known adapters.
package adapter

import (
	"errors"
	"os/exec"
)

// Adapter is the capability contract every package manager must satisfy.
// Each method maps to one of the four resolver operations: detect, query,
// plan install, and normalize package IDs.
type Adapter interface {
	// Name returns the canonical manager identifier used in gpm.json
	// (e.g. "apt", "brew", "flatpak").
	Name() string

	// Available reports whether this manager's binary exists in PATH.
	Available() bool

	// NormalizeID returns the concrete package name for this manager.
	// managers is the optional per-manager name overrides from the gpm.json
	// "managers" field. Returns the resolved name and true if an explicit
	// mapping was found; returns id and false when falling back to the ID.
	NormalizeID(id string, managers map[string]string) (name string, explicit bool)

	// PlanInstall returns the command argv to install pkgName via this manager.
	PlanInstall(pkgName string) []string

	// Query reports whether pkgName is already installed on this host.
	// Returns false, nil when the package is absent (not an error condition).
	Query(pkgName string) (bool, error)
}

// All is the ordered registry of every known adapter.
// The slice order determines fallback priority: when no preference is
// specified in gpm.json the first available adapter wins.
var All = []Adapter{
	Brew{},
	MacPorts{},
	Apt{},
	Dnf{},
	Pacman{},
	Paru{},
	Yay{},
	Flatpak{},
	Snap{},
	Linuxbrew{},
}

// ByName returns the adapter whose Name() matches name, or nil if none match.
func ByName(name string) Adapter {
	for _, a := range All {
		if a.Name() == name {
			return a
		}
	}
	return nil
}

// lookPath is the exec.LookPath implementation used by adapters.
// Replaced in tests to avoid PATH dependence.
var lookPath = exec.LookPath

// normalizeID is the standard NormalizeID implementation shared by all adapters.
// key must equal the adapter's Name() string.
func normalizeID(key, id string, managers map[string]string) (string, bool) {
	if name, ok := managers[key]; ok {
		return name, true
	}
	return id, false
}

// runQuery executes cmd with args and interprets the exit status as an
// installed/absent signal. A non-zero exit code means "not installed"
// (false, nil). Only an OS-level execution failure is returned as an error.
func runQuery(cmd string, args ...string) (bool, error) {
	err := exec.Command(cmd, args...).Run()
	if err == nil {
		return true, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return false, nil
	}
	return false, err
}
