// Package resolver detects available package managers and resolves packages
// to concrete install actions based on what is present on the current host.
package resolver

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/ks1686/genv/internal/adapter"
	"github.com/ks1686/genv/internal/genvfile"
	"github.com/ks1686/genv/internal/schema"
	"github.com/ks1686/genv/internal/version"
)

// fprintf/fPrintln/fprint wrap fmt write functions to discard unactionable I/O errors.
func fprintf(w io.Writer, format string, a ...any) { _, _ = fmt.Fprintf(w, format, a...) }
func fPrintln(w io.Writer, a ...any)               { _, _ = fmt.Fprintln(w, a...) }
func fprint(w io.Writer, a ...any)                 { _, _ = fmt.Fprint(w, a...) }

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
	Cmd          []string // installation command; nil if unresolved
	UninstallCmd []string // uninstall command; nil if unresolved
}

// Resolved reports whether a manager was found for this package.
func (a Action) Resolved() bool { return a.Manager != "" }

// ResolveOne resolves a single package into an Action using the provided set of
// available manager names. Used by addCmd to install one package immediately.
func ResolveOne(pkg schema.Package, available map[string]bool) Action {
	return resolveOnGOOS(pkg, available, runtime.GOOS)
}

// Plan resolves every package in f into an Action, using the provided set of
// available manager names. Call Detect() to build the available map.
func Plan(f *schema.GenvFile, available map[string]bool) []Action {
	return planOnGOOS(f, available, runtime.GOOS)
}

func planOnGOOS(f *schema.GenvFile, available map[string]bool, goos string) []Action {
	actions := make([]Action, 0, len(f.Packages))
	for _, pkg := range f.Packages {
		actions = append(actions, resolveOnGOOS(pkg, available, goos))
	}
	return actions
}

func resolveOnGOOS(pkg schema.Package, available map[string]bool, goos string) Action {
	// 1. Honor the prefer hint if that manager is available.
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

	// 3. Fall back to the first available default-fallback-eligible adapter,
	//    using the package ID as name. Only OS/system package managers are
	//    eligible here; ecosystem/language/plugin managers are explicit-only
	//    (reachable via prefer or the managers map above) so `genv add git`
	//    never silently resolves to npm/cargo/go just because one happens to be
	//    installed.
	for _, a := range adapter.All {
		if available[a.Name()] && adapter.IsDefaultFallbackEligible(a) && adapter.AutomaticOnGOOS(a.Name(), goos) {
			name, _ := a.NormalizeID(pkg.ID, pkg.Managers)
			return Action{Pkg: pkg, Manager: a.Name(), PkgName: name, Cmd: a.PlanInstall(name), UninstallCmd: a.PlanUninstall(name)}
		}
	}

	// Unresolved — no compatible manager on this host.
	return Action{Pkg: pkg}
}

// PrintPlan writes a human-readable installation plan to w and returns the number
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
	fprintf(w, "Installation plan — %d package", total)
	if total != 1 {
		fprint(w, "s")
	}
	if unresolved > 0 {
		fprintf(w, " (%d unresolved)", unresolved)
	}
	fPrintln(w)
	fPrintln(w)

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	for _, a := range actions {
		if a.Resolved() {
			fprintf(tw, "  %s\tvia %s\t%s\n", a.Pkg.ID, a.Manager, strings.Join(a.Cmd, " "))
		} else {
			fprintf(tw, "  %s\tunresolved\t(no manager available)\n", a.Pkg.ID)
		}
	}
	_ = tw.Flush()
	fPrintln(w)

	if unresolved > 0 {
		fprintf(w, "%d package(s) could not be resolved: no compatible manager found on this host.\n", unresolved)
		fPrintln(w, "Hint: install a supported package manager or add a 'managers' entry in genv.json.")
		fPrintln(w, "Use --strict to treat unresolved packages as a hard error.")
	}
	return
}

// runSubcmd prints the command to stdout, spawns it as a subprocess wiring
// stdin/stdout/stderr, logs timing via slog, and returns any execution error.
// Shared by Execute and ExecuteApply to avoid repeating the logging boilerplate.
func runSubcmd(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	if len(args) == 0 || args[0] == "" {
		return fmt.Errorf("empty command")
	}
	fprintf(stdout, "\n==> %s\n", strings.Join(args, " "))
	slog.Debug("spawn", "cmd", strings.Join(args, " "))
	start := time.Now()
	runCtx := ctx
	cancel := func() {}
	if d := SubprocessTimeout(ctx); d > 0 {
		runCtx, cancel = context.WithTimeout(ctx, d)
	}
	defer cancel()
	cmd := exec.CommandContext(runCtx, args[0], args[1:]...)
	cmd.Stdin = stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if runtime.GOOS == "windows" {
		if _, err := exec.LookPath("git"); err != nil {
			if dir := adapter.ScoopGitCmdDir(); dir != "" {
				cmd.Env = append(os.Environ(), "PATH="+dir+string(os.PathListSeparator)+os.Getenv("PATH"))
			}
		}
	}
	err := cmd.Run()
	slog.Debug("done", "cmd", args[0], "duration", time.Since(start), "err", err)
	return err
}

// Execute runs each resolved install action sequentially, writing subprocess
// output to stdout/stderr. stdin is forwarded to child processes so that
// interactive password prompts (e.g. sudo) work correctly.
// Unresolved packages are silently skipped.
// ctx controls the deadline for every subprocess; use context.Background() for no timeout.
// Returns one error per failed install; a non-empty slice means partial failure.
func Execute(ctx context.Context, actions []Action, stdin io.Reader, stdout, stderr io.Writer) []error {
	var errs []error
	for _, a := range actions {
		if !a.Resolved() {
			continue
		}
		if err := runSubcmd(ctx, a.Cmd, stdin, stdout, stderr); err != nil {
			errs = append(errs, fmt.Errorf("package %q (via %s): %w", a.Pkg.ID, a.Manager, err))
		}
	}
	return errs
}

// ---- Upgrade (genv upgrade) --------------------------------------------------

// UpgradeAction is the resolved upgrade action for one or more packages that
// can be upgraded together by the same package manager. Most adapters produce
// one action per package; adapters implementing BatchUpgrader may produce one
// action for several packages.
type UpgradeAction struct {
	LPs []genvfile.LockedPackage
	Mgr adapter.Adapter
	Cmd []string
}

// SkippedPackage records a package that was skipped during upgrade planning.
type SkippedPackage struct {
	ID      string
	Manager string
	Reason  string
}

// PlanUpgrade builds an upgrade plan for all packages tracked in the lock file.
// It returns a list of actions and a list of packages that were skipped because
// their recorded package manager adapter is no longer registered.
// Packages are grouped by manager; when a manager implements BatchUpgrader and
// has more than one tracked package, a single batched command is emitted.
func PlanUpgrade(packages []genvfile.LockedPackage) (plan []UpgradeAction, skipped []SkippedPackage) {
	adapters := make(map[string]adapter.Adapter)
	getAdapter := func(name string) adapter.Adapter {
		if mgr, ok := adapters[name]; ok {
			return mgr
		}
		mgr := adapter.ByName(name)
		adapters[name] = mgr
		return mgr
	}

	// Group packages by manager while preserving first-seen order.
	type group struct {
		mgr adapter.Adapter
		lps []genvfile.LockedPackage
	}
	groups := make(map[string]*group)
	var order []string
	for _, lp := range packages {
		mgr := getAdapter(lp.Manager)
		if mgr == nil {
			skipped = append(skipped, SkippedPackage{ID: lp.ID, Manager: lp.Manager, Reason: fmt.Sprintf("adapter %q not registered", lp.Manager)})
			continue
		}
		if _, ok := groups[lp.Manager]; !ok {
			groups[lp.Manager] = &group{mgr: mgr}
			order = append(order, lp.Manager)
		}
		groups[lp.Manager].lps = append(groups[lp.Manager].lps, lp)
	}

	for _, name := range order {
		g := groups[name]
		if batcher, ok := g.mgr.(adapter.BatchUpgrader); ok && len(g.lps) > 1 {
			pkgNames := make([]string, len(g.lps))
			for i, lp := range g.lps {
				pkgNames[i] = lp.PkgName
			}
			plan = append(plan, UpgradeAction{LPs: g.lps, Mgr: g.mgr, Cmd: batcher.PlanUpgradeBatch(pkgNames)})
			continue
		}
		for _, lp := range g.lps {
			plan = append(plan, UpgradeAction{LPs: []genvfile.LockedPackage{lp}, Mgr: g.mgr, Cmd: g.mgr.PlanUpgrade(lp.PkgName)})
		}
	}
	return plan, skipped
}

// lookupAdapter resolves a manager name to its adapter. It is a var so tests
// can inject fake adapters for FilterOutdated without touching the global
// registry, mirroring the lookPath seam in the adapter package.
var lookupAdapter = adapter.ByName

// FilterOutdated narrows packages to those with an update actually available,
// querying each manager's OutdatedLister capability. Packages whose manager does
// not implement OutdatedLister are kept unchanged (no detection is possible, so
// nothing is dropped). When a manager's outdated query fails, all of that
// manager's packages are kept conservatively and a warning is recorded, so a
// real update is never silently missed. Input order is preserved.
func FilterOutdated(packages []genvfile.LockedPackage) (kept []genvfile.LockedPackage, warnings []string) {
	// Group package names by manager so each manager is queried at most once,
	// preserving first-seen order for stable, testable output.
	type group struct {
		mgr      adapter.Adapter
		pkgNames []string
	}
	groups := make(map[string]*group)
	var order []string
	for _, lp := range packages {
		if _, ok := groups[lp.Manager]; !ok {
			groups[lp.Manager] = &group{mgr: lookupAdapter(lp.Manager)}
			order = append(order, lp.Manager)
		}
		g := groups[lp.Manager]
		g.pkgNames = append(g.pkgNames, lp.PkgName)
	}

	// For each manager, decide which of its packages to keep.
	// keep[manager] == nil means "keep all" (no detection / query failed);
	// otherwise it is the set of manager-native names that are outdated.
	keep := make(map[string]map[string]bool, len(order))
	for _, name := range order {
		g := groups[name]
		lister, ok := g.mgr.(adapter.OutdatedLister)
		if !ok {
			keep[name] = nil // no capability: keep all
			continue
		}
		started := time.Now()
		outdated, err := lister.ListOutdated(g.pkgNames)
		elapsed := time.Since(started).Round(time.Millisecond)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("could not determine outdated packages for %s after %s (%v) — keeping all", name, elapsed, err))
			keep[name] = nil // query failed: keep all conservatively
			continue
		}
		// Timing is logged as a warning so scheduled updates.log and CLI stderr
		// can distinguish a real slow query from a silent timeout fallback.
		warnings = append(warnings, fmt.Sprintf("outdated timing: %s took %s (%d hits)", name, elapsed, len(outdated)))
		set := make(map[string]bool, len(outdated))
		for pkgName := range outdated {
			set[pkgName] = true
		}
		keep[name] = set
	}

	for _, lp := range packages {
		set := keep[lp.Manager]
		if set == nil || set[lp.PkgName] {
			kept = append(kept, lp)
		}
	}
	return kept, warnings
}

// UpgradeExecution records the outcome of ExecuteUpgrade so the caller can update
// the lock file with new versions.
type UpgradeExecution struct {
	Upgraded []genvfile.LockedPackage
	Errors   []error
	Failures []UpgradeFailure
}

// UpgradeFailure identifies the tracked packages affected by one failed action.
type UpgradeFailure struct {
	IDs []string
	Err error
}

// ExecuteUpgrade runs each resolved upgrade action sequentially, updating the
// InstalledVersion for packages whose version changed. Returns an UpgradeExecution
// holding the updated packages and any errors encountered.
func ExecuteUpgrade(ctx context.Context, plan []UpgradeAction, stdin io.Reader, stdout, stderr io.Writer) UpgradeExecution {
	var out UpgradeExecution
	for _, a := range plan {
		ids := make([]string, len(a.LPs))
		for i, lp := range a.LPs {
			ids[i] = lp.ID
		}

		cmdErr := runSubcmd(ctx, a.Cmd, stdin, stdout, stderr)
		if cmdErr != nil {
			wrappedErr := fmt.Errorf("upgrade %q: %w", ids, cmdErr)
			out.Errors = append(out.Errors, wrappedErr)
			out.Failures = append(out.Failures, UpgradeFailure{IDs: ids, Err: wrappedErr})
		}

		// Collect current versions for every package in the action. Use a single
		// ListInstalledVersions call when the adapter supports it, then fall back
		// to per-package QueryVersion for anything missing.
		versions := make(map[string]string, len(a.LPs))
		if versionLister, ok := a.Mgr.(adapter.VersionLister); ok {
			if listedVersions, err := versionLister.ListInstalledVersions(); err == nil {
				for _, lp := range a.LPs {
					if v, ok := listedVersions[lp.PkgName]; ok {
						versions[lp.ID] = v
					}
				}
			}
		}
		for _, lp := range a.LPs {
			if _, ok := versions[lp.ID]; ok {
				continue
			}
			if v, err := a.Mgr.QueryVersion(lp.PkgName); err == nil && v != "" {
				versions[lp.ID] = v
			}
		}

		// Update the lock for packages whose version actually changed. On a
		// successful command include every package so already-current packages
		// remain recorded; on failure only include packages that upgraded anyway.
		for i := range a.LPs {
			lp := &a.LPs[i]
			v, hasV := versions[lp.ID]
			versionChanged := hasV && v != "" && v != lp.InstalledVersion
			if cmdErr == nil || versionChanged {
				if versionChanged {
					lp.InstalledVersion = v
				}
				out.Upgraded = append(out.Upgraded, *lp)
			}
		}
	}
	return out
}

// ---- Reconcile (genv apply) --------------------------------------------------

// versionDrifted reports whether the InstalledVersion recorded for lp fails the
// version constraint in pkg. Returns false when InstalledVersion is empty
// (old lock entries without version data are never treated as drifted).
func versionDrifted(pkg schema.Package, lp genvfile.LockedPackage) bool {
	return lp.InstalledVersion != "" && !version.Satisfies(pkg.Version, lp.InstalledVersion)
}

// ReconcileResult holds the delta between the desired state (genv.json) and the
// previously applied state (genv.lock.json). ToInstall are packages added to the
// spec since the last apply; ToRemove are packages that were removed from it.
type ReconcileResult struct {
	ToInstall []Action
	ToRemove  []Action // UninstallCmd populated; Pkg.ID identifies the package
	Unchanged []genvfile.LockedPackage
	Adopted   []genvfile.LockedPackage // live-installed, not yet in the lock
	Warnings  []string
}

// LiveSet is manager name → manager-native package name → installed.
// Names are matched case-insensitively. A nil LiveSet means lock-only
// (do not probe the live system).
type LiveSet map[string]map[string]bool

func (s LiveSet) has(manager, pkgName string) bool {
	if s == nil || manager == "" || pkgName == "" {
		return false
	}
	names := s[manager]
	if names[pkgName] {
		return true
	}
	for n := range names {
		if strings.EqualFold(n, pkgName) {
			return true
		}
	}
	return false
}

// LoadLiveSet calls ListInstalled once per available manager. Listing errors
// become warnings; they never fail the whole apply/status run.
func LoadLiveSet(available map[string]bool) (LiveSet, []string) {
	out := make(LiveSet)
	var warns []string
	for name, ok := range available {
		if !ok {
			continue
		}
		mgr := adapter.ByName(name)
		if mgr == nil {
			continue
		}
		list, err := mgr.ListInstalled()
		if err != nil {
			warns = append(warns, fmt.Sprintf("listing %s: %v", name, err))
			continue
		}
		set := make(map[string]bool, len(list))
		for _, pkgName := range list {
			set[pkgName] = true
		}
		out[name] = set
	}
	return out, warns
}

// Reconcile computes the delta between the desired packages (from genv.json)
// and the previously applied state (from genv.lock.json).
//
//   - ToInstall: in desired but not in lock → resolve via available managers.
//   - ToRemove:  in lock but not in desired → uninstall using the manager
//     recorded in the lock (not re-resolved, preserving the original manager).
//   - Unchanged: in both desired and lock → nothing to do.
//
// Reconcile is lock-only. Prefer ReconcileWith when a live inventory exists.
func Reconcile(desired []schema.Package, managed []genvfile.LockedPackage, available map[string]bool) ReconcileResult {
	return ReconcileWith(desired, managed, available, nil)
}

// ReconcileWith is Reconcile plus a live inventory. Packages in desired, not in
// the lock, but already installed under the resolved manager are Adopted
// instead of ToInstall so apply can lock them without spawning an installer.
func ReconcileWith(desired []schema.Package, managed []genvfile.LockedPackage, available map[string]bool, live LiveSet) ReconcileResult {
	adapters := make(map[string]adapter.Adapter)
	getAdapter := func(name string) adapter.Adapter {
		if mgr, ok := adapters[name]; ok {
			return mgr
		}
		mgr := adapter.ByName(name)
		adapters[name] = mgr
		return mgr
	}

	managedByID := make(map[string]genvfile.LockedPackage, len(managed))
	for _, lp := range managed {
		managedByID[lp.ID] = lp
	}
	desiredByID := make(map[string]bool, len(desired))
	specByID := make(map[string]schema.Package, len(desired))
	for _, pkg := range desired {
		desiredByID[pkg.ID] = true
		specByID[pkg.ID] = pkg
	}

	var toInstall []Action
	var adopted []genvfile.LockedPackage
	for _, pkg := range desired {
		lp, alreadyManaged := managedByID[pkg.ID]
		if !alreadyManaged {
			action := resolveOnGOOS(pkg, available, runtime.GOOS)
			if action.Resolved() && live.has(action.Manager, action.PkgName) {
				adopted = append(adopted, genvfile.LockedPackage{
					ID:      pkg.ID,
					Manager: action.Manager,
					PkgName: action.PkgName,
				})
				continue
			}
			toInstall = append(toInstall, action)
			continue
		}
		// Package is already in the lock. Check version constraint: if the lock
		// recorded an InstalledVersion and it no longer satisfies the spec
		// constraint, queue for reinstallation.
		if versionDrifted(pkg, lp) {
			toInstall = append(toInstall, resolveOnGOOS(pkg, available, runtime.GOOS))
		}
	}

	var toRemove []Action
	var unchanged []genvfile.LockedPackage
	var warnings []string
	for _, lp := range managed {
		if !desiredByID[lp.ID] {
			a := getAdapter(lp.Manager)
			if a == nil {
				unchanged = append(unchanged, lp)
				warnings = append(warnings, fmt.Sprintf("lock package %q uses unregistered manager %q; skipping uninstall", lp.ID, lp.Manager))
				continue
			}
			toRemove = append(toRemove, Action{
				Pkg:          schema.Package{ID: lp.ID},
				Manager:      lp.Manager,
				PkgName:      lp.PkgName,
				UninstallCmd: a.PlanUninstall(lp.PkgName),
			})
			continue
		}
		// In desired — skip packages queued for reinstall; they must not appear in Unchanged.
		if versionDrifted(specByID[lp.ID], lp) {
			continue
		}
		unchanged = append(unchanged, lp)
	}

	return ReconcileResult{ToInstall: toInstall, ToRemove: toRemove, Unchanged: unchanged, Adopted: adopted, Warnings: warnings}
}

// PrintReconcilePlan writes a human-readable apply plan to w. Each line is
// prefixed with '+' (install), '-' (remove), or ' ' (unchanged). Returns the
// counts of packages to install, to remove, and unresolved so the caller can
// decide whether there is any work to do and enforce --strict without a second pass.
func PrintReconcilePlan(result ReconcileResult, w io.Writer) (toInstall, toRemove, unresolved int) {
	toInstall = len(result.ToInstall)
	toRemove = len(result.ToRemove)
	unchanged := len(result.Unchanged)
	adopted := len(result.Adopted)
	total := toInstall + toRemove + unchanged + adopted

	fprintf(w, "Apply plan — %d package", total)
	if total != 1 {
		fprint(w, "s")
	}
	var parts []string
	if toInstall > 0 {
		parts = append(parts, fmt.Sprintf("%d to install", toInstall))
	}
	if toRemove > 0 {
		parts = append(parts, fmt.Sprintf("%d to remove", toRemove))
	}
	if adopted > 0 {
		parts = append(parts, fmt.Sprintf("%d already installed", adopted))
	}
	if unchanged > 0 {
		parts = append(parts, fmt.Sprintf("%d up to date", unchanged))
	}
	if len(parts) > 0 {
		fprintf(w, " (%s)", strings.Join(parts, ", "))
	}
	fPrintln(w)
	fPrintln(w)

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	for _, a := range result.ToInstall {
		if a.Resolved() {
			fprintf(tw, "  + %s	via %s	%s\n", a.Pkg.ID, a.Manager, strings.Join(a.Cmd, " "))
		} else {
			fprintf(tw, "  + %s	unresolved	(no manager available)\n", a.Pkg.ID)
		}
	}
	for _, a := range result.ToRemove {
		fprintf(tw, "  - %s	via %s	%s\n", a.Pkg.ID, a.Manager, strings.Join(a.UninstallCmd, " "))
	}
	for _, lp := range result.Adopted {
		fprintf(tw, "  = %s	via %s	(already installed)\n", lp.ID, lp.Manager)
	}
	for _, lp := range result.Unchanged {
		fprintf(tw, "    %s	via %s	(up to date)\n", lp.ID, lp.Manager)
	}
	_ = tw.Flush()
	fPrintln(w)

	for _, a := range result.ToInstall {
		if !a.Resolved() {
			unresolved++
		}
	}
	if unresolved > 0 {
		fprintf(w, "%d package(s) could not be resolved: no compatible manager found on this host.\n", unresolved)
		fPrintln(w, "Hint: install a supported package manager or add a 'managers' entry in genv.json.")
		fPrintln(w, "Use --strict to treat unresolved packages as a hard error.")
	}
	return
}

// ApplyExecution records the outcome of ExecuteApply so the caller can update
// the lock file: only successful operations change persisted state.
type ApplyExecution struct {
	Installed   []genvfile.LockedPackage // successfully installed
	Uninstalled []string                 // IDs successfully removed
	Errors      []error
}

// ExecuteApply runs all removals then all installs from a ReconcileResult.
// Removals are run first (mirrors how package managers handle upgrades/downgrades).
// Cache-clean commands run once per manager that had at least one successful removal.
// ctx controls the deadline for every subprocess; use context.Background() for no timeout.
// Returns an ApplyExecution so the caller can write an updated lock file that
// reflects only what actually succeeded.
func ExecuteApply(ctx context.Context, result ReconcileResult, stdin io.Reader, stdout, stderr io.Writer) ApplyExecution {
	var out ApplyExecution
	cleanManagers := make(map[string]bool)

	for _, a := range result.ToRemove {
		if err := runSubcmd(ctx, a.UninstallCmd, stdin, stdout, stderr); err != nil {
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
			if err := runSubcmd(ctx, cleanCmd, stdin, stdout, stderr); err != nil {
				out.Errors = append(out.Errors, fmt.Errorf("cache clean (via %s): %w", managerName, err))
			}
		}
	}

	adapters := make(map[string]adapter.Adapter)
	getAdapter := func(name string) adapter.Adapter {
		if mgr, ok := adapters[name]; ok {
			return mgr
		}
		mgr := adapter.ByName(name)
		adapters[name] = mgr
		return mgr
	}

	for _, a := range result.ToInstall {
		if !a.Resolved() {
			continue
		}
		if mgr := getAdapter(a.Manager); mgr != nil {
			if _, trackOnly := mgr.(adapter.TrackOnly); trackOnly {
				installed, qerr := mgr.Query(a.PkgName)
				if qerr != nil {
					out.Errors = append(out.Errors, fmt.Errorf("query %q (via %s): %w", a.Pkg.ID, a.Manager, qerr))
					continue
				}
				if !installed {
					out.Errors = append(out.Errors, fmt.Errorf("install %q (via %s): not on PATH — install it with the official installer, then re-run apply", a.Pkg.ID, a.Manager))
					continue
				}
				out.Installed = append(out.Installed, genvfile.LockedPackage{
					ID:      a.Pkg.ID,
					Manager: a.Manager,
					PkgName: a.PkgName,
				})
				continue
			}
		}
		if err := runSubcmd(ctx, a.Cmd, stdin, stdout, stderr); err != nil {
			out.Errors = append(out.Errors, fmt.Errorf("install %q (via %s): %w", a.Pkg.ID, a.Manager, err))
		} else {
			lp := genvfile.LockedPackage{
				ID:      a.Pkg.ID,
				Manager: a.Manager,
				PkgName: a.PkgName,
			}
			// Best-effort version capture; ignore errors (non-critical).
			if mgr := getAdapter(a.Manager); mgr != nil {
				if v, err := mgr.QueryVersion(a.PkgName); err == nil {
					lp.InstalledVersion = v
				}
			}
			out.Installed = append(out.Installed, lp)
		}
	}

	return out
}
