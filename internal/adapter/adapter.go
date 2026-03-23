// Package adapter defines the Adapter interface that every package manager
// must implement, along with the ordered registry of all known adapters.
package adapter

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

	// PlanUninstall returns the command argv to uninstall pkgName via this manager.
	PlanUninstall(pkgName string) []string

	// PlanClean returns zero or more commands to purge cached data for this
	// manager. Each inner slice is an independent command (argv). Returns nil
	// when the manager has no meaningful cache-clean operation.
	PlanClean() [][]string

	// Query reports whether pkgName is already installed on this host.
	// Returns false, nil when the package is absent (not an error condition).
	Query(pkgName string) (bool, error)

	// ListInstalled returns the concrete package names of all packages currently
	// installed via this manager. Returns nil, nil when the manager is unavailable
	// or no packages are installed. Names are manager-specific identifiers, not gpm IDs.
	ListInstalled() ([]string, error)

	// QueryVersion returns the installed version string for pkgName.
	// Returns "", nil when the package is not installed or the version cannot be
	// determined. Version strings are manager-specific and not normalised.
	QueryVersion(pkgName string) (string, error)
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
// On WSL2 hosts it uses wslSafeLookPath to prevent Windows-mounted binaries
// from shadowing Linux-native package managers.
var lookPath = wslSafeLookPath

// wslSafeLookPath wraps exec.LookPath. On WSL2 it sanitizes PATH first to
// remove Windows drive mount entries (/mnt/c/, /mnt/d/, …) so that Windows
// binaries cannot shadow Linux-native package managers.
func wslSafeLookPath(file string) (string, error) {
	if isWSL() {
		clean := sanitizePathForWSL(os.Getenv("PATH"))
		for _, dir := range filepath.SplitList(clean) {
			candidate := filepath.Join(dir, file)
			if info, err := os.Stat(candidate); err == nil && !info.IsDir() && info.Mode()&0111 != 0 {
				return candidate, nil
			}
		}
		return "", &os.PathError{Op: "lookpath", Path: file, Err: os.ErrNotExist}
	}
	return exec.LookPath(file)
}

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

// runListOutput runs cmd and returns stdout split into trimmed, non-empty lines.
// A non-zero exit code is treated as "no packages" (nil, nil), not an error.
func runListOutput(cmd string, args ...string) ([]string, error) {
	out, err := exec.Command(cmd, args...).Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return nil, nil
		}
		return nil, err
	}
	var result []string
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			result = append(result, line)
		}
	}
	return result, nil
}

// runVersionOutput runs cmd and returns trimmed stdout as the version string.
// A non-zero exit code is treated as "not installed" ("", nil), not an error.
func runVersionOutput(cmd string, args ...string) (string, error) {
	out, err := exec.Command(cmd, args...).Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return "", nil
		}
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// parseMgrQueryVersion extracts the version from "pkgname version" output,
// as produced by pacman-style query commands (pacman -Q, paru -Q, yay -Q).
func parseMgrQueryVersion(out string) string {
	if parts := strings.SplitN(out, " ", 2); len(parts) == 2 {
		return parts[1]
	}
	return ""
}
