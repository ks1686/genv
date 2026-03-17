// Package resolver detects available package managers and resolves packages
// to concrete install actions based on what is present on the current host.
package resolver

import (
	"fmt"
	"io"
	"os/exec"
	"strings"
	"text/tabwriter"

	"github.com/ks1686/gpm/internal/schema"
)

// managerBin maps each known manager name to its executable binary.
var managerBin = map[string]string{
	"apt":       "apt-get",
	"dnf":       "dnf",
	"pacman":    "pacman",
	"flatpak":   "flatpak",
	"snap":      "snap",
	"brew":      "brew",
	"linuxbrew": "brew",
}

// fallbackOrder is the preference order when a package specifies no manager hint.
var fallbackOrder = []string{"brew", "apt", "dnf", "pacman", "flatpak", "snap", "linuxbrew"}

// installArgs returns the install command slice for the given manager and package name.
func installArgs(mgr, pkgName string) []string {
	switch mgr {
	case "apt":
		return []string{"sudo", "apt-get", "install", "-y", pkgName}
	case "dnf":
		return []string{"sudo", "dnf", "install", "-y", pkgName}
	case "pacman":
		return []string{"sudo", "pacman", "-S", "--noconfirm", pkgName}
	case "flatpak":
		return []string{"flatpak", "install", "-y", "--noninteractive", pkgName}
	case "snap":
		return []string{"sudo", "snap", "install", pkgName}
	case "brew", "linuxbrew":
		return []string{"brew", "install", pkgName}
	default:
		return nil
	}
}

// Detect returns the set of package manager names available on the current host
// by checking for each manager's binary in PATH.
func Detect() map[string]bool {
	available := make(map[string]bool)
	found := make(map[string]bool) // binary → whether LookPath succeeded
	for mgr, bin := range managerBin {
		if _, checked := found[bin]; !checked {
			_, err := exec.LookPath(bin)
			found[bin] = err == nil
		}
		if found[bin] {
			available[mgr] = true
		}
	}
	return available
}

// Action is the resolved install action for a single package.
// Manager is empty when no available manager could be matched.
type Action struct {
	Pkg     schema.Package
	Manager string   // empty if unresolved
	PkgName string   // concrete name to pass to the manager
	Cmd     []string // nil if unresolved
}

// Resolved reports whether a manager was found for this package.
func (a Action) Resolved() bool { return a.Manager != "" }

// Plan resolves every package in f into an Action, using the provided set of
// available manager names. Call Detect() to build the available map.
func Plan(f *schema.GpmFile, available map[string]bool) []Action {
	actions := make([]Action, 0, len(f.Packages))
	for _, pkg := range f.Packages {
		actions = append(actions, resolve(pkg, available))
	}
	return actions
}

func resolve(pkg schema.Package, available map[string]bool) Action {
	// 1. Honour the prefer hint if that manager is available.
	if pkg.Prefer != "" && available[pkg.Prefer] {
		name := concretePackageName(pkg, pkg.Prefer)
		return Action{Pkg: pkg, Manager: pkg.Prefer, PkgName: name, Cmd: installArgs(pkg.Prefer, name)}
	}

	// 2. Pick the first available manager from the explicit managers map,
	//    preserving the canonical fallback priority order.
	for _, mgr := range fallbackOrder {
		if name, ok := pkg.Managers[mgr]; ok && available[mgr] {
			return Action{Pkg: pkg, Manager: mgr, PkgName: name, Cmd: installArgs(mgr, name)}
		}
	}

	// 3. Fall back to any available manager using the package ID as the name.
	for _, mgr := range fallbackOrder {
		if available[mgr] {
			name := pkg.ID
			return Action{Pkg: pkg, Manager: mgr, PkgName: name, Cmd: installArgs(mgr, name)}
		}
	}

	// Unresolved — no compatible manager on this host.
	return Action{Pkg: pkg}
}

// concretePackageName returns the manager-specific package name, falling back
// to the package ID when no explicit mapping exists.
func concretePackageName(pkg schema.Package, mgr string) string {
	if n, ok := pkg.Managers[mgr]; ok {
		return n
	}
	return pkg.ID
}

// PrintPlan writes a human-readable install plan to w.
func PrintPlan(actions []Action, w io.Writer) {
	resolved, unresolved := 0, 0
	for _, a := range actions {
		if a.Resolved() {
			resolved++
		} else {
			unresolved++
		}
	}

	total := len(actions)
	fmt.Fprintf(w, "Install plan — %d package", total)
	if total != 1 {
		fmt.Fprint(w, "s")
	}
	if unresolved > 0 {
		fmt.Fprintf(w, " (%d unresolved)", unresolved)
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w)

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	for _, a := range actions {
		if a.Resolved() {
			fmt.Fprintf(tw, "  %s\tvia %s\t%s\n", a.Pkg.ID, a.Manager, strings.Join(a.Cmd, " "))
		} else {
			fmt.Fprintf(tw, "  %s\tunresolved\t(no manager available)\n", a.Pkg.ID)
		}
	}
	tw.Flush()
	fmt.Fprintln(w)

	if unresolved > 0 {
		fmt.Fprintf(w, "%d package(s) could not be resolved: no compatible manager found on this host.\n", unresolved)
		fmt.Fprintln(w, "Hint: install a supported package manager or add a 'managers' entry in gpm.json.")
		fmt.Fprintln(w, "Use --strict to treat unresolved packages as a hard error.")
	}
}

// Execute runs each resolved install action sequentially, writing subprocess
// output to stdout/stderr. stdin is forwarded to child processes so that
// interactive password prompts (e.g. sudo) work correctly.
// Unresolved packages are silently skipped.
// Returns one error per failed install; a non-empty slice means partial failure.
func Execute(actions []Action, stdin io.Reader, stdout, stderr io.Writer) []error {
	var errs []error
	for _, a := range actions {
		if !a.Resolved() {
			continue
		}
		fmt.Fprintf(stdout, "\n==> %s\n", strings.Join(a.Cmd, " "))
		cmd := exec.Command(a.Cmd[0], a.Cmd[1:]...)
		cmd.Stdin = stdin
		cmd.Stdout = stdout
		cmd.Stderr = stderr
		if err := cmd.Run(); err != nil {
			errs = append(errs, fmt.Errorf("package %q (via %s): %w", a.Pkg.ID, a.Manager, err))
		}
	}
	return errs
}
