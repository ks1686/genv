package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"text/tabwriter"

	"github.com/ks1686/genv/internal/adapter"
	"github.com/ks1686/genv/internal/commands"
	"github.com/ks1686/genv/internal/genvfile"
	"github.com/ks1686/genv/internal/logging"
	"github.com/ks1686/genv/internal/output"
	"github.com/ks1686/genv/internal/resolver"
	"github.com/ks1686/genv/internal/schema"
)

// Structured exit codes.
const (
	exitOK         = 0 // success
	exitUsage      = 1 // bad arguments or unknown command
	exitIO         = 2 // filesystem or serialisation error
	exitValidation = 3 // genv.json fails schema validation
	exitLogic      = 4 // semantic error (duplicate id, not found, etc.)
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	if len(args) == 0 {
		printUsage()
		return exitUsage
	}

	switch args[0] {
	case "add":
		return addCmd(args[1:])
	case "remove", "rm":
		return removeCmd(args[1:])
	case "adopt":
		return adoptCmd(args[1:])
	case "disown":
		return disownCmd(args[1:])
	case "list", "ls":
		return listCmd(args[1:])
	case "apply":
		return applyCmd(args[1:])
	case "edit":
		return editCmd(args[1:])
	case "clean":
		return cleanCmd(args[1:])
	case "scan":
		return scanCmd(args[1:])
	case "status":
		return statusCmd(args[1:])
	case "version", "--version":
		printVersion()
		return exitOK
	case "help", "--help", "-h":
		printUsage()
		return exitOK
	default:
		fmt.Fprintf(os.Stderr, "genv: unknown command %q\n\nRun 'genv help' for usage.\n", args[0])
		return exitUsage
	}
}

// defaultSpecPath returns the XDG-aware default path for genv.json.
// Falls back to "genv.json" in the current directory if the config dir cannot
// be determined (e.g. no home directory set).
func defaultSpecPath() string {
	p, err := genvfile.DefaultSpecPath()
	if err != nil {
		return "genv.json"
	}
	return p
}

// confirm writes prompt to stdout and reads a y/Y response from stdin.
// Returns true if the user confirmed.
func confirm(prompt string) bool {
	fmt.Fprint(os.Stdout, prompt)
	answer, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	answer = strings.TrimSpace(answer)
	return answer == "y" || answer == "Y"
}

// addCmd implements `genv add <id> [flags]`.
// Adds the package to genv.json and immediately installs it, then updates the lock.
func addCmd(args []string) int {
	fs := flag.NewFlagSet("add", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: genv add <id> [flags]")
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "flags:")
		fs.PrintDefaults()
	}

	file := fs.String("file", defaultSpecPath(), "path to genv.json")
	version := fs.String("version", "", `version constraint, e.g. "0.10.*" (default: omitted, meaning any)`)
	prefer := fs.String("prefer", "", "preferred package manager (e.g. brew)")
	managerFlag := fs.String("manager", "", `manager-specific names, comma-separated mgr:name pairs (e.g. flatpak:org.mozilla.firefox,brew:firefox)`)

	id, flagArgs := extractPositional(args)
	if err := fs.Parse(flagArgs); err != nil {
		return exitUsage
	}
	if id == "" {
		fmt.Fprintln(os.Stderr, "genv add: missing package id")
		fs.Usage()
		return exitUsage
	}

	managers, err := parseManagerFlag(*managerFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "genv add: --manager: %v\n", err)
		return exitUsage
	}

	// 1. Update genv.json.
	f, isNew, err := genvfile.ReadOrNew(*file)
	if err != nil {
		fmt.Fprintf(os.Stderr, "genv: %v\n", err)
		if errors.Is(err, genvfile.ErrInvalidFile) {
			return exitValidation
		}
		return exitIO
	}

	if err := commands.Add(f, id, *version, *prefer, managers); err != nil {
		fmt.Fprintf(os.Stderr, "genv: %v\n", err)
		if errors.Is(err, commands.ErrAlreadyTracked) {
			return exitLogic
		}
		return exitUsage
	}

	if err := genvfile.Write(*file, f); err != nil {
		fmt.Fprintf(os.Stderr, "genv: %v\n", err)
		return exitIO
	}
	if isNew {
		fmt.Fprintf(os.Stdout, "created %s\n", *file)
	}

	// 2. Resolve and install the package.
	available := resolver.Detect()
	pkg := schema.Package{ID: id, Version: *version, Prefer: *prefer, Managers: managers}
	action := resolver.ResolveOne(pkg, available)
	if !action.Resolved() {
		fmt.Fprintf(os.Stdout, "added %s to spec (no manager available to install it now; run 'genv apply' after installing a compatible package manager)\n", id)
		return exitOK
	}

	fmt.Fprintf(os.Stdout, "added %s — installing via %s\n", id, action.Manager)
	fmt.Fprintf(os.Stdout, "\n==> %s\n", strings.Join(action.Cmd, " "))
	cmd := exec.Command(action.Cmd[0], action.Cmd[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		// Install failure is non-fatal: the spec was already updated.
		// The user can run 'genv apply' to retry.
		fmt.Fprintf(os.Stderr, "genv: install failed: %v\n", err)
		fmt.Fprintln(os.Stderr, "Package was added to spec. Run 'genv apply' to retry.")
		return exitOK
	}

	// 3. Update lock file.
	lockPath := genvfile.LockPathFrom(*file)
	lf, err := genvfile.ReadLock(lockPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "genv: reading lock: %v\n", err)
		return exitIO
	}
	lf.Packages = append(lf.Packages, genvfile.LockedPackage{
		ID:      action.Pkg.ID,
		Manager: action.Manager,
		PkgName: action.PkgName,
	})
	if err := genvfile.WriteLock(lockPath, lf); err != nil {
		fmt.Fprintf(os.Stderr, "genv: writing lock: %v\n", err)
		return exitIO
	}

	return exitOK
}

// removeCmd implements `genv remove <id>`.
// Removes the package from genv.json and immediately uninstalls it, then updates the lock.
func removeCmd(args []string) int {
	fs := flag.NewFlagSet("remove", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: genv remove <id> [flags]")
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "flags:")
		fs.PrintDefaults()
	}

	file := fs.String("file", defaultSpecPath(), "path to genv.json")

	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	if fs.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "genv remove: missing package id")
		fs.Usage()
		return exitUsage
	}
	id := fs.Arg(0)

	// 1. Update genv.json.
	f, err := genvfile.Read(*file)
	if err != nil {
		if errors.Is(err, genvfile.ErrNotFound) {
			fmt.Fprintf(os.Stderr, "genv: %s not found\n", *file)
			return exitLogic
		}
		fmt.Fprintf(os.Stderr, "genv: %v\n", err)
		if errors.Is(err, genvfile.ErrInvalidFile) {
			return exitValidation
		}
		return exitIO
	}

	if err := commands.Remove(f, id); err != nil {
		fmt.Fprintf(os.Stderr, "genv: %v\n", err)
		return exitLogic
	}

	if err := genvfile.Write(*file, f); err != nil {
		fmt.Fprintf(os.Stderr, "genv: %v\n", err)
		return exitIO
	}

	// 2. Find the package in the lock file to know which manager installed it.
	lockPath := genvfile.LockPathFrom(*file)
	lf, err := genvfile.ReadLock(lockPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "genv: reading lock: %v\n", err)
		return exitIO
	}

	var locked *genvfile.LockedPackage
	remaining := make([]genvfile.LockedPackage, 0, len(lf.Packages))
	for i := range lf.Packages {
		if lf.Packages[i].ID == id {
			locked = &lf.Packages[i]
		} else {
			remaining = append(remaining, lf.Packages[i])
		}
	}

	if locked == nil {
		// Never installed by genv — nothing to uninstall on the system.
		fmt.Fprintf(os.Stdout, "removed %s from spec (was not installed by genv)\n", id)
		return exitOK
	}

	// 3. Uninstall from the system using the manager recorded in the lock.
	mgr := adapter.ByName(locked.Manager)
	if mgr == nil {
		fmt.Fprintf(os.Stderr, "genv: adapter %q no longer registered; cannot uninstall — remove manually\n", locked.Manager)
		return exitLogic
	}

	uninstallCmd := mgr.PlanUninstall(locked.PkgName)
	fmt.Fprintf(os.Stdout, "removed %s from spec — uninstalling via %s\n", id, locked.Manager)
	fmt.Fprintf(os.Stdout, "\n==> %s\n", strings.Join(uninstallCmd, " "))
	cmd := exec.Command(uninstallCmd[0], uninstallCmd[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	uninstallErr := cmd.Run()
	if uninstallErr != nil {
		fmt.Fprintf(os.Stderr, "genv: uninstall failed: %v\n", uninstallErr)
		// Still update the lock — the package is removed from the spec.
	}

	// Cache clean.
	for _, cleanCmd := range mgr.PlanClean() {
		fmt.Fprintf(os.Stdout, "\n==> %s\n", strings.Join(cleanCmd, " "))
		c := exec.Command(cleanCmd[0], cleanCmd[1:]...)
		c.Stdin = os.Stdin
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr
		if err := c.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "genv: cache clean warning: %v\n", err)
		}
	}

	// 4. Update lock file (remove the entry regardless of uninstall success).
	lf.Packages = remaining
	if err := genvfile.WriteLock(lockPath, lf); err != nil {
		fmt.Fprintf(os.Stderr, "genv: writing lock: %v\n", err)
		return exitIO
	}

	if uninstallErr != nil {
		return exitLogic
	}
	return exitOK
}

// adoptCmd implements `genv adopt <id> [flags]`.
// Verifies the package is already installed on the system and then adds it to
// genv.json and the lock file without running an install command.
func adoptCmd(args []string) int {
	fs := flag.NewFlagSet("adopt", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: genv adopt <id> [flags]")
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "flags:")
		fs.PrintDefaults()
	}

	file := fs.String("file", defaultSpecPath(), "path to genv.json")
	version := fs.String("version", "", `version constraint, e.g. "0.10.*" (default: omitted, meaning any)`)
	prefer := fs.String("prefer", "", "preferred package manager (e.g. brew)")
	managerFlag := fs.String("manager", "", `manager-specific names, comma-separated mgr:name pairs (e.g. flatpak:org.mozilla.firefox,brew:firefox)`)

	id, flagArgs := extractPositional(args)
	if err := fs.Parse(flagArgs); err != nil {
		return exitUsage
	}
	if id == "" {
		fmt.Fprintln(os.Stderr, "genv adopt: missing package id")
		fs.Usage()
		return exitUsage
	}

	managers, err := parseManagerFlag(*managerFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "genv adopt: --manager: %v\n", err)
		return exitUsage
	}

	// 1. Resolve to find which manager handles this package.
	available := resolver.Detect()
	pkg := schema.Package{ID: id, Version: *version, Prefer: *prefer, Managers: managers}
	action := resolver.ResolveOne(pkg, available)
	if !action.Resolved() {
		fmt.Fprintf(os.Stderr, "genv adopt: no available manager for %q — install a compatible package manager first\n", id)
		return exitLogic
	}

	// 2. Verify the package is actually installed.
	mgr := adapter.ByName(action.Manager)
	installed, err := mgr.Query(action.PkgName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "genv adopt: querying %s: %v\n", action.Manager, err)
		return exitLogic
	}
	if !installed {
		fmt.Fprintf(os.Stderr, "genv adopt: %q is not installed via %s — use 'genv add %s' to install it\n", id, action.Manager, id)
		return exitLogic
	}

	// 3. Update genv.json.
	f, isNew, err := genvfile.ReadOrNew(*file)
	if err != nil {
		fmt.Fprintf(os.Stderr, "genv: %v\n", err)
		if errors.Is(err, genvfile.ErrInvalidFile) {
			return exitValidation
		}
		return exitIO
	}

	if err := commands.Add(f, id, *version, *prefer, managers); err != nil {
		fmt.Fprintf(os.Stderr, "genv: %v\n", err)
		if errors.Is(err, commands.ErrAlreadyTracked) {
			return exitLogic
		}
		return exitUsage
	}

	if err := genvfile.Write(*file, f); err != nil {
		fmt.Fprintf(os.Stderr, "genv: %v\n", err)
		return exitIO
	}
	if isNew {
		fmt.Fprintf(os.Stdout, "created %s\n", *file)
	}

	// 4. Update lock file.
	lockPath := genvfile.LockPathFrom(*file)
	lf, err := genvfile.ReadLock(lockPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "genv: reading lock: %v\n", err)
		return exitIO
	}
	lf.Packages = append(lf.Packages, genvfile.LockedPackage{
		ID:      action.Pkg.ID,
		Manager: action.Manager,
		PkgName: action.PkgName,
	})
	if err := genvfile.WriteLock(lockPath, lf); err != nil {
		fmt.Fprintf(os.Stderr, "genv: writing lock: %v\n", err)
		return exitIO
	}

	fmt.Fprintf(os.Stdout, "adopted %s — now tracked via %s (already installed)\n", id, action.Manager)
	return exitOK
}

// disownCmd implements `genv disown <id>`.
// Removes the package from genv.json and the lock file without uninstalling it,
// leaving it managed by the underlying package manager directly.
func disownCmd(args []string) int {
	fs := flag.NewFlagSet("disown", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: genv disown <id> [flags]")
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "flags:")
		fs.PrintDefaults()
	}

	file := fs.String("file", defaultSpecPath(), "path to genv.json")

	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	if fs.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "genv disown: missing package id")
		fs.Usage()
		return exitUsage
	}
	id := fs.Arg(0)

	// 1. Update genv.json.
	f, err := genvfile.Read(*file)
	if err != nil {
		if errors.Is(err, genvfile.ErrNotFound) {
			fmt.Fprintf(os.Stderr, "genv: %s not found\n", *file)
			return exitLogic
		}
		fmt.Fprintf(os.Stderr, "genv: %v\n", err)
		if errors.Is(err, genvfile.ErrInvalidFile) {
			return exitValidation
		}
		return exitIO
	}

	if err := commands.Remove(f, id); err != nil {
		fmt.Fprintf(os.Stderr, "genv: %v\n", err)
		return exitLogic
	}

	if err := genvfile.Write(*file, f); err != nil {
		fmt.Fprintf(os.Stderr, "genv: %v\n", err)
		return exitIO
	}

	// 2. Remove from lock file without uninstalling.
	lockPath := genvfile.LockPathFrom(*file)
	lf, err := genvfile.ReadLock(lockPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "genv: reading lock: %v\n", err)
		return exitIO
	}

	wasTracked := false
	remaining := make([]genvfile.LockedPackage, 0, len(lf.Packages))
	for i := range lf.Packages {
		if lf.Packages[i].ID == id {
			wasTracked = true
		} else {
			remaining = append(remaining, lf.Packages[i])
		}
	}
	lf.Packages = remaining
	if err := genvfile.WriteLock(lockPath, lf); err != nil {
		fmt.Fprintf(os.Stderr, "genv: writing lock: %v\n", err)
		return exitIO
	}

	if wasTracked {
		fmt.Fprintf(os.Stdout, "disowned %s — removed from tracking (package remains installed)\n", id)
	} else {
		fmt.Fprintf(os.Stdout, "disowned %s — removed from spec (was not in lock)\n", id)
	}
	return exitOK
}

// listCmd implements `genv list`.
// Lists all packages currently tracked in the lock file (i.e. installed by genv).
func listCmd(args []string) int {
	fs := flag.NewFlagSet("list", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: genv list [flags]")
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "flags:")
		fs.PrintDefaults()
	}

	file := fs.String("file", defaultSpecPath(), "path to genv.json")

	if err := fs.Parse(args); err != nil {
		return exitUsage
	}

	lf, err := genvfile.ReadLock(genvfile.LockPathFrom(*file))
	if err != nil {
		fmt.Fprintf(os.Stderr, "genv: %v\n", err)
		return exitIO
	}

	if len(lf.Packages) == 0 {
		fmt.Fprintln(os.Stdout, "no packages installed by genv.")
		return exitOK
	}

	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tMANAGER\tPACKAGE NAME")
	for _, p := range lf.Packages {
		fmt.Fprintf(tw, "%s\t%s\t%s\n", p.ID, p.Manager, p.PkgName)
	}
	tw.Flush()
	return exitOK
}

// applyCmd implements `genv apply [--dry-run] [--strict] [--yes] [--json] [--timeout] [--debug]`.
// Reconciles the system against genv.json by installing added packages and
// removing packages that were deleted from the spec since the last apply.
func applyCmd(args []string) int {
	fs := flag.NewFlagSet("apply", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: genv apply [flags]")
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "flags:")
		fs.PrintDefaults()
	}

	file := fs.String("file", defaultSpecPath(), "path to genv.json")
	dryRun := fs.Bool("dry-run", false, "print the reconcile plan without executing")
	strict := fs.Bool("strict", false, "exit with an error if any package cannot be resolved")
	yes := fs.Bool("yes", false, "skip the confirmation prompt (for CI and scripts)")
	jsonOut := fs.Bool("json", false, "emit machine-readable JSON to stdout instead of human-readable text")
	timeout := fs.Duration("timeout", 0, "per-subprocess timeout, e.g. 5m or 30s (0 means no timeout)")
	debug := fs.Bool("debug", false, "emit debug-level structured logs to stderr")

	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	if *debug {
		logging.Init(true)
	}

	ctx := context.Background()
	if *timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, *timeout)
		defer cancel()
	}

	f, err := genvfile.Read(*file)
	if err != nil {
		if errors.Is(err, genvfile.ErrNotFound) {
			fmt.Fprintf(os.Stderr, "genv: %s not found — run 'genv add' to create it\n", *file)
			return exitIO
		}
		fmt.Fprintf(os.Stderr, "genv: %v\n", err)
		if errors.Is(err, genvfile.ErrInvalidFile) {
			return exitValidation
		}
		return exitIO
	}

	lockPath := genvfile.LockPathFrom(*file)
	lf, err := genvfile.ReadLock(lockPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "genv: reading lock: %v\n", err)
		return exitIO
	}

	available := resolver.Detect()
	result := resolver.Reconcile(f.Packages, lf.Packages, available)

	if *jsonOut {
		// Build plan data directly from the reconcile result.
		planData := buildPlanResult(result)
		if *dryRun {
			return writeJSON(os.Stdout, output.Envelope{
				Command: "apply",
				OK:      true,
				Data:    planData,
			})
		}
		// Execute with subprocess output routed to stderr so stdout stays clean.
		execResult := resolver.ExecuteApply(ctx, result, os.Stdin, os.Stderr, os.Stderr)
		errs := errStrings(execResult.Errors)
		writeLockAfterApply(lockPath, lf, result, execResult)
		installed := make([]string, len(execResult.Installed))
		for i, lp := range execResult.Installed {
			installed[i] = lp.ID
		}
		return writeJSON(os.Stdout, output.Envelope{
			Command: "apply",
			OK:      len(errs) == 0,
			Data: output.ApplyResult{
				Installed:   installed,
				Uninstalled: execResult.Uninstalled,
			},
			Errors: errs,
		})
	}

	toInstall, toRemove, unresolvedCount := resolver.PrintReconcilePlan(result, os.Stdout)

	if toInstall == 0 && toRemove == 0 {
		fmt.Fprintln(os.Stdout, "already up to date.")
		return exitOK
	}

	if unresolvedCount > 0 && *strict {
		fmt.Fprintf(os.Stderr, "genv apply: %d package(s) unresolved; aborting (--strict)\n", unresolvedCount)
		return exitLogic
	}

	if *dryRun {
		return exitOK
	}

	if !*yes && !confirm(fmt.Sprintf("This will install %d and remove %d package(s). Continue? [y/N] ", toInstall, toRemove)) {
		fmt.Fprintln(os.Stdout, "Aborted.")
		return exitOK
	}

	execResult := resolver.ExecuteApply(ctx, result, os.Stdin, os.Stdout, os.Stderr)
	writeLockAfterApply(lockPath, lf, result, execResult)

	if len(execResult.Errors) > 0 {
		for _, e := range execResult.Errors {
			fmt.Fprintf(os.Stderr, "genv apply: %v\n", e)
		}
		return exitLogic
	}

	return exitOK
}

// writeLockAfterApply updates the lock file to reflect what actually succeeded.
// Called from both the JSON and human-readable paths of applyCmd.
func writeLockAfterApply(lockPath string, lf *genvfile.LockFile, result resolver.ReconcileResult, execResult resolver.ApplyExecution) {
	uninstalledSet := make(map[string]bool, len(execResult.Uninstalled))
	for _, id := range execResult.Uninstalled {
		uninstalledSet[id] = true
	}
	newPkgs := make([]genvfile.LockedPackage, 0, len(result.Unchanged)+len(execResult.Installed))
	newPkgs = append(newPkgs, result.Unchanged...)
	newPkgs = append(newPkgs, execResult.Installed...)
	for _, a := range result.ToRemove {
		if !uninstalledSet[a.Pkg.ID] {
			// Removal failed — keep in lock since it's still installed.
			newPkgs = append(newPkgs, genvfile.LockedPackage{
				ID:      a.Pkg.ID,
				Manager: a.Manager,
				PkgName: a.PkgName,
			})
		}
	}
	lf.Packages = newPkgs
	if err := genvfile.WriteLock(lockPath, lf); err != nil {
		fmt.Fprintf(os.Stderr, "genv: writing lock: %v\n", err)
	}
}

// buildPlanResult converts a ReconcileResult into the stable JSON PlanResult type.
func buildPlanResult(result resolver.ReconcileResult) output.PlanResult {
	toInstall := make([]output.PlanPackage, 0, len(result.ToInstall))
	var unresolved int
	for _, a := range result.ToInstall {
		if a.Resolved() {
			toInstall = append(toInstall, output.PlanPackage{
				ID:      a.Pkg.ID,
				Manager: a.Manager,
				Cmd:     strings.Join(a.Cmd, " "),
			})
		} else {
			unresolved++
			toInstall = append(toInstall, output.PlanPackage{ID: a.Pkg.ID})
		}
	}
	toRemove := make([]output.PlanPackage, 0, len(result.ToRemove))
	for _, a := range result.ToRemove {
		toRemove = append(toRemove, output.PlanPackage{
			ID:      a.Pkg.ID,
			Manager: a.Manager,
			Cmd:     strings.Join(a.UninstallCmd, " "),
		})
	}
	unchanged := make([]output.PlanPackage, 0, len(result.Unchanged))
	for _, lp := range result.Unchanged {
		unchanged = append(unchanged, output.PlanPackage{ID: lp.ID, Manager: lp.Manager})
	}
	return output.PlanResult{
		ToInstall:  toInstall,
		ToRemove:   toRemove,
		Unchanged:  unchanged,
		Unresolved: unresolved,
	}
}

// writeJSON serialises env to w and returns an exit code.
func writeJSON(w *os.File, env output.Envelope) int {
	if err := output.Write(w, env); err != nil {
		fmt.Fprintf(os.Stderr, "genv: writing JSON: %v\n", err)
		return exitIO
	}
	if !env.OK {
		return exitLogic
	}
	return exitOK
}

// errStrings converts a slice of errors to a slice of strings.
func errStrings(errs []error) []string {
	if len(errs) == 0 {
		return nil
	}
	s := make([]string, len(errs))
	for i, e := range errs {
		s[i] = e.Error()
	}
	return s
}

// scanCmd implements `genv scan`.
// Discovers all packages currently installed via available package managers and
// bulk-adopts them into genv.json and the lock file. Packages already tracked
// are skipped. Duplicate names discovered across multiple managers are
// deduplicated — the first adapter in registry order wins.
func scanCmd(args []string) int {
	fs := flag.NewFlagSet("scan", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: genv scan [flags]")
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "Discover all installed packages and adopt them into genv.json.")
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "flags:")
		fs.PrintDefaults()
	}

	file := fs.String("file", defaultSpecPath(), "path to genv.json")
	jsonOut := fs.Bool("json", false, "emit machine-readable JSON to stdout instead of human-readable text")
	debug := fs.Bool("debug", false, "emit debug-level structured logs to stderr")

	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	if *debug {
		logging.Init(true)
	}

	available := resolver.Detect()
	if len(available) == 0 {
		if *jsonOut {
			return writeJSON(os.Stdout, output.Envelope{
				Command: "scan",
				OK:      true,
				Data:    output.ScanResult{Added: 0, Skipped: 0},
			})
		}
		fmt.Fprintln(os.Stdout, "no supported package managers detected.")
		return exitOK
	}

	f, isNew, err := genvfile.ReadOrNew(*file)
	if err != nil {
		fmt.Fprintf(os.Stderr, "genv: %v\n", err)
		if errors.Is(err, genvfile.ErrInvalidFile) {
			return exitValidation
		}
		return exitIO
	}

	lockPath := genvfile.LockPathFrom(*file)
	lf, err := genvfile.ReadLock(lockPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "genv: reading lock: %v\n", err)
		return exitIO
	}

	// Build sets of already-tracked IDs so we can skip them.
	trackedInSpec := make(map[string]bool, len(f.Packages))
	for _, p := range f.Packages {
		trackedInSpec[p.ID] = true
	}

	// Deduplicate across managers using a seen set.
	seen := make(map[string]bool)
	var added int
	var skipped int

	for _, a := range adapter.All {
		if !available[a.Name()] {
			continue
		}
		pkgs, err := a.ListInstalled()
		if err != nil {
			fmt.Fprintf(os.Stderr, "genv scan: %s: listing packages: %v\n", a.Name(), err)
			continue
		}
		for _, pkgName := range pkgs {
			if seen[pkgName] {
				continue // already handled by a higher-priority manager
			}
			seen[pkgName] = true

			if trackedInSpec[pkgName] {
				skipped++
				continue // already in spec
			}

			// Add to spec.
			if err := commands.Add(f, pkgName, "", "", nil); err != nil {
				// ErrAlreadyTracked can race with trackedInSpec; skip silently.
				skipped++
				continue
			}
			trackedInSpec[pkgName] = true

			// Record in lock with best-effort version capture.
			lp := genvfile.LockedPackage{
				ID:      pkgName,
				Manager: a.Name(),
				PkgName: pkgName,
			}
			if v, err := a.QueryVersion(pkgName); err == nil {
				lp.InstalledVersion = v
			}
			lf.Packages = append(lf.Packages, lp)
			added++
		}
	}

	if added > 0 {
		if err := genvfile.Write(*file, f); err != nil {
			fmt.Fprintf(os.Stderr, "genv: writing spec: %v\n", err)
			return exitIO
		}
		if err := genvfile.WriteLock(lockPath, lf); err != nil {
			fmt.Fprintf(os.Stderr, "genv: writing lock: %v\n", err)
			return exitIO
		}
	}

	if *jsonOut {
		return writeJSON(os.Stdout, output.Envelope{
			Command: "scan",
			OK:      true,
			Data:    output.ScanResult{Added: added, Skipped: skipped},
		})
	}

	if added == 0 && skipped == 0 {
		fmt.Fprintln(os.Stdout, "no packages found.")
		return exitOK
	}
	if isNew && added > 0 {
		fmt.Fprintf(os.Stdout, "created %s\n", *file)
	}
	fmt.Fprintf(os.Stdout, "scan complete: %d added, %d already tracked\n", added, skipped)
	return exitOK
}

// statusCmd implements `genv status [--json] [--debug]`.
// Computes a three-way diff between genv.json, genv.lock.json, and recorded
// version data to surface drift, missing installs, and orphaned lock entries.
// Exits with exitLogic when any drift or extra packages are found, so it can
// be used as a CI gate.
func statusCmd(args []string) int {
	fs := flag.NewFlagSet("status", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: genv status [flags]")
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "Show the diff between genv.json, the lock file, and recorded versions.")
		fmt.Fprintln(os.Stderr, "Note: status compares spec vs lock data — it does not query the live system.")
		fmt.Fprintln(os.Stderr, "Run 'genv apply' to reconcile any differences shown.")
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "flags:")
		fs.PrintDefaults()
	}

	file := fs.String("file", defaultSpecPath(), "path to genv.json")
	jsonOut := fs.Bool("json", false, "emit machine-readable JSON to stdout instead of human-readable text")
	debug := fs.Bool("debug", false, "emit debug-level structured logs to stderr")

	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	if *debug {
		logging.Init(true)
	}

	f, err := genvfile.Read(*file)
	if err != nil {
		if errors.Is(err, genvfile.ErrNotFound) {
			fmt.Fprintf(os.Stderr, "genv: %s not found — run 'genv add' to create it\n", *file)
			return exitIO
		}
		fmt.Fprintf(os.Stderr, "genv: %v\n", err)
		if errors.Is(err, genvfile.ErrInvalidFile) {
			return exitValidation
		}
		return exitIO
	}

	lf, err := genvfile.ReadLock(genvfile.LockPathFrom(*file))
	if err != nil {
		fmt.Fprintf(os.Stderr, "genv: reading lock: %v\n", err)
		return exitIO
	}

	entries := commands.Status(f, lf)

	if *jsonOut {
		jsonEntries := make([]output.StatusEntry, 0, len(entries))
		var hasDrift bool
		for _, e := range entries {
			jsonEntries = append(jsonEntries, output.StatusEntry{
				ID:               e.ID,
				Manager:          e.Manager,
				Kind:             string(e.Kind),
				SpecVersion:      e.SpecVersion,
				InstalledVersion: e.InstalledVersion,
			})
			if e.Kind == commands.StatusDrift || e.Kind == commands.StatusExtra {
				hasDrift = true
			}
		}
		return writeJSON(os.Stdout, output.Envelope{
			Command: "status",
			OK:      !hasDrift,
			Data:    output.StatusResult{Entries: jsonEntries},
		})
	}

	if len(entries) == 0 {
		fmt.Fprintln(os.Stdout, "nothing tracked.")
		return exitOK
	}

	// Count by kind for the summary line.
	counts := make(map[commands.StatusKind]int)
	for _, e := range entries {
		counts[e.Kind]++
	}
	total := len(entries)
	fmt.Fprintf(os.Stdout, "Status — %d package", total)
	if total != 1 {
		fmt.Fprint(os.Stdout, "s")
	}
	var parts []string
	if n := counts[commands.StatusOK]; n > 0 {
		parts = append(parts, fmt.Sprintf("%d ok", n))
	}
	if n := counts[commands.StatusDrift]; n > 0 {
		parts = append(parts, fmt.Sprintf("%d drift", n))
	}
	if n := counts[commands.StatusMissing]; n > 0 {
		parts = append(parts, fmt.Sprintf("%d missing", n))
	}
	if n := counts[commands.StatusExtra]; n > 0 {
		parts = append(parts, fmt.Sprintf("%d extra", n))
	}
	if len(parts) > 0 {
		fmt.Fprintf(os.Stdout, " (%s)", strings.Join(parts, ", "))
	}
	fmt.Fprintln(os.Stdout)
	fmt.Fprintln(os.Stdout)

	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	for _, e := range entries {
		mgr := e.Manager
		if mgr == "" {
			mgr = "—"
		}
		switch e.Kind {
		case commands.StatusOK:
			v := e.InstalledVersion
			if v == "" {
				v = "*"
			}
			fmt.Fprintf(tw, "  ok\t%s\t%s\t%s\n", e.ID, mgr, v)
		case commands.StatusDrift:
			fmt.Fprintf(tw, "  drift\t%s\t%s\t(spec: %s, installed: %s)\n",
				e.ID, mgr, e.SpecVersion, e.InstalledVersion)
		case commands.StatusMissing:
			note := "(in spec, not in lock — run 'genv apply')"
			fmt.Fprintf(tw, "  missing\t%s\t%s\t%s\n", e.ID, mgr, note)
		case commands.StatusExtra:
			note := "(in lock, not in spec — run 'genv apply' or 'genv disown')"
			fmt.Fprintf(tw, "  extra\t%s\t%s\t%s\n", e.ID, mgr, note)
		}
	}
	tw.Flush()

	if counts[commands.StatusDrift] > 0 || counts[commands.StatusExtra] > 0 {
		return exitLogic
	}
	return exitOK
}

// cleanCmd implements `genv clean`.
// Runs each available package manager's cache-clean commands.
func cleanCmd(args []string) int {
	fs := flag.NewFlagSet("clean", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: genv clean [flags]")
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "flags:")
		fs.PrintDefaults()
	}
	dryRun := fs.Bool("dry-run", false, "print the clean commands without executing")
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}

	availableNames := resolver.Detect()
	if len(availableNames) == 0 {
		fmt.Fprintln(os.Stdout, "no supported package managers detected.")
		return exitOK
	}

	exitCode := exitOK
	for _, mgr := range adapter.All {
		if !availableNames[mgr.Name()] {
			continue
		}
		cmds := mgr.PlanClean()
		if len(cmds) == 0 {
			continue
		}
		fmt.Fprintf(os.Stdout, "\n[%s]\n", mgr.Name())
		for _, cleanCmd := range cmds {
			fmt.Fprintf(os.Stdout, "==> %s\n", strings.Join(cleanCmd, " "))
			if *dryRun {
				continue
			}
			c := exec.Command(cleanCmd[0], cleanCmd[1:]...)
			c.Stdin = os.Stdin
			c.Stdout = os.Stdout
			c.Stderr = os.Stderr
			if err := c.Run(); err != nil {
				fmt.Fprintf(os.Stderr, "genv clean: %s: %v\n", mgr.Name(), err)
				exitCode = exitLogic
			}
		}
	}
	return exitCode
}

// editCmd implements `genv edit`.
// Opens genv.json in the user's preferred editor ($VISUAL, $EDITOR, or vi).
func editCmd(args []string) int {
	fs := flag.NewFlagSet("edit", flag.ContinueOnError)
	file := fs.String("file", defaultSpecPath(), "path to genv.json")
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}

	editor := os.Getenv("VISUAL")
	if editor == "" {
		editor = os.Getenv("EDITOR")
	}
	if editor == "" {
		editor = "vi"
	}

	cmd := exec.Command(editor, *file)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "genv edit: %v\n", err)
		return exitLogic
	}
	return exitOK
}

// extractPositional separates the first non-flag argument (the package id)
// from the flag arguments, so flags work in any position relative to the id.
// Handles both "--flag value" and "--flag=value" forms.
func extractPositional(args []string) (positional string, flagArgs []string) {
	i := 0
	for i < len(args) {
		arg := args[i]
		if strings.HasPrefix(arg, "-") {
			flagArgs = append(flagArgs, arg)
			// "--flag=value" carries its value inline; no extra arg to consume.
			if !strings.Contains(arg, "=") && i+1 < len(args) {
				i++
				flagArgs = append(flagArgs, args[i])
			}
		} else if positional == "" {
			positional = arg
		}
		i++
	}
	return
}

// parseManagerFlag parses a comma-separated "mgr:name" list into a map.
// An empty input returns nil, nil.
func parseManagerFlag(s string) (map[string]string, error) {
	if s == "" {
		return nil, nil
	}
	result := make(map[string]string)
	for _, token := range strings.Split(s, ",") {
		token = strings.TrimSpace(token)
		if token == "" {
			continue
		}
		parts := strings.SplitN(token, ":", 2)
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			return nil, fmt.Errorf("invalid format %q; expected mgr:name", token)
		}
		result[parts[0]] = parts[1]
	}
	if len(result) == 0 {
		return nil, nil
	}
	return result, nil
}

func printUsage() {
	fmt.Fprint(os.Stderr, `genv — global package manager

Usage:
  genv <command> [flags]

Commands:
  add <id>    Add a package to the spec and install it now
  remove <id> Remove a package from the spec and uninstall it now  (alias: rm)
  adopt <id>  Track an already-installed package in genv.json without reinstalling
  disown <id> Stop tracking a package in genv.json without uninstalling it
  list        List all packages installed by genv                   (alias: ls)
  apply       Reconcile system state with genv.json (install added, remove deleted)
  scan        Discover all installed packages and bulk-adopt them into genv.json
  status      Show diff between genv.json, the lock file, and recorded versions
  clean       Clear the cache of all detected package managers
  edit        Open genv.json in $EDITOR
  version     Show genv build version information
  help        Show this help text

Flags common to all commands:
  --file <path>   Path to genv.json (default: $XDG_CONFIG_HOME/genv/genv.json or ~/.config/genv/genv.json, falling back to ./genv.json)

Add/Adopt-specific flags:
  --version <ver>              Version constraint, e.g. "0.10.*"
  --prefer <mgr>               Preferred manager, e.g. brew
  --manager <mgr:name,...>     Manager-specific package names, e.g.
                               flatpak:org.mozilla.firefox,brew:firefox

Apply-specific flags:
  --dry-run            Print the reconcile plan without executing
  --strict             Exit with an error if any package cannot be resolved
  --yes                Skip the confirmation prompt (for CI and scripts)
  --json               Emit machine-readable JSON to stdout
  --timeout <duration> Per-subprocess timeout, e.g. 5m or 30s (0 = none)
  --debug              Emit debug-level structured logs to stderr

Status-specific flags:
  --json    Emit machine-readable JSON to stdout
  --debug   Emit debug-level structured logs to stderr

Scan-specific flags:
  --json    Emit machine-readable JSON to stdout
  --debug   Emit debug-level structured logs to stderr

Clean-specific flags:
  --dry-run   Print the clean commands without executing

Exit codes:
  0  success (status: all ok or missing only)
  1  bad arguments or unknown command
  2  filesystem or serialisation error
  3  genv.json fails schema validation
  4  semantic error — also returned by 'genv status' when drift or extra entries exist

`)
	fmt.Fprintf(os.Stderr, "Supported package managers:\n  %s\n", commands.KnownManagerList())
}

func printVersion() {
	fmt.Fprintf(os.Stdout, "genv %s\n", version)
	fmt.Fprintf(os.Stdout, "commit: %s\n", commit)
	fmt.Fprintf(os.Stdout, "built:  %s\n", date)
}
