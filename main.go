package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"text/tabwriter"

	"github.com/ks1686/gpm/internal/adapter"
	"github.com/ks1686/gpm/internal/commands"
	"github.com/ks1686/gpm/internal/gpmfile"
	"github.com/ks1686/gpm/internal/resolver"
	"github.com/ks1686/gpm/internal/schema"
)

// Structured exit codes.
const (
	exitOK         = 0 // success
	exitUsage      = 1 // bad arguments or unknown command
	exitIO         = 2 // filesystem or serialisation error
	exitValidation = 3 // gpm.json fails schema validation
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
	case "version", "--version":
		printVersion()
		return exitOK
	case "help", "--help", "-h":
		printUsage()
		return exitOK
	default:
		fmt.Fprintf(os.Stderr, "gpm: unknown command %q\n\nRun 'gpm help' for usage.\n", args[0])
		return exitUsage
	}
}

// lockPathFrom derives the lock file path from the gpm.json path.
// "gpm.json" → "gpm.lock.json", "custom.json" → "custom.lock.json".
func lockPathFrom(jsonPath string) string {
	return strings.TrimSuffix(jsonPath, ".json") + ".lock.json"
}

// confirm writes prompt to stdout and reads a y/Y response from stdin.
// Returns true if the user confirmed.
func confirm(prompt string) bool {
	fmt.Fprint(os.Stdout, prompt)
	answer, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	answer = strings.TrimSpace(answer)
	return answer == "y" || answer == "Y"
}

// addCmd implements `gpm add <id> [flags]`.
// Adds the package to gpm.json and immediately installs it, then updates the lock.
func addCmd(args []string) int {
	fs := flag.NewFlagSet("add", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: gpm add <id> [flags]")
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "flags:")
		fs.PrintDefaults()
	}

	file := fs.String("file", gpmfile.DefaultPath, "path to gpm.json")
	version := fs.String("version", "", `version constraint, e.g. "0.10.*" (default: omitted, meaning any)`)
	prefer := fs.String("prefer", "", "preferred package manager (e.g. brew)")
	managerFlag := fs.String("manager", "", `manager-specific names, comma-separated mgr:name pairs (e.g. flatpak:org.mozilla.firefox,brew:firefox)`)

	id, flagArgs := extractPositional(args)
	if err := fs.Parse(flagArgs); err != nil {
		return exitUsage
	}
	if id == "" {
		fmt.Fprintln(os.Stderr, "gpm add: missing package id")
		fs.Usage()
		return exitUsage
	}

	managers, err := parseManagerFlag(*managerFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "gpm add: --manager: %v\n", err)
		return exitUsage
	}

	// 1. Update gpm.json.
	f, isNew, err := gpmfile.ReadOrNew(*file)
	if err != nil {
		fmt.Fprintf(os.Stderr, "gpm: %v\n", err)
		if errors.Is(err, gpmfile.ErrInvalidFile) {
			return exitValidation
		}
		return exitIO
	}

	if err := commands.Add(f, id, *version, *prefer, managers); err != nil {
		fmt.Fprintf(os.Stderr, "gpm: %v\n", err)
		if errors.Is(err, commands.ErrAlreadyTracked) {
			return exitLogic
		}
		return exitUsage
	}

	if err := gpmfile.Write(*file, f); err != nil {
		fmt.Fprintf(os.Stderr, "gpm: %v\n", err)
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
		fmt.Fprintf(os.Stdout, "added %s to spec (no manager available to install it now; run 'gpm apply' after installing a compatible package manager)\n", id)
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
		// The user can run 'gpm apply' to retry.
		fmt.Fprintf(os.Stderr, "gpm: install failed: %v\n", err)
		fmt.Fprintln(os.Stderr, "Package was added to spec. Run 'gpm apply' to retry.")
		return exitOK
	}

	// 3. Update lock file.
	lockPath := lockPathFrom(*file)
	lf, err := gpmfile.ReadLock(lockPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "gpm: reading lock: %v\n", err)
		return exitIO
	}
	lf.Packages = append(lf.Packages, gpmfile.LockedPackage{
		ID:      action.Pkg.ID,
		Manager: action.Manager,
		PkgName: action.PkgName,
	})
	if err := gpmfile.WriteLock(lockPath, lf); err != nil {
		fmt.Fprintf(os.Stderr, "gpm: writing lock: %v\n", err)
		return exitIO
	}

	return exitOK
}

// removeCmd implements `gpm remove <id>`.
// Removes the package from gpm.json and immediately uninstalls it, then updates the lock.
func removeCmd(args []string) int {
	fs := flag.NewFlagSet("remove", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: gpm remove <id> [flags]")
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "flags:")
		fs.PrintDefaults()
	}

	file := fs.String("file", gpmfile.DefaultPath, "path to gpm.json")

	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	if fs.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "gpm remove: missing package id")
		fs.Usage()
		return exitUsage
	}
	id := fs.Arg(0)

	// 1. Update gpm.json.
	f, err := gpmfile.Read(*file)
	if err != nil {
		if errors.Is(err, gpmfile.ErrNotFound) {
			fmt.Fprintf(os.Stderr, "gpm: %s not found\n", *file)
			return exitLogic
		}
		fmt.Fprintf(os.Stderr, "gpm: %v\n", err)
		if errors.Is(err, gpmfile.ErrInvalidFile) {
			return exitValidation
		}
		return exitIO
	}

	if err := commands.Remove(f, id); err != nil {
		fmt.Fprintf(os.Stderr, "gpm: %v\n", err)
		return exitLogic
	}

	if err := gpmfile.Write(*file, f); err != nil {
		fmt.Fprintf(os.Stderr, "gpm: %v\n", err)
		return exitIO
	}

	// 2. Find the package in the lock file to know which manager installed it.
	lockPath := lockPathFrom(*file)
	lf, err := gpmfile.ReadLock(lockPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "gpm: reading lock: %v\n", err)
		return exitIO
	}

	var locked *gpmfile.LockedPackage
	remaining := make([]gpmfile.LockedPackage, 0, len(lf.Packages))
	for i := range lf.Packages {
		if lf.Packages[i].ID == id {
			locked = &lf.Packages[i]
		} else {
			remaining = append(remaining, lf.Packages[i])
		}
	}

	if locked == nil {
		// Never installed by gpm — nothing to uninstall on the system.
		fmt.Fprintf(os.Stdout, "removed %s from spec (was not installed by gpm)\n", id)
		return exitOK
	}

	// 3. Uninstall from the system using the manager recorded in the lock.
	mgr := adapter.ByName(locked.Manager)
	if mgr == nil {
		fmt.Fprintf(os.Stderr, "gpm: adapter %q no longer registered; cannot uninstall — remove manually\n", locked.Manager)
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
		fmt.Fprintf(os.Stderr, "gpm: uninstall failed: %v\n", uninstallErr)
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
			fmt.Fprintf(os.Stderr, "gpm: cache clean warning: %v\n", err)
		}
	}

	// 4. Update lock file (remove the entry regardless of uninstall success).
	lf.Packages = remaining
	if err := gpmfile.WriteLock(lockPath, lf); err != nil {
		fmt.Fprintf(os.Stderr, "gpm: writing lock: %v\n", err)
		return exitIO
	}

	if uninstallErr != nil {
		return exitLogic
	}
	return exitOK
}

// adoptCmd implements `gpm adopt <id> [flags]`.
// Verifies the package is already installed on the system and then adds it to
// gpm.json and the lock file without running an install command.
func adoptCmd(args []string) int {
	fs := flag.NewFlagSet("adopt", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: gpm adopt <id> [flags]")
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "flags:")
		fs.PrintDefaults()
	}

	file := fs.String("file", gpmfile.DefaultPath, "path to gpm.json")
	version := fs.String("version", "", `version constraint, e.g. "0.10.*" (default: omitted, meaning any)`)
	prefer := fs.String("prefer", "", "preferred package manager (e.g. brew)")
	managerFlag := fs.String("manager", "", `manager-specific names, comma-separated mgr:name pairs (e.g. flatpak:org.mozilla.firefox,brew:firefox)`)

	id, flagArgs := extractPositional(args)
	if err := fs.Parse(flagArgs); err != nil {
		return exitUsage
	}
	if id == "" {
		fmt.Fprintln(os.Stderr, "gpm adopt: missing package id")
		fs.Usage()
		return exitUsage
	}

	managers, err := parseManagerFlag(*managerFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "gpm adopt: --manager: %v\n", err)
		return exitUsage
	}

	// 1. Resolve to find which manager handles this package.
	available := resolver.Detect()
	pkg := schema.Package{ID: id, Version: *version, Prefer: *prefer, Managers: managers}
	action := resolver.ResolveOne(pkg, available)
	if !action.Resolved() {
		fmt.Fprintf(os.Stderr, "gpm adopt: no available manager for %q — install a compatible package manager first\n", id)
		return exitLogic
	}

	// 2. Verify the package is actually installed.
	mgr := adapter.ByName(action.Manager)
	installed, err := mgr.Query(action.PkgName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "gpm adopt: querying %s: %v\n", action.Manager, err)
		return exitLogic
	}
	if !installed {
		fmt.Fprintf(os.Stderr, "gpm adopt: %q is not installed via %s — use 'gpm add %s' to install it\n", id, action.Manager, id)
		return exitLogic
	}

	// 3. Update gpm.json.
	f, isNew, err := gpmfile.ReadOrNew(*file)
	if err != nil {
		fmt.Fprintf(os.Stderr, "gpm: %v\n", err)
		if errors.Is(err, gpmfile.ErrInvalidFile) {
			return exitValidation
		}
		return exitIO
	}

	if err := commands.Add(f, id, *version, *prefer, managers); err != nil {
		fmt.Fprintf(os.Stderr, "gpm: %v\n", err)
		if errors.Is(err, commands.ErrAlreadyTracked) {
			return exitLogic
		}
		return exitUsage
	}

	if err := gpmfile.Write(*file, f); err != nil {
		fmt.Fprintf(os.Stderr, "gpm: %v\n", err)
		return exitIO
	}
	if isNew {
		fmt.Fprintf(os.Stdout, "created %s\n", *file)
	}

	// 4. Update lock file.
	lockPath := lockPathFrom(*file)
	lf, err := gpmfile.ReadLock(lockPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "gpm: reading lock: %v\n", err)
		return exitIO
	}
	lf.Packages = append(lf.Packages, gpmfile.LockedPackage{
		ID:      action.Pkg.ID,
		Manager: action.Manager,
		PkgName: action.PkgName,
	})
	if err := gpmfile.WriteLock(lockPath, lf); err != nil {
		fmt.Fprintf(os.Stderr, "gpm: writing lock: %v\n", err)
		return exitIO
	}

	fmt.Fprintf(os.Stdout, "adopted %s — now tracked via %s (already installed)\n", id, action.Manager)
	return exitOK
}

// disownCmd implements `gpm disown <id>`.
// Removes the package from gpm.json and the lock file without uninstalling it,
// leaving it managed by the underlying package manager directly.
func disownCmd(args []string) int {
	fs := flag.NewFlagSet("disown", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: gpm disown <id> [flags]")
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "flags:")
		fs.PrintDefaults()
	}

	file := fs.String("file", gpmfile.DefaultPath, "path to gpm.json")

	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	if fs.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "gpm disown: missing package id")
		fs.Usage()
		return exitUsage
	}
	id := fs.Arg(0)

	// 1. Update gpm.json.
	f, err := gpmfile.Read(*file)
	if err != nil {
		if errors.Is(err, gpmfile.ErrNotFound) {
			fmt.Fprintf(os.Stderr, "gpm: %s not found\n", *file)
			return exitLogic
		}
		fmt.Fprintf(os.Stderr, "gpm: %v\n", err)
		if errors.Is(err, gpmfile.ErrInvalidFile) {
			return exitValidation
		}
		return exitIO
	}

	if err := commands.Remove(f, id); err != nil {
		fmt.Fprintf(os.Stderr, "gpm: %v\n", err)
		return exitLogic
	}

	if err := gpmfile.Write(*file, f); err != nil {
		fmt.Fprintf(os.Stderr, "gpm: %v\n", err)
		return exitIO
	}

	// 2. Remove from lock file without uninstalling.
	lockPath := lockPathFrom(*file)
	lf, err := gpmfile.ReadLock(lockPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "gpm: reading lock: %v\n", err)
		return exitIO
	}

	wasTracked := false
	remaining := make([]gpmfile.LockedPackage, 0, len(lf.Packages))
	for i := range lf.Packages {
		if lf.Packages[i].ID == id {
			wasTracked = true
		} else {
			remaining = append(remaining, lf.Packages[i])
		}
	}
	lf.Packages = remaining
	if err := gpmfile.WriteLock(lockPath, lf); err != nil {
		fmt.Fprintf(os.Stderr, "gpm: writing lock: %v\n", err)
		return exitIO
	}

	if wasTracked {
		fmt.Fprintf(os.Stdout, "disowned %s — removed from tracking (package remains installed)\n", id)
	} else {
		fmt.Fprintf(os.Stdout, "disowned %s — removed from spec (was not in lock)\n", id)
	}
	return exitOK
}

// listCmd implements `gpm list`.
// Lists all packages currently tracked in the lock file (i.e. installed by gpm).
func listCmd(args []string) int {
	fs := flag.NewFlagSet("list", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: gpm list [flags]")
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "flags:")
		fs.PrintDefaults()
	}

	file := fs.String("file", gpmfile.DefaultPath, "path to gpm.json")

	if err := fs.Parse(args); err != nil {
		return exitUsage
	}

	lf, err := gpmfile.ReadLock(lockPathFrom(*file))
	if err != nil {
		fmt.Fprintf(os.Stderr, "gpm: %v\n", err)
		return exitIO
	}

	if len(lf.Packages) == 0 {
		fmt.Fprintln(os.Stdout, "no packages installed by gpm.")
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

// applyCmd implements `gpm apply [--dry-run] [--strict]`.
// Reconciles the system against gpm.json by installing added packages and
// removing packages that were deleted from the spec since the last apply.
func applyCmd(args []string) int {
	fs := flag.NewFlagSet("apply", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: gpm apply [flags]")
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "flags:")
		fs.PrintDefaults()
	}

	file := fs.String("file", gpmfile.DefaultPath, "path to gpm.json")
	dryRun := fs.Bool("dry-run", false, "print the reconcile plan without executing")
	strict := fs.Bool("strict", false, "exit with an error if any package cannot be resolved")

	if err := fs.Parse(args); err != nil {
		return exitUsage
	}

	f, err := gpmfile.Read(*file)
	if err != nil {
		if errors.Is(err, gpmfile.ErrNotFound) {
			fmt.Fprintf(os.Stderr, "gpm: %s not found — run 'gpm add' to create it\n", *file)
			return exitIO
		}
		fmt.Fprintf(os.Stderr, "gpm: %v\n", err)
		if errors.Is(err, gpmfile.ErrInvalidFile) {
			return exitValidation
		}
		return exitIO
	}

	lockPath := lockPathFrom(*file)
	lf, err := gpmfile.ReadLock(lockPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "gpm: reading lock: %v\n", err)
		return exitIO
	}

	available := resolver.Detect()
	result := resolver.Reconcile(f.Packages, lf.Packages, available)
	toInstall, toRemove, unresolvedCount := resolver.PrintReconcilePlan(result, os.Stdout)

	if toInstall == 0 && toRemove == 0 {
		fmt.Fprintln(os.Stdout, "already up to date.")
		return exitOK
	}

	if unresolvedCount > 0 && *strict {
		fmt.Fprintf(os.Stderr, "gpm apply: %d package(s) unresolved; aborting (--strict)\n", unresolvedCount)
		return exitLogic
	}

	if *dryRun {
		return exitOK
	}

	if !confirm(fmt.Sprintf("This will install %d and remove %d package(s). Continue? [y/N] ", toInstall, toRemove)) {
		fmt.Fprintln(os.Stdout, "Aborted.")
		return exitOK
	}

	execResult := resolver.ExecuteApply(result, os.Stdin, os.Stdout, os.Stderr)

	// Update lock: unchanged + successfully installed + failed removals.
	uninstalledSet := make(map[string]bool, len(execResult.Uninstalled))
	for _, id := range execResult.Uninstalled {
		uninstalledSet[id] = true
	}
	newPkgs := make([]gpmfile.LockedPackage, 0, len(result.Unchanged)+len(execResult.Installed))
	newPkgs = append(newPkgs, result.Unchanged...)
	newPkgs = append(newPkgs, execResult.Installed...)
	for _, a := range result.ToRemove {
		if !uninstalledSet[a.Pkg.ID] {
			// Removal failed — keep in lock since it's still installed.
			newPkgs = append(newPkgs, gpmfile.LockedPackage{
				ID:      a.Pkg.ID,
				Manager: a.Manager,
				PkgName: a.PkgName,
			})
		}
	}
	lf.Packages = newPkgs
	if err := gpmfile.WriteLock(lockPath, lf); err != nil {
		fmt.Fprintf(os.Stderr, "gpm: writing lock: %v\n", err)
		return exitIO
	}

	if len(execResult.Errors) > 0 {
		for _, e := range execResult.Errors {
			fmt.Fprintf(os.Stderr, "gpm apply: %v\n", e)
		}
		return exitLogic
	}

	return exitOK
}

// cleanCmd implements `gpm clean`.
// Runs each available package manager's cache-clean commands.
func cleanCmd(args []string) int {
	fs := flag.NewFlagSet("clean", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: gpm clean [flags]")
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
				fmt.Fprintf(os.Stderr, "gpm clean: %s: %v\n", mgr.Name(), err)
				exitCode = exitLogic
			}
		}
	}
	return exitCode
}

// editCmd implements `gpm edit`.
// Opens gpm.json in the user's preferred editor ($VISUAL, $EDITOR, or vi).
func editCmd(args []string) int {
	fs := flag.NewFlagSet("edit", flag.ContinueOnError)
	file := fs.String("file", gpmfile.DefaultPath, "path to gpm.json")
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
		fmt.Fprintf(os.Stderr, "gpm edit: %v\n", err)
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
	fmt.Fprint(os.Stderr, `gpm — global package manager

Usage:
  gpm <command> [flags]

Commands:
  add <id>    Add a package to the spec and install it now
  remove <id> Remove a package from the spec and uninstall it now  (alias: rm)
  adopt <id>  Track an already-installed package in gpm.json without reinstalling
  disown <id> Stop tracking a package in gpm.json without uninstalling it
  list        List all packages installed by gpm                   (alias: ls)
  apply       Reconcile system state with gpm.json (install added, remove deleted)
  clean       Clear the cache of all detected package managers
  edit        Open gpm.json in $EDITOR
  version     Show gpm build version information
  help        Show this help text

Flags common to all commands:
  --file <path>   Path to gpm.json (default: ./gpm.json)

Add/Adopt-specific flags:
  --version <ver>              Version constraint, e.g. "0.10.*"
  --prefer <mgr>               Preferred manager, e.g. brew
  --manager <mgr:name,...>     Manager-specific package names, e.g.
                               flatpak:org.mozilla.firefox,brew:firefox

Apply-specific flags:
  --dry-run   Print the reconcile plan without executing
  --strict    Exit with an error if any package cannot be resolved

Clean-specific flags:
  --dry-run   Print the clean commands without executing

`)
	fmt.Fprintf(os.Stderr, "Supported package managers:\n  %s\n", commands.KnownManagerList())
}

func printVersion() {
	fmt.Fprintf(os.Stdout, "gpm %s\n", version)
	fmt.Fprintf(os.Stdout, "commit: %s\n", commit)
	fmt.Fprintf(os.Stdout, "built:  %s\n", date)
}
