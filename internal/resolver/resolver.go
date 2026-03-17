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
	"github.com/ks1686/gpm/internal/gpmfile"
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

// Action is the resolved install/uninstall action for a single package.
// Manager is empty when no available manager could be matched.
type Action struct {
	Pkg          schema.Package
	Manager      string   // empty if unresolved
	PkgName      string   // concrete name to pass to the manager
	Cmd          []string // install command; nil if unresolved
	UninstallCmd []string // uninstall command; nil if unresolved
}

// Resolved reports whether a manager was found for this package.
func (a Action) Resolved() bool { return a.Manager != "" }

// ResolveOne resolves a single package into an Action using the provided set of
// available manager names. Used by addCmd to install one package immediately.
func ResolveOne(pkg schema.Package, available map[string]bool) Action {
	return resolve(pkg, available)
}

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
	// ByName is guaranteed non-nil here: available is built from adapter.All
	// in Detect(), so any name present in available has a registered adapter.
	if pkg.Prefer != "" && available[pkg.Prefer] {
		if a := adapter.ByName(pkg.Prefer); a != nil {
			name, _ := a.NormalizeID(pkg.ID, pkg.Managers)
			return Action{Pkg: pkg, Manager: a.Name(), PkgName: name, Cmd: a.PlanInstall(name), UninstallCmd: a.PlanUninstall(name)}
		}
	}

	// 2. Pick the first available adapter in registry order whose manager name
	//    appears in the package's explicit managers map.
	for _, a := range adapter.All {
		if _, ok := pkg.Managers[a.Name()]; ok && available[a.Name()] {
			name, _ := a.NormalizeID(pkg.ID, pkg.Managers)
			return Action{Pkg: pkg, Manager: a.Name(), PkgName: name, Cmd: a.PlanInstall(name), UninstallCmd: a.PlanUninstall(name)}
		}
	}

	// 3. Fall back to the first available adapter, using the package ID as name.
	for _, a := range adapter.All {
		if available[a.Name()] {
			name, _ := a.NormalizeID(pkg.ID, pkg.Managers)
			return Action{Pkg: pkg, Manager: a.Name(), PkgName: name, Cmd: a.PlanInstall(name), UninstallCmd: a.PlanUninstall(name)}
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

// PrintUninstallPlan writes a human-readable uninstall plan to w.
// It mirrors PrintPlan but shows uninstall commands instead of install commands.
func PrintUninstallPlan(actions []Action, w io.Writer) (resolved, unresolved int) {
	for _, a := range actions {
		if a.Resolved() {
			resolved++
		} else {
			unresolved++
		}
	}

	total := len(actions)
	fmt.Fprintf(w, "Uninstall plan — %d package", total)
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
			fmt.Fprintf(tw, "  %s\tvia %s\t%s\n", a.Pkg.ID, a.Manager, strings.Join(a.UninstallCmd, " "))
		} else {
			fmt.Fprintf(tw, "  %s\tunresolved\t(no manager available)\n", a.Pkg.ID)
		}
	}
	tw.Flush()
	fmt.Fprintln(w)
	return
}

// ExecuteUninstall runs each resolved uninstall action sequentially, then
// runs cache-clean commands for every manager that was used (deduplicated).
// stdin is forwarded to child processes for sudo prompts.
// Returns one error per failed operation; a non-empty slice means partial failure.
func ExecuteUninstall(actions []Action, stdin io.Reader, stdout, stderr io.Writer) []error {
	var errs []error
	seenManagers := make(map[string]bool)

	for _, a := range actions {
		if !a.Resolved() {
			continue
		}
		fmt.Fprintf(stdout, "\n==> %s\n", strings.Join(a.UninstallCmd, " "))
		cmd := exec.Command(a.UninstallCmd[0], a.UninstallCmd[1:]...)
		cmd.Stdin = stdin
		cmd.Stdout = stdout
		cmd.Stderr = stderr
		if err := cmd.Run(); err != nil {
			errs = append(errs, fmt.Errorf("package %q (via %s): %w", a.Pkg.ID, a.Manager, err))
		}
		seenManagers[a.Manager] = true
	}

	// Run cache clean once per manager that was involved.
	for managerName := range seenManagers {
		a := adapter.ByName(managerName)
		if a == nil {
			continue
		}
		for _, cleanCmd := range a.PlanClean() {
			fmt.Fprintf(stdout, "\n==> %s\n", strings.Join(cleanCmd, " "))
			cmd := exec.Command(cleanCmd[0], cleanCmd[1:]...)
			cmd.Stdin = stdin
			cmd.Stdout = stdout
			cmd.Stderr = stderr
			if err := cmd.Run(); err != nil {
				errs = append(errs, fmt.Errorf("cache clean (via %s): %w", managerName, err))
			}
		}
	}

	return errs
}

// ---- Reconcile (gpm apply) --------------------------------------------------

// ReconcileResult holds the delta between the desired state (gpm.json) and the
// previously applied state (gpm.lock.json). ToInstall are packages added to the
// spec since the last apply; ToRemove are packages that were removed from it.
type ReconcileResult struct {
	ToInstall []Action
	ToRemove  []Action // UninstallCmd populated; Pkg.ID identifies the package
	Unchanged []gpmfile.LockedPackage
}

// Reconcile computes the delta between the desired packages (from gpm.json)
// and the previously applied state (from gpm.lock.json).
//
//   - ToInstall: in desired but not in lock → resolve via available managers.
//   - ToRemove:  in lock but not in desired → uninstall using the manager
//     recorded in the lock (not re-resolved, preserving the original manager).
//   - Unchanged: in both desired and lock → nothing to do.
func Reconcile(desired []schema.Package, managed []gpmfile.LockedPackage, available map[string]bool) ReconcileResult {
	managedByID := make(map[string]gpmfile.LockedPackage, len(managed))
	for _, lp := range managed {
		managedByID[lp.ID] = lp
	}
	desiredByID := make(map[string]bool, len(desired))
	for _, pkg := range desired {
		desiredByID[pkg.ID] = true
	}

	var toInstall []Action
	for _, pkg := range desired {
		if _, alreadyManaged := managedByID[pkg.ID]; !alreadyManaged {
			toInstall = append(toInstall, resolve(pkg, available))
		}
	}

	var toRemove []Action
	var unchanged []gpmfile.LockedPackage
	for _, lp := range managed {
		if desiredByID[lp.ID] {
			unchanged = append(unchanged, lp)
			continue
		}
		a := adapter.ByName(lp.Manager)
		if a == nil {
			continue // adapter no longer registered; skip silently
		}
		toRemove = append(toRemove, Action{
			Pkg:          schema.Package{ID: lp.ID},
			Manager:      lp.Manager,
			PkgName:      lp.PkgName,
			UninstallCmd: a.PlanUninstall(lp.PkgName),
		})
	}

	return ReconcileResult{ToInstall: toInstall, ToRemove: toRemove, Unchanged: unchanged}
}

// PrintReconcilePlan writes a human-readable apply plan to w. Each line is
// prefixed with '+' (install), '-' (remove), or ' ' (unchanged). Returns the
// counts of packages to install and to remove so the caller can decide whether
// there is actually any work to do.
func PrintReconcilePlan(result ReconcileResult, w io.Writer) (toInstall, toRemove int) {
	toInstall = len(result.ToInstall)
	toRemove = len(result.ToRemove)
	unchanged := len(result.Unchanged)
	total := toInstall + toRemove + unchanged

	fmt.Fprintf(w, "Apply plan — %d package", total)
	if total != 1 {
		fmt.Fprint(w, "s")
	}
	var parts []string
	if toInstall > 0 {
		parts = append(parts, fmt.Sprintf("%d to install", toInstall))
	}
	if toRemove > 0 {
		parts = append(parts, fmt.Sprintf("%d to remove", toRemove))
	}
	if unchanged > 0 {
		parts = append(parts, fmt.Sprintf("%d up to date", unchanged))
	}
	if len(parts) > 0 {
		fmt.Fprintf(w, " (%s)", strings.Join(parts, ", "))
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w)

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	for _, a := range result.ToInstall {
		if a.Resolved() {
			fmt.Fprintf(tw, "  + %s\tvia %s\t%s\n", a.Pkg.ID, a.Manager, strings.Join(a.Cmd, " "))
		} else {
			fmt.Fprintf(tw, "  + %s\tunresolved\t(no manager available)\n", a.Pkg.ID)
		}
	}
	for _, a := range result.ToRemove {
		fmt.Fprintf(tw, "  - %s\tvia %s\t%s\n", a.Pkg.ID, a.Manager, strings.Join(a.UninstallCmd, " "))
	}
	for _, lp := range result.Unchanged {
		fmt.Fprintf(tw, "    %s\tvia %s\t(up to date)\n", lp.ID, lp.Manager)
	}
	tw.Flush()
	fmt.Fprintln(w)

	unresolvedCount := 0
	for _, a := range result.ToInstall {
		if !a.Resolved() {
			unresolvedCount++
		}
	}
	if unresolvedCount > 0 {
		fmt.Fprintf(w, "%d package(s) could not be resolved: no compatible manager found on this host.\n", unresolvedCount)
		fmt.Fprintln(w, "Hint: install a supported package manager or add a 'managers' entry in gpm.json.")
		fmt.Fprintln(w, "Use --strict to treat unresolved packages as a hard error.")
	}
	return
}

// ApplyExecution records the outcome of ExecuteApply so the caller can update
// the lock file: only successful operations change persisted state.
type ApplyExecution struct {
	Installed   []gpmfile.LockedPackage // successfully installed
	Uninstalled []string                // IDs successfully removed
	Errors      []error
}

// ExecuteApply runs all removals then all installs from a ReconcileResult.
// Removals are run first (mirrors how package managers handle upgrades/downgrades).
// Cache-clean commands run once per manager that had at least one successful removal.
// Returns an ApplyExecution so the caller can write an updated lock file that
// reflects only what actually succeeded.
func ExecuteApply(result ReconcileResult, stdin io.Reader, stdout, stderr io.Writer) ApplyExecution {
	var out ApplyExecution
	cleanManagers := make(map[string]bool)

	for _, a := range result.ToRemove {
		fmt.Fprintf(stdout, "\n==> %s\n", strings.Join(a.UninstallCmd, " "))
		cmd := exec.Command(a.UninstallCmd[0], a.UninstallCmd[1:]...)
		cmd.Stdin = stdin
		cmd.Stdout = stdout
		cmd.Stderr = stderr
		if err := cmd.Run(); err != nil {
			out.Errors = append(out.Errors, fmt.Errorf("remove %q (via %s): %w", a.Pkg.ID, a.Manager, err))
		} else {
			out.Uninstalled = append(out.Uninstalled, a.Pkg.ID)
			cleanManagers[a.Manager] = true
		}
	}

	for managerName := range cleanManagers {
		mgr := adapter.ByName(managerName)
		if mgr == nil {
			continue
		}
		for _, cleanCmd := range mgr.PlanClean() {
			fmt.Fprintf(stdout, "\n==> %s\n", strings.Join(cleanCmd, " "))
			cmd := exec.Command(cleanCmd[0], cleanCmd[1:]...)
			cmd.Stdin = stdin
			cmd.Stdout = stdout
			cmd.Stderr = stderr
			if err := cmd.Run(); err != nil {
				out.Errors = append(out.Errors, fmt.Errorf("cache clean (via %s): %w", managerName, err))
			}
		}
	}

	for _, a := range result.ToInstall {
		if !a.Resolved() {
			continue
		}
		fmt.Fprintf(stdout, "\n==> %s\n", strings.Join(a.Cmd, " "))
		cmd := exec.Command(a.Cmd[0], a.Cmd[1:]...)
		cmd.Stdin = stdin
		cmd.Stdout = stdout
		cmd.Stderr = stderr
		if err := cmd.Run(); err != nil {
			out.Errors = append(out.Errors, fmt.Errorf("install %q (via %s): %w", a.Pkg.ID, a.Manager, err))
		} else {
			out.Installed = append(out.Installed, gpmfile.LockedPackage{
				ID:      a.Pkg.ID,
				Manager: a.Manager,
				PkgName: a.PkgName,
			})
		}
	}

	return out
}
