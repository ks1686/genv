// Package resolver detects available package managers and resolves packages
// to concrete install actions based on what is present on the current host.
package resolver

import (
	"fmt"
	"io"
	"os/exec"
	"strings"
	"text/tabwriter"

	"github.com/ks1686/gpm/internal/adapter"
	"github.com/ks1686/gpm/internal/schema"
)

// Detect returns the set of package manager names available on the current host
// by checking each registered adapter's binary in PATH.
func Detect() map[string]bool {
	available := make(map[string]bool)
	for _, a := range adapter.All {
		if a.Available() {
			available[a.Name()] = true
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
		if a := adapter.ByName(pkg.Prefer); a != nil {
			name, _ := a.NormalizeID(pkg.ID, pkg.Managers)
			return Action{Pkg: pkg, Manager: a.Name(), PkgName: name, Cmd: a.PlanInstall(name)}
		}
	}

	// 2. Pick the first available adapter in registry order whose manager name
	//    appears in the package's explicit managers map.
	for _, a := range adapter.All {
		if name, ok := pkg.Managers[a.Name()]; ok && available[a.Name()] {
			return Action{Pkg: pkg, Manager: a.Name(), PkgName: name, Cmd: a.PlanInstall(name)}
		}
	}

	// 3. Fall back to the first available adapter, using the package ID as name.
	for _, a := range adapter.All {
		if available[a.Name()] {
			name, _ := a.NormalizeID(pkg.ID, pkg.Managers)
			return Action{Pkg: pkg, Manager: a.Name(), PkgName: name, Cmd: a.PlanInstall(name)}
		}
	}

	// Unresolved — no compatible manager on this host.
	return Action{Pkg: pkg}
}

// PrintPlan writes a human-readable install plan to w and returns the number
// of resolved and unresolved packages so callers can act on the counts without
// a second pass over the actions slice.
func PrintPlan(actions []Action, w io.Writer) (resolved, unresolved int) {
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
	return
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
