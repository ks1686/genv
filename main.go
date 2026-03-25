package main

import (
	"bufio"
	"context"
	_ "embed"
	"errors"
	"flag"
	"fmt"
	"io"
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
	"github.com/ks1686/genv/internal/search"
)

//go:embed completions/genv.bash
var completionBash string

//go:embed completions/genv.zsh
var completionZsh string

//go:embed completions/genv.fish
var completionFish string

// Structured exit codes.
const (
	exitOK         = 0 // success
	exitUsage      = 1 // bad arguments or unknown command
	exitIO         = 2 // filesystem or serialization error
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
	case "completion":
		return completionCmd(args[1:])
	case "validate":
		return validateCmd(args[1:])
	case "upgrade":
		return upgradeCmd(args[1:])
	case "init":
		return initCmd(args[1:])
	case "__complete":
		return completeInternalCmd(args[1:])
	case "version", "--version":
		printVersion()
		return exitOK
	case "help", "--help", "-h":
		printUsage()
		return exitOK
	default:
		fprintf(os.Stderr, "genv: unknown command %q\n\nRun 'genv help' for usage.\n", args[0])
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

// fprintf/fPrintln/fprint are write helpers that discard the unactionable
// return values of terminal I/O calls — write errors to stdout/stderr are
// not recoverable in a CLI.
func fprintf(w io.Writer, format string, a ...any) { _, _ = fmt.Fprintf(w, format, a...) }
func fPrintln(w io.Writer, a ...any)               { _, _ = fmt.Fprintln(w, a...) }
func fprint(w io.Writer, a ...any)                 { _, _ = fmt.Fprint(w, a...) }

// confirm writes prompt to stdout and reads a y/Y response from stdin.
// Returns true if the user confirmed.
func confirm(prompt string) bool {
	fprint(os.Stdout, prompt)
	answer, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	answer = strings.TrimSpace(answer)
	return answer == "y" || answer == "Y"
}

// isTerminal reports whether stdin is an interactive terminal.
// Returns false when GENV_NO_INTERACTIVE is set (used to disable interactive
// prompts in tests and CI pipelines without needing a --no-search flag).
func isTerminal() bool {
	if os.Getenv("GENV_NO_INTERACTIVE") != "" {
		return false
	}
	fi, err := os.Stdin.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}

// pickString presents a numbered list of strings and returns the chosen item.
// Returns ("", false) when the user cancels (0), input is invalid, or items is empty.
func pickString(items []string) (string, bool) {
	if len(items) == 0 {
		return "", false
	}
	for i, item := range items {
		fprintf(os.Stdout, "  [%d] %s\n", i+1, item)
	}
	fprintf(os.Stdout, "\nselect [1-%d] or 0 to cancel: ", len(items))
	var choice int
	if _, err := fmt.Fscan(os.Stdin, &choice); err != nil || choice <= 0 || choice > len(items) {
		return "", false
	}
	return items[choice-1], true
}

// pickCandidate presents a numbered list of search candidates and returns the
// one the user selects. Returns nil when the user cancels (0), input is
// invalid, or candidates is empty.
func pickCandidate(id string, candidates []search.Candidate) *search.Candidate {
	if len(candidates) == 0 {
		return nil
	}
	fprintf(os.Stdout, "multiple packages match %q — select one to install:\n\n", id)
	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	for i, c := range candidates {
		fmt.Fprintf(tw, "  [%d]\t%s:\t%s\n", i+1, c.Manager, c.PkgName)
	}
	_ = tw.Flush()
	fprintf(os.Stdout, "\nselect [1-%d] or 0 to cancel: ", len(candidates))
	var choice int
	if _, err := fmt.Fscan(os.Stdin, &choice); err != nil || choice <= 0 || choice > len(candidates) {
		return nil
	}
	c := candidates[choice-1]
	return &c
}

// addToSpec reads or creates the spec at file, records the package, and writes
// it back. Prints "created <file>" when the file is brand-new. Returns an exit
// code; exitOK means success.
func addToSpec(file, id, version, prefer string, managers map[string]string) int {
	f, isNew, err := genvfile.ReadOrNew(file)
	if err != nil {
		fprintf(os.Stderr, "genv: %v\n", err)
		if errors.Is(err, genvfile.ErrInvalidFile) {
			return exitValidation
		}
		return exitIO
	}
	if err := commands.Add(f, id, version, prefer, managers); err != nil {
		fprintf(os.Stderr, "genv: %v\n", err)
		if errors.Is(err, commands.ErrAlreadyTracked) {
			return exitLogic
		}
		return exitUsage
	}
	if err := genvfile.Write(file, f); err != nil {
		fprintf(os.Stderr, "genv: %v\n", err)
		return exitIO
	}
	if isNew {
		fprintf(os.Stdout, "created %s\n", file)
	}
	return exitOK
}

// appendLockEntry reads the lock at lockPath, appends lp, and writes it back.
// Returns an exit code; exitOK means success.
func appendLockEntry(lockPath string, lp genvfile.LockedPackage) int {
	lf, err := genvfile.ReadLock(lockPath)
	if err != nil {
		fprintf(os.Stderr, "genv: reading lock: %v\n", err)
		return exitIO
	}
	lf.Packages = append(lf.Packages, lp)
	if err := genvfile.WriteLock(lockPath, lf); err != nil {
		fprintf(os.Stderr, "genv: writing lock: %v\n", err)
		return exitIO
	}
	return exitOK
}

// removeFromSpecAndReadLock reads the spec at file, removes id from it, writes
// it back, then reads and returns the lock file. Returns the lock, the lock
// path, and an exit code. exitOK means all steps succeeded.
func removeFromSpecAndReadLock(file, id string) (*genvfile.LockFile, string, int) {
	f, err := genvfile.Read(file)
	if err != nil {
		if errors.Is(err, genvfile.ErrNotFound) {
			fprintf(os.Stderr, "genv: %s not found\n", file)
			return nil, "", exitLogic
		}
		fprintf(os.Stderr, "genv: %v\n", err)
		if errors.Is(err, genvfile.ErrInvalidFile) {
			return nil, "", exitValidation
		}
		return nil, "", exitIO
	}
	if err := commands.Remove(f, id); err != nil {
		fprintf(os.Stderr, "genv: %v\n", err)
		return nil, "", exitLogic
	}
	if err := genvfile.Write(file, f); err != nil {
		fprintf(os.Stderr, "genv: %v\n", err)
		return nil, "", exitIO
	}
	lockPath := genvfile.LockPathFrom(file)
	lf, err := genvfile.ReadLock(lockPath)
	if err != nil {
		fprintf(os.Stderr, "genv: reading lock: %v\n", err)
		return nil, "", exitIO
	}
	return lf, lockPath, exitOK
}

// addCmd implements `genv add <id> [flags]`.
// Adds the package to genv.json and immediately installs it, then updates the lock.
func addCmd(args []string) int {
	fs := flag.NewFlagSet("add", flag.ContinueOnError)
	fs.Usage = func() {
		fPrintln(os.Stderr, "usage: genv add <id> [flags]")
		fPrintln(os.Stderr)
		fPrintln(os.Stderr, "flags:")
		fs.PrintDefaults()
	}

	file := fs.String("file", defaultSpecPath(), "path to genv.json")
	version := fs.String("version", "", `version constraint, e.g. "0.10.*" (default: omitted, meaning any)`)
	prefer := fs.String("prefer", "", "preferred package manager (e.g. brew)")
	managerFlag := fs.String("manager", "", `manager-specific names, comma-separated mgr:name pairs (e.g. flatpak:org.mozilla.firefox,brew:firefox)`)
	noSearch := fs.Bool("no-search", false, "skip interactive package search and use id as-is")

	id, flagArgs := extractPositional(args)
	if err := fs.Parse(flagArgs); err != nil {
		return exitUsage
	}
	if id == "" {
		fPrintln(os.Stderr, "genv add: missing package id")
		fs.Usage()
		return exitUsage
	}

	managers, err := parseManagerFlag(*managerFlag)
	if err != nil {
		fprintf(os.Stderr, "genv add: --manager: %v\n", err)
		return exitUsage
	}

	// Detect available managers once; used by both the search picker (step 0)
	// and the resolver (step 2).
	available := resolver.Detect()

	// 0. When no explicit manager mapping is given and stdin is a terminal,
	//    search available package managers and let the user pick a match.
	//    This resolves ambiguous short names (e.g. "firefox" → flatpak:org.mozilla.firefox).
	if !*noSearch && len(managers) == 0 && *prefer == "" && isTerminal() {
		fPrintln(os.Stdout, "searching available package managers…")
		candidates := search.All(id, available)
		if len(candidates) == 0 {
			fPrintln(os.Stdout, "no packages found matching that name; adding as-is")
		} else {
			choice := pickCandidate(id, candidates)
			if choice == nil {
				fPrintln(os.Stdout, "cancelled")
				return exitOK
			}
			*prefer = choice.Manager
			// Only record a manager override when the concrete name differs from id.
			if choice.PkgName != id {
				managers = map[string]string{choice.Manager: choice.PkgName}
			}
			fPrintln(os.Stdout)
		}
	}

	// 1. Update genv.json.
	if exit := addToSpec(*file, id, *version, *prefer, managers); exit != exitOK {
		return exit
	}

	// 2. Resolve and install the package.
	pkg := schema.Package{ID: id, Version: *version, Prefer: *prefer, Managers: managers}
	action := resolver.ResolveOne(pkg, available)
	if !action.Resolved() {
		fprintf(os.Stdout, "added %s to spec (no manager available to install it now; run 'genv apply' after installing a compatible package manager)\n", id)
		return exitOK
	}

	fprintf(os.Stdout, "added %s — installing via %s\n", id, action.Manager)
	fprintf(os.Stdout, "\n==> %s\n", strings.Join(action.Cmd, " "))
	cmd := exec.Command(action.Cmd[0], action.Cmd[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		// Installation failure is non-fatal: the spec was already updated.
		// The user can run 'genv apply' to retry.
		fprintf(os.Stderr, "genv: installation failed: %v\n", err)
		fPrintln(os.Stderr, "Package was added to spec. Run 'genv apply' to retry.")
		return exitOK
	}

	// 3. Update lock file.
	return appendLockEntry(genvfile.LockPathFrom(*file), genvfile.LockedPackage{
		ID:      action.Pkg.ID,
		Manager: action.Manager,
		PkgName: action.PkgName,
	})
}

// removeCmd implements `genv remove <id>`.
// Removes the package from genv.json and immediately uninstalls it, then updates the lock.
func removeCmd(args []string) int {
	fs := flag.NewFlagSet("remove", flag.ContinueOnError)
	fs.Usage = func() {
		fPrintln(os.Stderr, "usage: genv remove <id> [flags]")
		fPrintln(os.Stderr)
		fPrintln(os.Stderr, "flags:")
		fs.PrintDefaults()
	}

	file := fs.String("file", defaultSpecPath(), "path to genv.json")

	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	if fs.NArg() < 1 {
		fPrintln(os.Stderr, "genv remove: missing package id")
		fs.Usage()
		return exitUsage
	}
	id := fs.Arg(0)

	// 0. When stdin is a terminal and id has no exact match in the spec,
	//    fall back to substring matching so users can type short names
	//    (e.g. "firefox" resolving to a tracked id like "org.mozilla.firefox").
	if isTerminal() {
		if f, err := genvfile.Read(*file); err == nil {
			idLower := strings.ToLower(id)
			exact := false
			var matches []string
			for _, p := range f.Packages {
				if p.ID == id {
					exact = true
					break
				}
				if strings.Contains(strings.ToLower(p.ID), idLower) {
					matches = append(matches, p.ID)
				}
			}
			if !exact {
				switch len(matches) {
				case 0:
					fprintf(os.Stderr, "genv: %q is not tracked\n", id)
					return exitLogic
				case 1:
					id = matches[0]
				default:
					fprintf(os.Stdout, "multiple tracked packages match %q:\n\n", id)
					chosen, ok := pickString(matches)
					if !ok {
						fPrintln(os.Stdout, "cancelled")
						return exitOK
					}
					id = chosen
				}
			}
		}
	}

	// 1. Update genv.json and read lock.
	lf, lockPath, exit := removeFromSpecAndReadLock(*file, id)
	if exit != exitOK {
		return exit
	}

	// 2. Find the package in the lock file to know which manager installed it.
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
		fprintf(os.Stdout, "removed %s from spec (was not installed by genv)\n", id)
		return exitOK
	}

	// 3. Uninstall from the system using the manager recorded in the lock.
	mgr := adapter.ByName(locked.Manager)
	if mgr == nil {
		fprintf(os.Stderr, "genv: adapter %q no longer registered; cannot uninstall — remove manually\n", locked.Manager)
		return exitLogic
	}

	uninstallCmd := mgr.PlanUninstall(locked.PkgName)
	fprintf(os.Stdout, "removed %s from spec — uninstalling via %s\n", id, locked.Manager)
	fprintf(os.Stdout, "\n==> %s\n", strings.Join(uninstallCmd, " "))
	cmd := exec.Command(uninstallCmd[0], uninstallCmd[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	uninstallErr := cmd.Run()
	if uninstallErr != nil {
		fprintf(os.Stderr, "genv: uninstall failed: %v\n", uninstallErr)
		// Still update the lock — the package is removed from the spec.
	}

	// Cache clean.
	for _, cleanCmd := range mgr.PlanClean() {
		fprintf(os.Stdout, "\n==> %s\n", strings.Join(cleanCmd, " "))
		c := exec.Command(cleanCmd[0], cleanCmd[1:]...)
		c.Stdin = os.Stdin
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr
		if err := c.Run(); err != nil {
			fprintf(os.Stderr, "genv: cache clean warning: %v\n", err)
		}
	}

	// 4. Update lock file (remove the entry regardless of uninstall success).
	lf.Packages = remaining
	if err := genvfile.WriteLock(lockPath, lf); err != nil {
		fprintf(os.Stderr, "genv: writing lock: %v\n", err)
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
		fPrintln(os.Stderr, "usage: genv adopt <id> [flags]")
		fPrintln(os.Stderr)
		fPrintln(os.Stderr, "flags:")
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
		fPrintln(os.Stderr, "genv adopt: missing package id")
		fs.Usage()
		return exitUsage
	}

	managers, err := parseManagerFlag(*managerFlag)
	if err != nil {
		fprintf(os.Stderr, "genv adopt: --manager: %v\n", err)
		return exitUsage
	}

	// 1. Resolve to find which manager handles this package.
	available := resolver.Detect()
	pkg := schema.Package{ID: id, Version: *version, Prefer: *prefer, Managers: managers}
	action := resolver.ResolveOne(pkg, available)
	if !action.Resolved() {
		fprintf(os.Stderr, "genv adopt: no available manager for %q — install a compatible package manager first\n", id)
		return exitLogic
	}

	// 2. Verify the package is actually installed.
	mgr := adapter.ByName(action.Manager)
	installed, err := mgr.Query(action.PkgName)
	if err != nil {
		fprintf(os.Stderr, "genv adopt: querying %s: %v\n", action.Manager, err)
		return exitLogic
	}
	if !installed {
		fprintf(os.Stderr, "genv adopt: %q is not installed via %s — use 'genv add %s' to install it\n", id, action.Manager, id)
		return exitLogic
	}

	// 3. Update genv.json.
	if exit := addToSpec(*file, id, *version, *prefer, managers); exit != exitOK {
		return exit
	}

	// 4. Update lock file.
	if exit := appendLockEntry(genvfile.LockPathFrom(*file), genvfile.LockedPackage{
		ID:      action.Pkg.ID,
		Manager: action.Manager,
		PkgName: action.PkgName,
	}); exit != exitOK {
		return exit
	}

	fprintf(os.Stdout, "adopted %s — now tracked via %s (already installed)\n", id, action.Manager)
	return exitOK
}

// disownCmd implements `genv disown <id>`.
// Removes the package from genv.json and the lock file without uninstalling it,
// leaving it managed by the underlying package manager directly.
func disownCmd(args []string) int {
	fs := flag.NewFlagSet("disown", flag.ContinueOnError)
	fs.Usage = func() {
		fPrintln(os.Stderr, "usage: genv disown <id> [flags]")
		fPrintln(os.Stderr)
		fPrintln(os.Stderr, "flags:")
		fs.PrintDefaults()
	}

	file := fs.String("file", defaultSpecPath(), "path to genv.json")

	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	if fs.NArg() < 1 {
		fPrintln(os.Stderr, "genv disown: missing package id")
		fs.Usage()
		return exitUsage
	}
	id := fs.Arg(0)

	// 1. Update genv.json and read lock.
	lf, lockPath, exit := removeFromSpecAndReadLock(*file, id)
	if exit != exitOK {
		return exit
	}

	// 2. Remove from lock file without uninstalling.
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
		fprintf(os.Stderr, "genv: writing lock: %v\n", err)
		return exitIO
	}

	if wasTracked {
		fprintf(os.Stdout, "disowned %s — removed from tracking (package remains installed)\n", id)
	} else {
		fprintf(os.Stdout, "disowned %s — removed from spec (was not in lock)\n", id)
	}
	return exitOK
}

// listCmd implements `genv list`.
// Lists all packages currently tracked in the lock file (i.e. installed by genv).
func listCmd(args []string) int {
	fs := flag.NewFlagSet("list", flag.ContinueOnError)
	fs.Usage = func() {
		fPrintln(os.Stderr, "usage: genv list [flags]")
		fPrintln(os.Stderr)
		fPrintln(os.Stderr, "flags:")
		fs.PrintDefaults()
	}

	file := fs.String("file", defaultSpecPath(), "path to genv.json")

	if err := fs.Parse(args); err != nil {
		return exitUsage
	}

	lf, err := genvfile.ReadLock(genvfile.LockPathFrom(*file))
	if err != nil {
		fprintf(os.Stderr, "genv: %v\n", err)
		return exitIO
	}

	if len(lf.Packages) == 0 {
		fPrintln(os.Stdout, "no packages installed by genv.")
		return exitOK
	}

	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fPrintln(tw, "ID\tMANAGER\tPACKAGE NAME")
	for _, p := range lf.Packages {
		fprintf(tw, "%s\t%s\t%s\n", p.ID, p.Manager, p.PkgName)
	}
	_ = tw.Flush()
	return exitOK
}

// applyCmd implements `genv apply [--dry-run] [--strict] [--yes] [--json] [--timeout] [--debug]`.
// Reconciles the system against genv.json by installing added packages and
// removing packages that were deleted from the spec since the last apply.
func applyCmd(args []string) int {
	fs := flag.NewFlagSet("apply", flag.ContinueOnError)
	fs.Usage = func() {
		fPrintln(os.Stderr, "usage: genv apply [flags]")
		fPrintln(os.Stderr)
		fPrintln(os.Stderr, "flags:")
		fs.PrintDefaults()
	}

	file := fs.String("file", defaultSpecPath(), "path to genv.json")
	dryRun := fs.Bool("dry-run", false, "print the reconcile plan without executing")
	strict := fs.Bool("strict", false, "exit with an error if any package cannot be resolved")
	yes := fs.Bool("yes", false, "skip the confirmation prompt (for CI and scripts)")
	quiet := fs.Bool("quiet", false, "suppress plan output (useful in scripts)")
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
			fprintf(os.Stderr, "genv: %s not found — run 'genv add' to create it\n", *file)
			return exitIO
		}
		fprintf(os.Stderr, "genv: %v\n", err)
		if errors.Is(err, genvfile.ErrInvalidFile) {
			return exitValidation
		}
		return exitIO
	}
	if f == nil {
		return exitIO
	}

	lockPath := genvfile.LockPathFrom(*file)
	lf, err := genvfile.ReadLock(lockPath)
	if err != nil {
		fprintf(os.Stderr, "genv: reading lock: %v\n", err)
		return exitIO
	}

	available := resolver.Detect()
	result := resolver.Reconcile(f.Packages, lf.Packages, available)

	if *jsonOut {
		// Build plan data directly from the reconcile result.
		planData := buildPlanResult(result)
		if *dryRun {
			return writeJSON(os.Stdout, output.Envelope{
				Version: output.SchemaVersion,
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
			Version: output.SchemaVersion,
			Command: "apply",
			OK:      len(errs) == 0,
			Data: output.ApplyResult{
				Installed:   installed,
				Uninstalled: execResult.Uninstalled,
			},
			Errors: errs,
		})
	}

	planOut := io.Writer(os.Stdout)
	if *quiet {
		planOut = io.Discard
	}
	toInstall, toRemove, unresolvedCount := resolver.PrintReconcilePlan(result, planOut)

	if toInstall == 0 && toRemove == 0 {
		if !*quiet {
			fPrintln(os.Stdout, "already up to date.")
		}
		return exitOK
	}

	if unresolvedCount > 0 && *strict {
		fprintf(os.Stderr, "genv apply: %d package(s) unresolved; aborting (--strict)\n", unresolvedCount)
		return exitLogic
	}

	if *dryRun {
		return exitOK
	}

	if !*yes && !confirm(fmt.Sprintf("This will install %d and remove %d package(s). Continue? [y/N] ", toInstall, toRemove)) {
		fPrintln(os.Stdout, "Aborted.")
		return exitOK
	}

	execResult := resolver.ExecuteApply(ctx, result, os.Stdin, os.Stdout, os.Stderr)
	writeLockAfterApply(lockPath, lf, result, execResult)

	if len(execResult.Errors) > 0 {
		for _, e := range execResult.Errors {
			fprintf(os.Stderr, "genv apply: %v\n", e)
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
		fprintf(os.Stderr, "genv: writing lock: %v\n", err)
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

// writeJSON serializes env to w and returns an exit code.
func writeJSON(w *os.File, env output.Envelope) int {
	if err := output.Write(w, env); err != nil {
		fprintf(os.Stderr, "genv: writing JSON: %v\n", err)
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
		fPrintln(os.Stderr, "usage: genv scan [flags]")
		fPrintln(os.Stderr)
		fPrintln(os.Stderr, "Discover all installed packages and adopt them into genv.json.")
		fPrintln(os.Stderr)
		fPrintln(os.Stderr, "flags:")
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
				Version: output.SchemaVersion,
				Command: "scan",
				OK:      true,
				Data:    output.ScanResult{Added: 0, Skipped: 0},
			})
		}
		fPrintln(os.Stdout, "no supported package managers detected.")
		return exitOK
	}

	f, isNew, err := genvfile.ReadOrNew(*file)
	if err != nil {
		fprintf(os.Stderr, "genv: %v\n", err)
		if errors.Is(err, genvfile.ErrInvalidFile) {
			return exitValidation
		}
		return exitIO
	}

	lockPath := genvfile.LockPathFrom(*file)
	lf, err := genvfile.ReadLock(lockPath)
	if err != nil {
		fprintf(os.Stderr, "genv: reading lock: %v\n", err)
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
			fprintf(os.Stderr, "genv scan: %s: listing packages: %v\n", a.Name(), err)
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
			fprintf(os.Stderr, "genv: writing spec: %v\n", err)
			return exitIO
		}
		if err := genvfile.WriteLock(lockPath, lf); err != nil {
			fprintf(os.Stderr, "genv: writing lock: %v\n", err)
			return exitIO
		}
	}

	if *jsonOut {
		return writeJSON(os.Stdout, output.Envelope{
			Version: output.SchemaVersion,
			Command: "scan",
			OK:      true,
			Data:    output.ScanResult{Added: added, Skipped: skipped},
		})
	}

	if added == 0 && skipped == 0 {
		fPrintln(os.Stdout, "no packages found.")
		return exitOK
	}
	if isNew && added > 0 {
		fprintf(os.Stdout, "created %s\n", *file)
	}
	fprintf(os.Stdout, "scan complete: %d added, %d already tracked\n", added, skipped)
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
		fPrintln(os.Stderr, "usage: genv status [flags]")
		fPrintln(os.Stderr)
		fPrintln(os.Stderr, "Show the diff between genv.json, the lock file, and recorded versions.")
		fPrintln(os.Stderr, "Note: status compares spec vs lock data — it does not query the live system.")
		fPrintln(os.Stderr, "Run 'genv apply' to reconcile any differences shown.")
		fPrintln(os.Stderr)
		fPrintln(os.Stderr, "flags:")
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
			fprintf(os.Stderr, "genv: %s not found — run 'genv add' to create it\n", *file)
			return exitIO
		}
		fprintf(os.Stderr, "genv: %v\n", err)
		if errors.Is(err, genvfile.ErrInvalidFile) {
			return exitValidation
		}
		return exitIO
	}

	lf, err := genvfile.ReadLock(genvfile.LockPathFrom(*file))
	if err != nil {
		fprintf(os.Stderr, "genv: reading lock: %v\n", err)
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
			Version: output.SchemaVersion,
			Command: "status",
			OK:      !hasDrift,
			Data:    output.StatusResult{Entries: jsonEntries},
		})
	}

	if len(entries) == 0 {
		fPrintln(os.Stdout, "nothing tracked.")
		return exitOK
	}

	// Count by kind for the summary line.
	counts := make(map[commands.StatusKind]int)
	for _, e := range entries {
		counts[e.Kind]++
	}
	total := len(entries)
	fprintf(os.Stdout, "Status — %d package", total)
	if total != 1 {
		fprint(os.Stdout, "s")
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
		fprintf(os.Stdout, " (%s)", strings.Join(parts, ", "))
	}
	fPrintln(os.Stdout)
	fPrintln(os.Stdout)

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
			fprintf(tw, "  ok\t%s\t%s\t%s\n", e.ID, mgr, v)
		case commands.StatusDrift:
			fprintf(tw, "  drift\t%s\t%s\t(spec: %s, installed: %s)\n",
				e.ID, mgr, e.SpecVersion, e.InstalledVersion)
		case commands.StatusMissing:
			note := "(in spec, not in lock — run 'genv apply')"
			fprintf(tw, "  missing\t%s\t%s\t%s\n", e.ID, mgr, note)
		case commands.StatusExtra:
			note := "(in lock, not in spec — run 'genv apply' or 'genv disown')"
			fprintf(tw, "  extra\t%s\t%s\t%s\n", e.ID, mgr, note)
		}
	}
	_ = tw.Flush()

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
		fPrintln(os.Stderr, "usage: genv clean [flags]")
		fPrintln(os.Stderr)
		fPrintln(os.Stderr, "flags:")
		fs.PrintDefaults()
	}
	dryRun := fs.Bool("dry-run", false, "print the clean commands without executing")
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}

	availableNames := resolver.Detect()
	if len(availableNames) == 0 {
		fPrintln(os.Stdout, "no supported package managers detected.")
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
		fprintf(os.Stdout, "\n[%s]\n", mgr.Name())
		for _, cleanCmd := range cmds {
			fprintf(os.Stdout, "==> %s\n", strings.Join(cleanCmd, " "))
			if *dryRun {
				continue
			}
			c := exec.Command(cleanCmd[0], cleanCmd[1:]...)
			c.Stdin = os.Stdin
			c.Stdout = os.Stdout
			c.Stderr = os.Stderr
			if err := c.Run(); err != nil {
				fprintf(os.Stderr, "genv clean: %s: %v\n", mgr.Name(), err)
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
		fprintf(os.Stderr, "genv edit: %v\n", err)
		return exitLogic
	}
	return exitOK
}

// completionCmd implements `genv completion <shell>`.
// Prints the shell completion script for bash, zsh, or fish to stdout.
func completionCmd(args []string) int {
	fs := flag.NewFlagSet("completion", flag.ContinueOnError)
	fs.Usage = func() {
		fPrintln(os.Stderr, "usage: genv completion <shell>")
		fPrintln(os.Stderr)
		fPrintln(os.Stderr, "  shell   One of: bash, zsh, fish")
		fPrintln(os.Stderr)
		fPrintln(os.Stderr, "examples:")
		fPrintln(os.Stderr, "  genv completion bash >> ~/.bashrc")
		fPrintln(os.Stderr, "  genv completion zsh  > ~/.zsh/completions/_genv")
		fPrintln(os.Stderr, "  genv completion fish > ~/.config/fish/completions/genv.fish")
	}
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	if fs.NArg() < 1 {
		fPrintln(os.Stderr, "genv completion: missing shell argument (bash, zsh, or fish)")
		fs.Usage()
		return exitUsage
	}
	switch fs.Arg(0) {
	case "bash":
		fprint(os.Stdout, completionBash)
	case "zsh":
		fprint(os.Stdout, completionZsh)
	case "fish":
		fprint(os.Stdout, completionFish)
	default:
		fprintf(os.Stderr, "genv completion: unknown shell %q — supported shells are: bash, zsh, fish\n", fs.Arg(0))
		return exitUsage
	}
	return exitOK
}

// completeInternalCmd implements the hidden `genv __complete <topic>` command
// used by shell completion scripts to fetch dynamic candidates at completion
// time. It prints one candidate per line to stdout and exits 0.
//
// Topics:
//   - packages [--file <path>]  — IDs from genv.json (for remove/disown/upgrade)
//   - managers                  — available package manager names (for --prefer)
func completeInternalCmd(args []string) int {
	if len(args) == 0 {
		return exitUsage
	}
	switch args[0] {
	case "packages":
		fs := flag.NewFlagSet("__complete packages", flag.ContinueOnError)
		fs.SetOutput(io.Discard) // silence flag errors during completion
		file := fs.String("file", defaultSpecPath(), "")
		_ = fs.Parse(args[1:])
		f, err := genvfile.Read(*file)
		if err != nil {
			return exitOK // silent: no spec yet is not an error during completion
		}
		for _, p := range f.Packages {
			fPrintln(os.Stdout, p.ID)
		}
	case "managers":
		available := resolver.Detect()
		for _, a := range adapter.All {
			if available[a.Name()] {
				fPrintln(os.Stdout, a.Name())
			}
		}
	default:
		return exitUsage
	}
	return exitOK
}

// validateCmd implements `genv validate`.
// Reads and validates genv.json, exiting 0 on success and 3 on any error.
func validateCmd(args []string) int {
	fs := flag.NewFlagSet("validate", flag.ContinueOnError)
	fs.Usage = func() {
		fPrintln(os.Stderr, "usage: genv validate [flags]")
		fPrintln(os.Stderr)
		fPrintln(os.Stderr, "flags:")
		fs.PrintDefaults()
	}
	file := fs.String("file", defaultSpecPath(), "path to genv.json")
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}

	_, err := genvfile.Read(*file)
	if err != nil {
		if errors.Is(err, genvfile.ErrNotFound) {
			fprintf(os.Stderr, "genv validate: %s not found — run 'genv init' to create one\n", *file)
			return exitValidation
		}
		fprintf(os.Stderr, "genv validate: %v\n", err)
		return exitValidation
	}
	fprintf(os.Stdout, "%s is valid.\n", *file)
	return exitOK
}

// upgradeCmd implements `genv upgrade [--dry-run] [--yes] [--debug]`.
// Upgrades all packages tracked in the lock file using their recorded manager.
func upgradeCmd(args []string) int {
	fs := flag.NewFlagSet("upgrade", flag.ContinueOnError)
	fs.Usage = func() {
		fPrintln(os.Stderr, "usage: genv upgrade [flags]")
		fPrintln(os.Stderr)
		fPrintln(os.Stderr, "flags:")
		fs.PrintDefaults()
	}
	file := fs.String("file", defaultSpecPath(), "path to genv.json")
	dryRun := fs.Bool("dry-run", false, "print the upgrade commands without executing")
	yes := fs.Bool("yes", false, "skip the confirmation prompt")
	debug := fs.Bool("debug", false, "emit debug-level structured logs to stderr")

	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	if *debug {
		logging.Init(true)
	}

	lockPath := genvfile.LockPathFrom(*file)
	lf, err := genvfile.ReadLock(lockPath)
	if err != nil {
		fprintf(os.Stderr, "genv upgrade: reading lock: %v\n", err)
		return exitIO
	}
	if len(lf.Packages) == 0 {
		fPrintln(os.Stdout, "no packages tracked — run 'genv add' or 'genv scan' first.")
		return exitOK
	}

	// Build upgrade plan, storing each adapter to avoid repeated ByName lookups.
	type upgradeAction struct {
		lp  genvfile.LockedPackage
		mgr adapter.Adapter
		cmd []string
	}
	var plan []upgradeAction
	for _, lp := range lf.Packages {
		mgr := adapter.ByName(lp.Manager)
		if mgr == nil {
			fprintf(os.Stderr, "genv upgrade: adapter %q not registered for %s — skipping\n", lp.Manager, lp.ID)
			continue
		}
		plan = append(plan, upgradeAction{lp: lp, mgr: mgr, cmd: mgr.PlanUpgrade(lp.PkgName)})
	}

	if len(plan) == 0 {
		fPrintln(os.Stdout, "no upgradeable packages found.")
		return exitOK
	}

	fPrintln(os.Stdout, "upgrade plan:")
	for _, a := range plan {
		fprintf(os.Stdout, "  %s  via %s  ==> %s\n", a.lp.ID, a.lp.Manager, strings.Join(a.cmd, " "))
	}

	if *dryRun {
		return exitOK
	}

	if !*yes && !confirm(fmt.Sprintf("\nUpgrade %d package(s)? [y/N] ", len(plan))) {
		fPrintln(os.Stdout, "Aborted.")
		return exitOK
	}

	// Build an ID→index map so each version update is O(1), not O(n).
	lockIndex := make(map[string]int, len(lf.Packages))
	for i, lp := range lf.Packages {
		lockIndex[lp.ID] = i
	}

	exitCode := exitOK
	for _, a := range plan {
		fprintf(os.Stdout, "\n==> %s\n", strings.Join(a.cmd, " "))
		cmd := exec.Command(a.cmd[0], a.cmd[1:]...)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			fprintf(os.Stderr, "genv upgrade: %s: %v\n", a.lp.ID, err)
			exitCode = exitLogic
			continue
		}
		// Update InstalledVersion in lock for successfully upgraded packages.
		if v, err := a.mgr.QueryVersion(a.lp.PkgName); err == nil && v != "" {
			if idx, ok := lockIndex[a.lp.ID]; ok {
				lf.Packages[idx].InstalledVersion = v
			}
		}
	}

	if err := genvfile.WriteLock(lockPath, lf); err != nil {
		fprintf(os.Stderr, "genv upgrade: writing lock: %v\n", err)
		return exitIO
	}
	return exitCode
}

// initCmd implements `genv init`.
// Interactively creates a new genv.json by prompting the user for package IDs.
func initCmd(args []string) int {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	fs.Usage = func() {
		fPrintln(os.Stderr, "usage: genv init [flags]")
		fPrintln(os.Stderr)
		fPrintln(os.Stderr, "flags:")
		fs.PrintDefaults()
	}
	file := fs.String("file", defaultSpecPath(), "path to genv.json")
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}

	// Refuse to overwrite an existing valid spec.
	if _, err := genvfile.Read(*file); err == nil {
		fprintf(os.Stderr, "genv init: %s already exists — edit it with 'genv edit' or add packages with 'genv add'\n", *file)
		return exitLogic
	}

	fprintf(os.Stdout, "Creating %s\n\n", *file)
	fPrintln(os.Stdout, "Enter package IDs to track, one per line. Leave blank and press Enter when done.")
	fPrintln(os.Stdout, "(Example: git, vim, curl)")
	fPrintln(os.Stdout)

	f := genvfile.New()
	reader := bufio.NewReader(os.Stdin)
	for {
		fprint(os.Stdout, "  package id (or Enter to finish): ")
		line, _ := reader.ReadString('\n')
		id := strings.TrimSpace(line)
		if id == "" {
			break
		}
		if err := commands.Add(f, id, "", "", nil); err != nil {
			if errors.Is(err, commands.ErrAlreadyTracked) {
				fprintf(os.Stdout, "  (skipping %q — already added)\n", id)
				continue
			}
			fprintf(os.Stderr, "genv init: %v\n", err)
			return exitLogic
		}
		fprintf(os.Stdout, "  added %s\n", id)
	}

	if len(f.Packages) == 0 {
		fPrintln(os.Stdout, "\nNo packages entered. Run 'genv add <id>' to add packages later.")
		// Still write an empty spec so the file exists.
	}

	if err := genvfile.Write(*file, f); err != nil {
		fprintf(os.Stderr, "genv init: %v\n", err)
		return exitIO
	}
	fprintf(os.Stdout, "\ncreated %s with %d package(s).\n", *file, len(f.Packages))
	if len(f.Packages) > 0 {
		fPrintln(os.Stdout, "Run 'genv apply' to install them.")
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
	fprint(os.Stderr, `genv — global environment manager

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
  completion  Print the shell completion script (bash, zsh, or fish)
  validate    Validate genv.json against the schema
  upgrade     Upgrade all tracked packages to their latest versions
  init        Create a new genv.json interactively
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
  --quiet              Suppress plan output (useful in scripts)
  --json               Emit machine-readable JSON to stdout
  --timeout <duration> Per-subprocess timeout, e.g. 5m or 30s (0 = none)
  --debug              Emit debug-level structured logs to stderr

Upgrade-specific flags:
  --dry-run   Print the upgrade commands without executing
  --yes       Skip the confirmation prompt
  --debug     Emit debug-level structured logs to stderr

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
  2  filesystem or serialization error
  3  genv.json fails schema validation
  4  semantic error — also returned by 'genv status' when drift or extra entries exist

`)
	fprintf(os.Stderr, "Supported package managers:\n  %s\n", commands.KnownManagerList())
}

func printVersion() {
	fprintf(os.Stdout, "genv %s\n", version)
	fprintf(os.Stdout, "commit: %s\n", commit)
	fprintf(os.Stdout, "built:  %s\n", date)
}
