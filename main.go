package main

import (
	"bufio"
	"context"
	_ "embed"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/ks1686/genv/internal/adapter"
	"github.com/ks1686/genv/internal/commands"
	genvenv "github.com/ks1686/genv/internal/env"
	"github.com/ks1686/genv/internal/files"
	"github.com/ks1686/genv/internal/genvfile"
	"github.com/ks1686/genv/internal/hooks"
	"github.com/ks1686/genv/internal/host"
	"github.com/ks1686/genv/internal/logging"
	"github.com/ks1686/genv/internal/output"
	"github.com/ks1686/genv/internal/resolver"
	"github.com/ks1686/genv/internal/schema"
	"github.com/ks1686/genv/internal/search"
	"github.com/ks1686/genv/internal/service"
	"github.com/ks1686/genv/internal/shellcfg"
)

func runForegroundCommand(argv []string) error {
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

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

var safeEditors = map[string]bool{
	"vi":    true,
	"vim":   true,
	"nano":  true,
	"emacs": true,
	"code":  true,
}

var safeFlags = map[string]bool{
	"--wait": true,
	"-w":     true,
	"-R":     true,
	"-nw":    true,
	"-n":     true,
}

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
	case "pull":
		return pullCmd(args[1:])
	case "init":
		return initCmd(args[1:])
	case "env":
		return envCmd(args[1:])
	case "shell":
		return shellCmd(args[1:])
	case "service":
		return serviceCmd(args[1:])
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
		_, _ = fmt.Fprintf(tw, "  [%d]\t%s:\t%s\n", i+1, c.Manager, c.PkgName)
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
func lockPathForSpec(file, override string) string {
	if override != "" {
		return override
	}
	return genvfile.LockPathFrom(file)
}

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
func removeFromSpecAndReadLock(file, id, lockFile string) (*genvfile.LockFile, string, int) {
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
	lockPath := lockPathForSpec(file, lockFile)
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
	lockFile := fs.String("lock-file", "", "path to genv lock file")
	version := fs.String("version", "", `version constraint, e.g. "0.10.*" (default: omitted, meaning any)`)
	prefer := fs.String("prefer", "", "preferred package manager (e.g. brew)")
	managerFlag := fs.String("manager", "", `manager-specific names, comma-separated mgr:name pairs (e.g. snap:hello,brew:hello)`)
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
	//    This resolves ambiguous short names (e.g. "hello" → snap:hello).
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
	if err := runForegroundCommand(action.Cmd); err != nil {
		// Installation failure is non-fatal: the spec was already updated.
		// The user can run 'genv apply' to retry.
		fprintf(os.Stderr, "genv: installation failed: %v\n", err)
		fPrintln(os.Stderr, "Package was added to spec. Run 'genv apply' to retry.")
		return exitOK
	}

	// 3. Update lock file.
	return appendLockEntry(lockPathForSpec(*file, *lockFile), genvfile.LockedPackage{
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
	lockFile := fs.String("lock-file", "", "path to genv lock file")

	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	if fs.NArg() < 1 {
		fPrintln(os.Stderr, "genv remove: missing package id")
		fs.Usage()
		return exitUsage
	}
	id := fs.Arg(0)

	return runRemove(*file, id, *lockFile)
}

func runRemove(file, id, lockFile string) int {
	// 0. When stdin is a terminal and id has no exact match in the spec,
	//    fall back to substring matching so users can type short names
	//    (e.g. "firefox" resolving to a tracked id like "org.mozilla.firefox").
	if isTerminal() {
		if f, err := genvfile.Read(file); err == nil {
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
	lf, lockPath, exit := removeFromSpecAndReadLock(file, id, lockFile)
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
	uninstallErr := runForegroundCommand(uninstallCmd)
	if uninstallErr != nil {
		fprintf(os.Stderr, "genv: uninstall failed: %v\n", uninstallErr)
		// Still update the lock — the package is removed from the spec.
	}

	// Cache clean.
	for _, cleanCmd := range mgr.PlanClean() {
		fprintf(os.Stdout, "\n==> %s\n", strings.Join(cleanCmd, " "))
		if err := runForegroundCommand(cleanCmd); err != nil {
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
	lockFile := fs.String("lock-file", "", "path to genv lock file")
	version := fs.String("version", "", `version constraint, e.g. "0.10.*" (default: omitted, meaning any)`)
	prefer := fs.String("prefer", "", "preferred package manager (e.g. brew)")
	managerFlag := fs.String("manager", "", `manager-specific names, comma-separated mgr:name pairs (e.g. snap:hello,brew:hello)`)
	hostFlag := fs.String("host", "", "host name for host-specific records (defaults to $GENV_HOST or os.Hostname())")
	filesOnly := fs.Bool("files", false, "adopt matching files block entries into the lock without changing targets")
	jsonOut := fs.Bool("json", false, "emit machine-readable JSON to stdout instead of human-readable text")

	id := ""
	if hasBoolFlag(args, "files") {
		if err := fs.Parse(args); err != nil {
			return exitUsage
		}
	} else {
		var flagArgs []string
		id, flagArgs = extractPositional(args)
		if err := fs.Parse(flagArgs); err != nil {
			return exitUsage
		}
		if id == "" {
			fPrintln(os.Stderr, "genv adopt: missing package id")
			fs.Usage()
			return exitUsage
		}
	}
	if *filesOnly {
		return adoptFilesCmd(*file, *lockFile, *hostFlag, *jsonOut)
	}

	managers, err := parseManagerFlag(*managerFlag)
	if err != nil {
		fprintf(os.Stderr, "genv adopt: --manager: %v\n", err)
		return exitUsage
	}

	hostName := hostForCommand(*hostFlag)
	slog.Debug("adopt host", "host", hostName)

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
	if exit := appendLockEntry(lockPathForSpec(*file, *lockFile), genvfile.LockedPackage{
		ID:      action.Pkg.ID,
		Manager: action.Manager,
		PkgName: action.PkgName,
	}); exit != exitOK {
		return exit
	}

	fprintf(os.Stdout, "adopted %s — now tracked via %s (already installed)\n", id, action.Manager)
	return exitOK
}

func adoptFilesCmd(file, lockFile, hostFlag string, jsonOut bool) int {
	f, err := genvfile.Read(file)
	if err != nil {
		if errors.Is(err, genvfile.ErrNotFound) {
			fprintf(os.Stderr, "genv: %s not found — run 'genv init' to create it\n", file)
			return exitIO
		}
		fprintf(os.Stderr, "genv: %v\n", err)
		if errors.Is(err, genvfile.ErrInvalidFile) {
			return exitValidation
		}
		return exitIO
	}
	hostName := hostForCommand(hostFlag)
	filtered := host.FilterForHost(f, hostName)
	statusCfg := filesConfigWithResolvedSources(filtered.Files, sourceRootForSpec(file, f))
	res, err := files.Status(statusCfg, hostName)
	if jsonOut {
		errs := []string(nil)
		if err != nil {
			errs = []string{err.Error()}
		}
		code := writeJSON(os.Stdout, output.Envelope{
			Version: output.SchemaVersion,
			Command: "adopt",
			OK:      err == nil && res != nil && res.OK,
			Data:    output.StatusResult{FileEntries: fileStatusEntries(res)},
			Errors:  errs,
		})
		return code
	}
	if err != nil {
		fprintf(os.Stderr, "genv adopt --files: %v\n", err)
		return exitLogic
	}
	if res == nil || !res.OK {
		fPrintln(os.Stdout, "files do not match spec:")
		writeFileStatus(os.Stdout, res)
		return exitLogic
	}

	lockPath := lockPathForSpec(file, lockFile)
	lf, err := genvfile.ReadLock(lockPath)
	if err != nil {
		fprintf(os.Stderr, "genv: reading lock: %v\n", err)
		return exitIO
	}
	lf.Files = mergeLockedFiles(lf.Files, lockedFilesFromSpec(filtered.Files))
	if err := genvfile.WriteLock(lockPath, lf); err != nil {
		fprintf(os.Stderr, "genv: writing lock: %v\n", err)
		return exitIO
	}
	fprintf(os.Stdout, "adopted %d file entry/entries into %s\n", len(lockedFilesFromSpec(filtered.Files)), lockPath)
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
	lockFile := fs.String("lock-file", "", "path to genv lock file")

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
	lf, lockPath, exit := removeFromSpecAndReadLock(*file, id, *lockFile)
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
	lockFile := fs.String("lock-file", "", "path to genv lock file")

	if err := fs.Parse(args); err != nil {
		return exitUsage
	}

	lf, err := genvfile.ReadLock(lockPathForSpec(*file, *lockFile))
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

// hostForCommand resolves the host class for a command. The explicit flag
// takes precedence; otherwise Classify() is used. If Classify() fails, a
// warning is logged and an empty string is returned, which causes all
// non-empty host predicates to be treated as non-matching.
func hostForCommand(hostFlag string) string {
	if hostFlag != "" {
		return hostFlag
	}
	h, err := host.Classify()
	if err != nil {
		slog.Warn("cannot determine host; host-specific records will be skipped", "error", err)
		return ""
	}
	return h
}

// applyCmd implements `genv apply [--dry-run] [--strict] [--yes] [--json] [--timeout] [--debug]`.
// Reconciles the system against genv.json by installing added packages and
// removing packages that were deleted from the spec since the last apply.
type applyOptions struct {
	File     string
	LockFile string
	Host     string
	DryRun   bool
	Strict   bool
	Yes      bool
	Quiet    bool
	JSONOut  bool
	Force    bool
	Timeout  time.Duration
	Debug    bool
}

func applyCmd(args []string) int {
	fs := flag.NewFlagSet("apply", flag.ContinueOnError)
	fs.Usage = func() {
		fPrintln(os.Stderr, "usage: genv apply [flags]")
		fPrintln(os.Stderr)
		fPrintln(os.Stderr, "flags:")
		fs.PrintDefaults()
	}

	opts := applyOptions{}
	fs.StringVar(&opts.File, "file", defaultSpecPath(), "path to genv.json")
	fs.StringVar(&opts.LockFile, "lock-file", "", "path to genv lock file")
	fs.BoolVar(&opts.DryRun, "dry-run", false, "print the reconcile plan without executing")
	fs.BoolVar(&opts.Force, "force", false, "overwrite mismatched managed files")
	fs.BoolVar(&opts.Strict, "strict", false, "exit with an error if any package cannot be resolved")
	fs.BoolVar(&opts.Yes, "yes", false, "skip the confirmation prompt (for CI and scripts)")
	fs.BoolVar(&opts.Quiet, "quiet", false, "suppress plan output (useful in scripts)")
	fs.BoolVar(&opts.JSONOut, "json", false, "emit machine-readable JSON to stdout instead of human-readable text")
	fs.DurationVar(&opts.Timeout, "timeout", 0, "per-subprocess timeout, e.g. 5m or 30s (0 means no timeout)")
	fs.BoolVar(&opts.Debug, "debug", false, "emit debug-level structured logs to stderr")
	fs.StringVar(&opts.Host, "host", "", "host name for host-specific records (defaults to $GENV_HOST or os.Hostname())")

	if err := fs.Parse(args); err != nil {
		return exitUsage
	}

	return runApply(opts)
}

func runApply(opts applyOptions) int {
	if opts.Debug {
		logging.Init(true)
	}

	ctx := context.Background()
	if opts.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, opts.Timeout)
		defer cancel()
	}

	f, err := genvfile.Read(opts.File)
	if err != nil {
		if errors.Is(err, genvfile.ErrNotFound) {
			fprintf(os.Stderr, "genv: %s not found — run 'genv add' to create it\n", opts.File)
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
	f = host.FilterForHost(f, hostForCommand(opts.Host))

	lockPath := lockPathForSpec(opts.File, opts.LockFile)
	lf, err := genvfile.ReadLock(lockPath)
	if err != nil {
		fprintf(os.Stderr, "genv: reading lock: %v\n", err)
		return exitIO
	}

	available := resolver.Detect()
	result := resolver.Reconcile(f.Packages, lf.Packages, available)

	if opts.JSONOut {
		return runApplyJSON(ctx, opts, lockPath, f, lf, result)
	}
	return runApplyText(ctx, opts, lockPath, f, lf, result)
}

func runApplyJSON(ctx context.Context, opts applyOptions, lockPath string, f *schema.GenvFile, lf *genvfile.LockFile, result resolver.ReconcileResult) int {
	planData := buildPlanResult(f, lf, result)
	if opts.DryRun {
		filePlan, filePlanErr := applyFiles(ctx, opts, f, lf)
		planData.Files = filePlanEntries(filePlan)
		if filePlanErr != nil {
			return writeJSON(os.Stdout, output.Envelope{
				Version: output.SchemaVersion,
				Command: "apply",
				OK:      false,
				Data:    planData,
				Errors:  []string{filePlanErr.Error()},
			})
		}
		return writeJSON(os.Stdout, output.Envelope{
			Version: output.SchemaVersion,
			Command: "apply",
			OK:      true,
			Data:    planData,
		})
	}

	execResult := resolver.ExecuteApply(ctx, result, os.Stdin, os.Stderr, os.Stderr)
	errs := errStrings(execResult.Errors)

	envApplied, envRemoved := applyEnvVars(f, lf, false)
	shellApplied, shellRemoved := applyShellCfg(f, lf, false)
	_, _, svcErrs := applyServices(ctx, f, lf, false)
	failedHooks := []string(nil)
	filePlan := &files.ApplyResult{}
	filePlanErr := error(nil)
	if len(errs) == 0 {
		if len(svcErrs) > 0 {
			errs = append(errs, errStrings(svcErrs)...)
		}
	}
	if len(errs) == 0 {
		filePlan, filePlanErr = applyFiles(ctx, opts, f, lf)
		if filePlanErr != nil {
			errs = append(errs, filePlanErr.Error())
		} else {
			failedHooks = runPostApplyHooks(ctx, f, hostForCommand(opts.Host), false)
			errs = append(errs, failedHooks...)
		}
	}
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
			Installed:    installed,
			Uninstalled:  execResult.Uninstalled,
			EnvApplied:   envApplied,
			EnvRemoved:   envRemoved,
			ShellApplied: shellApplied,
			ShellRemoved: shellRemoved,
			FilesApplied: append([]string(nil), filePlan.Created...),
			FilesUpdated: append([]string(nil), filePlan.Updated...),
			FailedHooks:  failedHooks,
		},
		Errors: errs,
	})
}

func runApplyText(ctx context.Context, opts applyOptions, lockPath string, f *schema.GenvFile, lf *genvfile.LockFile, result resolver.ReconcileResult) int {
	planOut := io.Writer(os.Stdout)
	if opts.Quiet {
		planOut = io.Discard
	}
	toInstall, toRemove, unresolvedCount := resolver.PrintReconcilePlan(result, planOut)

	var envChanges int
	for _, e := range genvenv.EnvStatus(f.Env, lf.Env) {
		if e.Kind != genvenv.EnvStatusOK {
			envChanges++
		}
	}
	var shellChanges int
	for _, e := range shellcfg.ShellStatus(f.Shell, lf.Shell) {
		if e.Kind != shellcfg.ShellStatusOK {
			shellChanges++
		}
	}
	var serviceChanges int
	for _, e := range service.ServiceStatus(f.Services, lf.Services) {
		if e.Kind != service.ServiceStatusOK {
			serviceChanges++
		}
	}
	planOpts := opts
	planOpts.DryRun = true
	filePlan, filePlanErr := applyFiles(ctx, planOpts, f, lf)
	fileChanges := 0
	if filePlan != nil {
		fileChanges = len(filePlan.Created) + len(filePlan.Updated) + len(filePlan.Mismatched)
	}
	if filePlanErr != nil {
		fprintf(os.Stderr, "genv apply: %v\n", filePlanErr)
		return exitLogic
	}

	if toInstall == 0 && toRemove == 0 && envChanges == 0 && shellChanges == 0 && serviceChanges == 0 && fileChanges == 0 {
		if !opts.Quiet {
			fPrintln(os.Stdout, "already up to date.")
		}
		return exitOK
	}

	if unresolvedCount > 0 && opts.Strict {
		fprintf(os.Stderr, "genv apply: %d package(s) unresolved; aborting (--strict)\n", unresolvedCount)
		return exitLogic
	}

	if opts.DryRun {
		if envChanges > 0 && !opts.Quiet {
			fprintf(os.Stdout, "env: %d variable(s) to apply\n", envChanges)
		}
		if shellChanges > 0 && !opts.Quiet {
			fprintf(os.Stdout, "shell: %d config entries to apply\n", shellChanges)
		}
		if serviceChanges > 0 && !opts.Quiet {
			fprintf(os.Stdout, "service: %d service(s) to reconcile\n", serviceChanges)
		}
		if fileChanges > 0 && !opts.Quiet {
			fprintf(os.Stdout, "files: %d file entry/entries to reconcile\n", fileChanges)
		}
		return exitOK
	}

	confirmMsg := fmt.Sprintf("This will install %d and remove %d package(s)", toInstall, toRemove)
	if envChanges > 0 {
		confirmMsg += fmt.Sprintf(", apply %d env variable(s)", envChanges)
	}
	if shellChanges > 0 {
		confirmMsg += fmt.Sprintf(", apply %d shell config entry/entries", shellChanges)
	}
	if serviceChanges > 0 {
		confirmMsg += fmt.Sprintf(", reconcile %d service(s)", serviceChanges)
	}
	if fileChanges > 0 {
		confirmMsg += fmt.Sprintf(", reconcile %d file entry/entries", fileChanges)
	}
	confirmMsg += ". Continue? [y/N] "

	if !opts.Yes && !confirm(confirmMsg) {
		fPrintln(os.Stdout, "Aborted.")
		return exitOK
	}

	execResult := resolver.ExecuteApply(ctx, result, os.Stdin, os.Stdout, os.Stderr)

	// Apply env, shell and services (update lf in memory), then write lock once.
	applyEnvVars(f, lf, !opts.Quiet)
	applyShellCfg(f, lf, !opts.Quiet)
	_, _, svcErrs := applyServices(ctx, f, lf, !opts.Quiet)
	var fileErrs []error
	if len(execResult.Errors) == 0 {
		filePlan, filePlanErr = applyFiles(ctx, opts, f, lf)
		if filePlanErr != nil {
			fileErrs = append(fileErrs, filePlanErr)
		}
	}
	var hookErrs []string
	if len(execResult.Errors) == 0 && len(svcErrs) == 0 && len(fileErrs) == 0 {
		hookErrs = runPostApplyHooks(ctx, f, hostForCommand(opts.Host), false)
	}
	writeLockAfterApply(lockPath, lf, result, execResult)

	if len(execResult.Errors) > 0 || len(svcErrs) > 0 || len(fileErrs) > 0 || len(hookErrs) > 0 {
		for _, e := range execResult.Errors {
			fprintf(os.Stderr, "genv apply: %v\n", e)
		}
		for _, e := range svcErrs {
			fprintf(os.Stderr, "genv apply: %v\n", e)
		}
		for _, e := range fileErrs {
			fprintf(os.Stderr, "genv apply: %v\n", e)
		}
		for _, e := range hookErrs {
			fprintf(os.Stderr, "genv apply: %s\n", e)
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

// applyEnvVars writes the managed env fragment, updates lf.Env in memory, and
// returns lists of applied and removed variable names. The caller is responsible
// for persisting the lock file (avoiding a double-write when packages and env
// vars are both applied in the same run).
// If verbose is true, it prints progress lines to stdout.
func applyEnvVars(f *schema.GenvFile, lf *genvfile.LockFile, verbose bool) (applied, removed []string) {
	if len(f.Env) == 0 && len(lf.Env) == 0 {
		return nil, nil
	}

	fragPath, err := genvenv.FragmentPath()
	if err != nil {
		fprintf(os.Stderr, "genv: cannot determine fragment path: %v\n", err)
		return nil, nil
	}

	// Use EnvStatus to determine what changed, avoiding duplicated diff logic.
	for _, e := range genvenv.EnvStatus(f.Env, lf.Env) {
		switch e.Kind {
		case genvenv.EnvStatusMissing, genvenv.EnvStatusModified:
			applied = append(applied, e.Name)
		case genvenv.EnvStatusExtra:
			removed = append(removed, e.Name)
		}
	}

	if err := genvenv.ApplyEnv(fragPath, f.Env, genvenv.RcFiles()); err != nil {
		fprintf(os.Stderr, "genv: writing env fragment: %v\n", err)
		return applied, removed
	}

	if verbose {
		for _, name := range applied {
			fprintf(os.Stdout, "  env: set %s\n", name)
		}
		for _, name := range removed {
			fprintf(os.Stdout, "  env: removed %s\n", name)
		}
		if len(applied) > 0 || len(removed) > 0 {
			fprintf(os.Stdout, "env fragment written to %s\n", fragPath)
		}
	}

	// Update lf.Env in memory; caller writes the lock once.
	newEnv := make([]genvfile.LockedEnvVar, 0, len(f.Env))
	for name, ev := range f.Env {
		newEnv = append(newEnv, genvfile.LockedEnvVar{
			Name:      name,
			Value:     ev.Value,
			Sensitive: ev.Sensitive,
		})
	}
	lf.Env = newEnv

	return applied, removed
}

// buildPlanResult converts a ReconcileResult into the stable JSON PlanResult type.
func buildPlanResult(f *schema.GenvFile, lf *genvfile.LockFile, result resolver.ReconcileResult) output.PlanResult {
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

	var toStart, toStop []string
	for _, e := range service.ServiceStatus(f.Services, lf.Services) {
		switch e.Kind {
		case service.ServiceStatusMissing, service.ServiceStatusModified:
			toStart = append(toStart, e.Name)
		case service.ServiceStatusExtra:
			toStop = append(toStop, e.Name)
		}
	}

	return output.PlanResult{
		ToInstall:       toInstall,
		ToRemove:        toRemove,
		Unchanged:       unchanged,
		Unresolved:      unresolved,
		ServicesToStart: toStart,
		ServicesToStop:  toStop,
	}
}

// toOutputShellEntries converts internal shell status entries to the stable
// JSON output type. hasDrift is true when any entry is modified or extra.
func toOutputShellEntries(entries []shellcfg.ShellStatusEntry) (out []output.ShellStatusEntry, hasDrift bool) {
	out = make([]output.ShellStatusEntry, 0, len(entries))
	for _, e := range entries {
		out = append(out, output.ShellStatusEntry{
			Kind:      string(e.Kind),
			EntryType: e.EntryType,
			Name:      e.Name,
			SpecValue: e.SpecValue,
			LockValue: e.LockValue,
		})
		if e.Kind == shellcfg.ShellStatusModified || e.Kind == shellcfg.ShellStatusExtra || e.Kind == shellcfg.ShellStatusMissing {
			hasDrift = true
		}
	}
	return out, hasDrift
}

// toOutputEnvEntries converts internal env status entries to the stable JSON
// output type, redacting sensitive values. hasDrift is true when any entry
// is modified or extra.
func toOutputEnvEntries(entries []genvenv.EnvStatusEntry) (out []output.EnvStatusEntry, hasDrift bool) {
	out = make([]output.EnvStatusEntry, 0, len(entries))
	for _, e := range entries {
		out = append(out, output.EnvStatusEntry{
			Name:      e.Name,
			Kind:      string(e.Kind),
			SpecValue: commands.RedactValue(e.SpecValue, e.Sensitive),
			LockValue: commands.RedactValue(e.LockValue, e.Sensitive),
			Sensitive: e.Sensitive,
		})
		if e.Kind == genvenv.EnvStatusModified || e.Kind == genvenv.EnvStatusExtra || e.Kind == genvenv.EnvStatusMissing {
			hasDrift = true
		}
	}
	return out, hasDrift
}

// writeShellStatusTable renders a []ShellStatusEntry to w using tabwriter.
// Returns true when any entry has drift (modified or extra).
func writeShellStatusTable(w io.Writer, entries []shellcfg.ShellStatusEntry) (hasDrift bool) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	for _, e := range entries {
		switch e.Kind {
		case shellcfg.ShellStatusOK:
			fprintf(tw, "  ok\t%s\t%s\t%s\n", e.EntryType, e.Name, e.SpecValue)
		case shellcfg.ShellStatusModified:
			hasDrift = true
			fprintf(tw, "  modified\t%s\t%s\t(run 'genv apply' to update)\n", e.EntryType, e.Name)
		case shellcfg.ShellStatusMissing:
			hasDrift = true
			fprintf(tw, "  missing\t%s\t%s\t(in spec, not applied — run 'genv apply')\n", e.EntryType, e.Name)
		case shellcfg.ShellStatusExtra:
			hasDrift = true
			fprintf(tw, "  extra\t%s\t%s\t(in lock, not in spec — run 'genv apply')\n", e.EntryType, e.Name)
		}
	}
	_ = tw.Flush()
	return hasDrift
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

func sourceRootForSpec(file string, f *schema.GenvFile) string {
	if f != nil && f.Repo != nil && f.Repo.URL != "" {
		return expandCLIPath(f.Repo.URL)
	}
	return filepath.Dir(file)
}

func expandCLIPath(path string) string {
	if strings.HasPrefix(path, "~") {
		if home, err := os.UserHomeDir(); err == nil {
			path = home + path[1:]
		}
	}
	return os.Expand(path, os.Getenv)
}

func lockedFilesFromSpec(cfg *schema.FilesConfig) []genvfile.LockedFile {
	if cfg == nil {
		return nil
	}
	locked := make([]genvfile.LockedFile, 0, len(cfg.Links)+len(cfg.Templates)+len(cfg.Dirs))
	for _, l := range cfg.Links {
		mode := l.Mode
		if mode == "" {
			mode = "link"
		}
		locked = append(locked, genvfile.LockedFile{Source: l.Source, Target: l.Target, Mode: mode})
	}
	for _, tmpl := range cfg.Templates {
		locked = append(locked, genvfile.LockedFile{Source: tmpl.Source, Target: tmpl.Target, Mode: "copy"})
	}
	for _, d := range cfg.Dirs {
		locked = append(locked, genvfile.LockedFile{Target: d.Target, Mode: "dir"})
	}
	return locked
}

func mergeLockedFiles(existing, adopted []genvfile.LockedFile) []genvfile.LockedFile {
	merged := append([]genvfile.LockedFile(nil), existing...)
	seen := make(map[genvfile.LockedFile]bool, len(existing)+len(adopted))
	for _, f := range existing {
		seen[f] = true
	}
	for _, f := range adopted {
		if seen[f] {
			continue
		}
		merged = append(merged, f)
		seen[f] = true
	}
	return merged
}

func filesConfigWithResolvedSources(cfg *schema.FilesConfig, sourceRoot string) *schema.FilesConfig {
	if cfg == nil {
		return nil
	}
	out := &schema.FilesConfig{
		Links:     append([]schema.FileLink(nil), cfg.Links...),
		Templates: append([]schema.FileTemplate(nil), cfg.Templates...),
		Dirs:      append([]schema.FileDir(nil), cfg.Dirs...),
	}
	for i := range out.Links {
		out.Links[i].Source = resolveCLISource(sourceRoot, out.Links[i].Source)
	}
	for i := range out.Templates {
		out.Templates[i].Source = resolveCLISource(sourceRoot, out.Templates[i].Source)
	}
	return out
}

func resolveCLISource(sourceRoot, source string) string {
	expanded := expandCLIPath(source)
	if filepath.IsAbs(expanded) || sourceRoot == "" {
		return expanded
	}
	return filepath.Join(sourceRoot, expanded)
}

func hasBoolFlag(args []string, name string) bool {
	long := "--" + name
	for _, arg := range args {
		if arg == long || strings.HasPrefix(arg, long+"=") {
			return true
		}
	}
	return false
}

func writeFileStatus(w io.Writer, res *files.StatusResult) {
	if res == nil {
		return
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	for _, e := range res.Entries {
		if e.Kind == "ok" {
			continue
		}
		fprintf(tw, "  %s\t%s\t%s\n", e.Kind, e.Target, e.Mode)
	}
	_ = tw.Flush()
}

func filePlanEntries(res *files.ApplyResult) []output.FilePlanEntry {
	if res == nil {
		return nil
	}
	entries := make([]output.FilePlanEntry, 0, len(res.Created)+len(res.Updated)+len(res.Skipped)+len(res.Mismatched))
	for _, target := range res.Created {
		entries = append(entries, output.FilePlanEntry{Target: target, Kind: "create"})
	}
	for _, target := range res.Updated {
		entries = append(entries, output.FilePlanEntry{Target: target, Kind: "update"})
	}
	for _, target := range res.Skipped {
		entries = append(entries, output.FilePlanEntry{Target: target, Kind: "ok"})
	}
	for _, target := range res.Mismatched {
		entries = append(entries, output.FilePlanEntry{Target: target, Kind: "mismatch"})
	}
	return entries
}

func fileStatusEntries(res *files.StatusResult) []output.FilePlanEntry {
	if res == nil {
		return nil
	}
	entries := make([]output.FilePlanEntry, 0, len(res.Entries))
	for _, e := range res.Entries {
		entries = append(entries, output.FilePlanEntry{Source: e.Source, Target: e.Target, Mode: e.Mode, Kind: e.Kind})
	}
	return entries
}

func applyFiles(ctx context.Context, opts applyOptions, f *schema.GenvFile, lf *genvfile.LockFile) (*files.ApplyResult, error) {
	res, err := files.Apply(ctx, f.Files, hostForCommand(opts.Host), files.ApplyOptions{
		SourceRoot: sourceRootForSpec(opts.File, f),
		Force:      opts.Force,
		DryRun:     opts.DryRun,
		Backup:     false,
	})
	if err == nil && !opts.DryRun {
		lf.Files = lockedFilesFromSpec(f.Files)
	}
	return res, err
}

func runPostApplyHooks(ctx context.Context, f *schema.GenvFile, hostName string, dryRun bool) []string {
	if f == nil || f.Hooks == nil || len(f.Hooks.PostApply) == 0 {
		return nil
	}
	if err := hooks.NewExecutor(os.Stdout, os.Stderr).PostApply(ctx, f.Hooks.PostApply, hostName, dryRun); err != nil {
		return []string{err.Error()}
	}
	return nil
}

func runUpgradeHooks(ctx context.Context, phase string, f *schema.GenvFile, hostName string, dryRun bool) []string {
	if f == nil || f.Hooks == nil {
		return nil
	}
	exec := hooks.NewExecutor(os.Stdout, os.Stderr)
	var err error
	switch phase {
	case "pre":
		if len(f.Hooks.PreUpgrade) > 0 {
			err = exec.PreUpgrade(ctx, f.Hooks.PreUpgrade, hostName, dryRun)
		}
	case "post":
		if len(f.Hooks.PostUpgrade) > 0 {
			err = exec.PostUpgrade(ctx, f.Hooks.PostUpgrade, hostName, dryRun)
		}
	}
	if err != nil {
		return []string{err.Error()}
	}
	return nil
}

// envCmd implements `genv env <subcommand>`.
// Subcommands: set, unset, list.
func envCmd(args []string) int {
	if len(args) == 0 {
		fPrintln(os.Stderr, "usage: genv env <set|unset|list> [flags]")
		fPrintln(os.Stderr)
		fPrintln(os.Stderr, "subcommands:")
		fPrintln(os.Stderr, "  set <NAME> <value> [--sensitive]   Add or update a variable in the spec")
		fPrintln(os.Stderr, "  unset <NAME>                        Remove a variable from the spec")
		fPrintln(os.Stderr, "  list [--json]                       Show all declared variables")
		return exitUsage
	}
	switch args[0] {
	case "set":
		return envSetCmd(args[1:])
	case "unset":
		return envUnsetCmd(args[1:])
	case "list", "ls":
		return envListCmd(args[1:])
	default:
		fprintf(os.Stderr, "genv env: unknown subcommand %q\n\nRun 'genv env' for usage.\n", args[0])
		return exitUsage
	}
}

// envSetCmd implements `genv env set <NAME> <value> [--sensitive] [--file]`.
func envSetCmd(args []string) int {
	fs := flag.NewFlagSet("env set", flag.ContinueOnError)
	fs.Usage = func() {
		fPrintln(os.Stderr, "usage: genv env set <NAME> <value> [flags]")
		fPrintln(os.Stderr)
		fPrintln(os.Stderr, "flags:")
		fs.PrintDefaults()
	}
	file := fs.String("file", defaultSpecPath(), "path to genv.json")
	sensitive := fs.Bool("sensitive", false, "mark value as sensitive (redacted in output and logs)")

	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	if fs.NArg() < 2 {
		fPrintln(os.Stderr, "genv env set: NAME and value are required")
		fs.Usage()
		return exitUsage
	}
	name := fs.Arg(0)
	value := fs.Arg(1)

	f, isNew, err := genvfile.ReadOrNew(*file)
	if err != nil {
		fprintf(os.Stderr, "genv: %v\n", err)
		if errors.Is(err, genvfile.ErrInvalidFile) {
			return exitValidation
		}
		return exitIO
	}

	if err := commands.EnvSet(f, name, value, *sensitive); err != nil {
		fprintf(os.Stderr, "genv: %v\n", err)
		return exitUsage
	}

	if err := genvfile.Write(*file, f); err != nil {
		fprintf(os.Stderr, "genv: %v\n", err)
		return exitIO
	}
	if isNew {
		fprintf(os.Stdout, "created %s\n", *file)
	}

	fprintf(os.Stdout, "set %s=%s\nRun 'genv apply' to export it to your shell.\n", name, commands.RedactValue(value, *sensitive))
	return exitOK
}

// envUnsetCmd implements `genv env unset <NAME> [--file]`.
func envUnsetCmd(args []string) int {
	fs := flag.NewFlagSet("env unset", flag.ContinueOnError)
	fs.Usage = func() {
		fPrintln(os.Stderr, "usage: genv env unset <NAME> [flags]")
		fPrintln(os.Stderr)
		fPrintln(os.Stderr, "flags:")
		fs.PrintDefaults()
	}
	file := fs.String("file", defaultSpecPath(), "path to genv.json")

	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	if fs.NArg() < 1 {
		fPrintln(os.Stderr, "genv env unset: NAME is required")
		fs.Usage()
		return exitUsage
	}
	name := fs.Arg(0)

	f, err := genvfile.Read(*file)
	if err != nil {
		if errors.Is(err, genvfile.ErrNotFound) {
			fprintf(os.Stderr, "genv: %s not found\n", *file)
			return exitLogic
		}
		fprintf(os.Stderr, "genv: %v\n", err)
		if errors.Is(err, genvfile.ErrInvalidFile) {
			return exitValidation
		}
		return exitIO
	}

	if err := commands.EnvUnset(f, name); err != nil {
		fprintf(os.Stderr, "genv: %v\n", err)
		if errors.Is(err, commands.ErrEnvNotFound) {
			return exitLogic
		}
		return exitUsage
	}

	if err := genvfile.Write(*file, f); err != nil {
		fprintf(os.Stderr, "genv: %v\n", err)
		return exitIO
	}

	fprintf(os.Stdout, "unset %s\nRun 'genv apply' to remove it from your shell.\n", name)
	return exitOK
}

// envListCmd implements `genv env list [--json] [--file]`.
func envListCmd(args []string) int {
	fs := flag.NewFlagSet("env list", flag.ContinueOnError)
	fs.Usage = func() {
		fPrintln(os.Stderr, "usage: genv env list [flags]")
		fPrintln(os.Stderr)
		fPrintln(os.Stderr, "flags:")
		fs.PrintDefaults()
	}
	file := fs.String("file", defaultSpecPath(), "path to genv.json")
	jsonOut := fs.Bool("json", false, "emit machine-readable JSON to stdout")

	if err := fs.Parse(args); err != nil {
		return exitUsage
	}

	f, err := genvfile.Read(*file)
	if err != nil {
		if errors.Is(err, genvfile.ErrNotFound) {
			fprintf(os.Stderr, "genv: %s not found — run 'genv env set' to create it\n", *file)
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

	entries := genvenv.EnvStatus(f.Env, lf.Env)

	if *jsonOut {
		jsonEntries := make([]output.EnvStatusEntry, 0, len(entries))
		for _, e := range entries {
			jsonEntries = append(jsonEntries, output.EnvStatusEntry{
				Name:      e.Name,
				Kind:      string(e.Kind),
				SpecValue: commands.RedactValue(e.SpecValue, e.Sensitive),
				LockValue: commands.RedactValue(e.LockValue, e.Sensitive),
				Sensitive: e.Sensitive,
			})
		}
		return writeJSON(os.Stdout, output.Envelope{
			Version: output.SchemaVersion,
			Command: "env list",
			OK:      true,
			Data:    output.EnvStatusResult{Entries: jsonEntries},
		})
	}

	commands.EnvList(f, os.Stdout)
	return exitOK
}

// shellCmd implements `genv shell <subcommand>`.
func shellCmd(args []string) int {
	if len(args) == 0 {
		fPrintln(os.Stderr, "usage: genv shell <alias|status|edit> [flags]")
		fPrintln(os.Stderr)
		fPrintln(os.Stderr, "subcommands:")
		fPrintln(os.Stderr, "  alias set <name> <value> [--shell bash|zsh|fish]   Add or update an alias")
		fPrintln(os.Stderr, "  alias unset <name>                                 Remove an alias")
		fPrintln(os.Stderr, "  status [--json]                                    Show shell config drift")
		fPrintln(os.Stderr, "  edit                                               Open genv.json in $EDITOR")
		return exitUsage
	}
	switch args[0] {
	case "alias":
		return shellAliasCmd(args[1:])
	case "status":
		return shellStatusCmd(args[1:])
	case "edit":
		return shellEditCmd(args[1:])
	default:
		fprintf(os.Stderr, "genv shell: unknown subcommand %q\n\nRun 'genv shell' for usage.\n", args[0])
		return exitUsage
	}
}

// shellAliasCmd dispatches `genv shell alias set|unset`.
func shellAliasCmd(args []string) int {
	if len(args) == 0 {
		fPrintln(os.Stderr, "usage: genv shell alias <set|unset> [flags]")
		return exitUsage
	}
	switch args[0] {
	case "set":
		return shellAliasSetCmd(args[1:])
	case "unset":
		return shellAliasUnsetCmd(args[1:])
	default:
		fprintf(os.Stderr, "genv shell alias: unknown subcommand %q\n", args[0])
		return exitUsage
	}
}

// shellAliasSetCmd implements `genv shell alias set <name> <value> [--shell] [--file]`.
func shellAliasSetCmd(args []string) int {
	fs := flag.NewFlagSet("shell alias set", flag.ContinueOnError)
	fs.Usage = func() {
		fPrintln(os.Stderr, "usage: genv shell alias set <name> <value> [flags]")
		fPrintln(os.Stderr)
		fPrintln(os.Stderr, "flags:")
		fs.PrintDefaults()
	}
	file := fs.String("file", defaultSpecPath(), "path to genv.json")
	shell := fs.String("shell", "", "target shell: "+schema.ValidShellTargetsMsg)

	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	if fs.NArg() < 2 {
		fPrintln(os.Stderr, "genv shell alias set: name and value are required")
		fs.Usage()
		return exitUsage
	}
	name, value := fs.Arg(0), fs.Arg(1)

	f, isNew, err := genvfile.ReadOrNew(*file)
	if err != nil {
		fprintf(os.Stderr, "genv: %v\n", err)
		if errors.Is(err, genvfile.ErrInvalidFile) {
			return exitValidation
		}
		return exitIO
	}

	if err := commands.ShellAliasSet(f, name, value, *shell); err != nil {
		fprintf(os.Stderr, "genv: %v\n", err)
		return exitUsage
	}

	if err := genvfile.Write(*file, f); err != nil {
		fprintf(os.Stderr, "genv: %v\n", err)
		return exitIO
	}
	if isNew {
		fprintf(os.Stdout, "created %s\n", *file)
	}

	shellNote := ""
	if *shell != "" {
		shellNote = fmt.Sprintf(" (%s only)", *shell)
	}
	fprintf(os.Stdout, "set alias %s=%q%s\nRun 'genv apply' to apply it to your shell.\n", name, value, shellNote)
	return exitOK
}

// shellAliasUnsetCmd implements `genv shell alias unset <name> [--file]`.
func shellAliasUnsetCmd(args []string) int {
	fs := flag.NewFlagSet("shell alias unset", flag.ContinueOnError)
	fs.Usage = func() {
		fPrintln(os.Stderr, "usage: genv shell alias unset <name> [flags]")
		fPrintln(os.Stderr)
		fPrintln(os.Stderr, "flags:")
		fs.PrintDefaults()
	}
	file := fs.String("file", defaultSpecPath(), "path to genv.json")

	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	if fs.NArg() < 1 {
		fPrintln(os.Stderr, "genv shell alias unset: name is required")
		fs.Usage()
		return exitUsage
	}
	name := fs.Arg(0)

	f, err := genvfile.Read(*file)
	if err != nil {
		if errors.Is(err, genvfile.ErrNotFound) {
			fprintf(os.Stderr, "genv: %s not found\n", *file)
			return exitLogic
		}
		fprintf(os.Stderr, "genv: %v\n", err)
		if errors.Is(err, genvfile.ErrInvalidFile) {
			return exitValidation
		}
		return exitIO
	}

	if err := commands.ShellAliasUnset(f, name); err != nil {
		fprintf(os.Stderr, "genv: %v\n", err)
		if errors.Is(err, commands.ErrShellAliasNotFound) {
			return exitLogic
		}
		return exitUsage
	}

	if err := genvfile.Write(*file, f); err != nil {
		fprintf(os.Stderr, "genv: %v\n", err)
		return exitIO
	}

	fprintf(os.Stdout, "unset alias %s\nRun 'genv apply' to remove it from your shell.\n", name)
	return exitOK
}

// shellStatusCmd implements `genv shell status [--json] [--file]`.
func shellStatusCmd(args []string) int {
	fs := flag.NewFlagSet("shell status", flag.ContinueOnError)
	fs.Usage = func() {
		fPrintln(os.Stderr, "usage: genv shell status [flags]")
		fPrintln(os.Stderr)
		fPrintln(os.Stderr, "flags:")
		fs.PrintDefaults()
	}
	file := fs.String("file", defaultSpecPath(), "path to genv.json")
	jsonOut := fs.Bool("json", false, "emit machine-readable JSON to stdout")

	if err := fs.Parse(args); err != nil {
		return exitUsage
	}

	f, err := genvfile.Read(*file)
	if err != nil {
		if errors.Is(err, genvfile.ErrNotFound) {
			fprintf(os.Stderr, "genv: %s not found — run 'genv shell alias set' to create it\n", *file)
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

	entries := shellcfg.ShellStatus(f.Shell, lf.Shell)

	if *jsonOut {
		jsonEntries, hasDrift := toOutputShellEntries(entries)
		return writeJSON(os.Stdout, output.Envelope{
			Version: output.SchemaVersion,
			Command: "shell status",
			OK:      !hasDrift,
			Data:    output.ShellStatusResult{Entries: jsonEntries},
		})
	}

	if len(entries) == 0 {
		fPrintln(os.Stdout, "no shell config declared.")
		return exitOK
	}

	if hasDrift := writeShellStatusTable(os.Stdout, entries); hasDrift {
		return exitLogic
	}
	return exitOK
}

// shellEditCmd implements `genv shell edit [--file]`.
// Opens genv.json in $EDITOR so the user can edit the shell block directly.
// The generated shell fragment (~/.config/genv/shell.sh) is a derivative
// artifact — all shell configuration belongs in genv.json.
func shellEditCmd(args []string) int {
	fs := flag.NewFlagSet("shell edit", flag.ContinueOnError)
	file := fs.String("file", defaultSpecPath(), "path to genv.json")
	fs.Usage = func() {
		fPrintln(os.Stderr, "usage: genv shell edit [flags]")
		fPrintln(os.Stderr)
		fPrintln(os.Stderr, "Open genv.json in $EDITOR to edit the shell block.")
		fPrintln(os.Stderr, "Run 'genv apply' after saving to apply changes.")
		fPrintln(os.Stderr)
		fPrintln(os.Stderr, "flags:")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}

	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vi"
	}

	cmd, err := buildEditorCmd(editor, *file)
	if err != nil {
		fprintf(os.Stderr, "genv shell edit: %v\n", err)
		return exitLogic
	}

	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fprintf(os.Stderr, "genv shell edit: editor exited with error: %v\n", err)
		return exitLogic
	}
	return exitOK
}

// applyShellCfg writes the managed shell fragment, updates lf.Shell in memory,
// and returns lists of applied and removed entry names. The caller writes the lock.
// If verbose is true, it prints progress lines to stdout.
func applyShellCfg(f *schema.GenvFile, lf *genvfile.LockFile, verbose bool) (applied, removed []string) {
	if f.Shell == nil && lf.Shell == nil {
		return nil, nil
	}

	fragPath, err := shellcfg.FragmentPath()
	if err != nil {
		fprintf(os.Stderr, "genv: cannot determine shell fragment path: %v\n", err)
		return nil, nil
	}

	// Use ShellStatus to determine what changed, avoiding duplicated diff logic.
	var hasFishEntries bool
	for _, e := range shellcfg.ShellStatus(f.Shell, lf.Shell) {
		label := e.EntryType + " '" + e.Name + "'"
		switch e.Kind {
		case shellcfg.ShellStatusMissing, shellcfg.ShellStatusModified:
			applied = append(applied, label)
		case shellcfg.ShellStatusExtra:
			removed = append(removed, label)
		}
	}
	// Fish detection is separate: ShellStatus entries don't carry the shell target.
	if f.Shell != nil {
		for _, a := range f.Shell.Aliases {
			if a.Shell == "fish" {
				hasFishEntries = true
				break
			}
		}
		if !hasFishEntries {
			for _, fn := range f.Shell.Functions {
				if fn.Shell == "fish" {
					hasFishEntries = true
					break
				}
			}
		}
	}

	var cfg *schema.ShellConfig
	if f.Shell != nil {
		cfg = f.Shell
	}
	if err := shellcfg.ApplyShell(fragPath, cfg, genvenv.RcFiles()); err != nil {
		fprintf(os.Stderr, "genv: writing shell fragment: %v\n", err)
		return applied, removed
	}

	if verbose {
		for _, name := range applied {
			fprintf(os.Stdout, "  shell: set %s\n", name)
		}
		for _, name := range removed {
			fprintf(os.Stdout, "  shell: removed %s\n", name)
		}
		if len(applied) > 0 || len(removed) > 0 {
			fprintf(os.Stdout, "shell fragment written to %s\n", fragPath)
		}
	}
	if hasFishEntries {
		fprintf(os.Stdout, "note: fish-specific shell entries are not auto-applied.\n")
		fprintf(os.Stdout, "      Add '. %s' to ~/.config/fish/config.fish to source them.\n", fragPath)
	}

	// Update lf.Shell in memory; caller writes the lock once.
	lf.Shell = shellcfg.SpecToLock(f.Shell)

	return applied, removed
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
	lockFile := fs.String("lock-file", "", "path to genv lock file")
	jsonOut := fs.Bool("json", false, "emit machine-readable JSON to stdout instead of human-readable text")
	debug := fs.Bool("debug", false, "emit debug-level structured logs to stderr")

	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	if *debug {
		logging.Init(true)
	}

	f, isNew, err := genvfile.ReadOrNew(*file)
	if err != nil {
		fprintf(os.Stderr, "genv: %v\n", err)
		if errors.Is(err, genvfile.ErrInvalidFile) {
			return exitValidation
		}
		return exitIO
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

	lockPath := lockPathForSpec(*file, *lockFile)
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
		versions := map[string]string(nil)
		var pkgs []string
		if versionLister, ok := a.(adapter.VersionLister); ok {
			if listedVersions, err := versionLister.ListInstalledVersions(); err == nil {
				versions = listedVersions
				for pkgName := range versions {
					pkgs = append(pkgs, pkgName)
				}
				sort.Strings(pkgs)
			}
		}
		if pkgs == nil {
			listed, err := a.ListInstalled()
			if err != nil {
				fprintf(os.Stderr, "genv scan: %s: listing packages: %v\n", a.Name(), err)
				continue
			}
			pkgs = listed
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
			if v, ok := versions[pkgName]; ok {
				lp.InstalledVersion = v
			} else if v, err := a.QueryVersion(pkgName); err == nil {
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
	lockFile := fs.String("lock-file", "", "path to genv lock file")
	jsonOut := fs.Bool("json", false, "emit machine-readable JSON to stdout instead of human-readable text")
	debug := fs.Bool("debug", false, "emit debug-level structured logs to stderr")
	filesOnly := fs.Bool("files", false, "check files block against the live filesystem only")
	hostFlag := fs.String("host", "", "host name for host-specific records (defaults to $GENV_HOST or os.Hostname())")

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
	hostName := hostForCommand(*hostFlag)
	f = host.FilterForHost(f, hostName)

	if *filesOnly {
		statusCfg := filesConfigWithResolvedSources(f.Files, sourceRootForSpec(*file, f))
		res, err := files.Status(statusCfg, hostName)
		if *jsonOut {
			errs := []string(nil)
			if err != nil {
				errs = []string{err.Error()}
			}
			return writeJSON(os.Stdout, output.Envelope{
				Version: output.SchemaVersion,
				Command: "status",
				OK:      err == nil && res != nil && res.OK,
				Data:    output.StatusResult{FileEntries: fileStatusEntries(res)},
				Errors:  errs,
			})
		}
		if err != nil {
			fprintf(os.Stderr, "genv status --files: %v\n", err)
			return exitLogic
		}
		if res == nil || res.OK {
			fPrintln(os.Stdout, "files up to date.")
			return exitOK
		}
		writeFileStatus(os.Stdout, res)
		return exitLogic
	}

	lf, err := genvfile.ReadLock(lockPathForSpec(*file, *lockFile))
	if err != nil {
		fprintf(os.Stderr, "genv: reading lock: %v\n", err)
		return exitIO
	}

	entries := commands.Status(f, lf)
	envEntries := genvenv.EnvStatus(f.Env, lf.Env)
	shellEntries := shellcfg.ShellStatus(f.Shell, lf.Shell)
	serviceEntries := service.ServiceStatus(f.Services, lf.Services)

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
		jsonEnvEntries, envDrift := toOutputEnvEntries(envEntries)
		if envDrift {
			hasDrift = true
		}
		jsonShellEntries, shellDrift := toOutputShellEntries(shellEntries)
		if shellDrift {
			hasDrift = true
		}
		jsonServiceEntries := make([]output.ServiceStatusEntry, 0, len(serviceEntries))
		for _, e := range serviceEntries {
			jsonServiceEntries = append(jsonServiceEntries, output.ServiceStatusEntry{
				Name:    e.Name,
				Kind:    string(e.Kind),
				Running: e.Running,
			})
			if e.Kind != service.ServiceStatusOK {
				hasDrift = true
			}
		}
		return writeJSON(os.Stdout, output.Envelope{
			Version: output.SchemaVersion,
			Command: "status",
			OK:      !hasDrift,
			Data:    output.StatusResult{Entries: jsonEntries, EnvEntries: jsonEnvEntries, ShellEntries: jsonShellEntries, ServiceEntries: jsonServiceEntries},
		})
	}

	if len(entries) == 0 && len(envEntries) == 0 && len(shellEntries) == 0 && len(serviceEntries) == 0 {
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

	// Env variable status section.
	if len(envEntries) > 0 {
		fPrintln(os.Stdout)
		fprintf(os.Stdout, "Env — %d variable", len(envEntries))
		if len(envEntries) != 1 {
			fprint(os.Stdout, "s")
		}
		fPrintln(os.Stdout)
		fPrintln(os.Stdout)
		tw2 := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		var hasEnvDrift bool
		for _, e := range envEntries {
			switch e.Kind {
			case genvenv.EnvStatusOK:
				fprintf(tw2, "  ok\t%s\t%s\n", e.Name, commands.RedactValue(e.SpecValue, e.Sensitive))
			case genvenv.EnvStatusModified:
				hasEnvDrift = true
				fprintf(tw2, "  modified\t%s\t(run 'genv apply' to update)\n", e.Name)
			case genvenv.EnvStatusMissing:
				fprintf(tw2, "  missing\t%s\t(in spec, not applied — run 'genv apply')\n", e.Name)
			case genvenv.EnvStatusExtra:
				hasEnvDrift = true
				fprintf(tw2, "  extra\t%s\t(in lock, not in spec — run 'genv apply' or 'genv env unset')\n", e.Name)
			}
		}
		_ = tw2.Flush()
		if hasEnvDrift {
			return exitLogic
		}
	}

	// Shell config status section.
	if len(shellEntries) > 0 {
		fPrintln(os.Stdout)
		fprintf(os.Stdout, "Shell — %d entr", len(shellEntries))
		if len(shellEntries) == 1 {
			fprint(os.Stdout, "y")
		} else {
			fprint(os.Stdout, "ies")
		}
		fPrintln(os.Stdout)
		fPrintln(os.Stdout)
		if writeShellStatusTable(os.Stdout, shellEntries) {
			return exitLogic
		}
	}

	// Service status section.
	if len(serviceEntries) > 0 {
		fPrintln(os.Stdout)
		fprintf(os.Stdout, "Services — %d service", len(serviceEntries))
		if len(serviceEntries) != 1 {
			fprint(os.Stdout, "s")
		}
		fPrintln(os.Stdout)
		fPrintln(os.Stdout)
		tw3 := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		var hasServiceDrift bool
		for _, e := range serviceEntries {
			runningStr := "stopped"
			if e.Running {
				runningStr = "running"
			}
			switch e.Kind {
			case service.ServiceStatusOK:
				fprintf(tw3, "  ok\t%s\t%s\n", e.Name, runningStr)
			case service.ServiceStatusModified:
				hasServiceDrift = true
				fprintf(tw3, "  modified\t%s\t(run 'genv apply' to update config)\n", e.Name)
			case service.ServiceStatusMissing:
				fprintf(tw3, "  missing\t%s\t(in spec, not applied — run 'genv apply')\n", e.Name)
			case service.ServiceStatusExtra:
				hasServiceDrift = true
				fprintf(tw3, "  extra\t%s\t(in lock, not in spec — run 'genv apply' or 'genv service remove')\n", e.Name)
			}
		}
		_ = tw3.Flush()
		if hasServiceDrift {
			return exitLogic
		}
	}

	if counts[commands.StatusDrift] > 0 || counts[commands.StatusExtra] > 0 {
		return exitLogic
	}
	return exitOK
}

// applyServices reconciles services, updates lf.Services in memory, and
// returns lists of applied and removed service names. The caller writes the lock.
// If verbose is true, it prints progress lines to stdout.
func applyServices(ctx context.Context, f *schema.GenvFile, lf *genvfile.LockFile, verbose bool) (applied, removed []string, errs []error) {
	if len(f.Services) == 0 && len(lf.Services) == 0 {
		return nil, nil, nil
	}

	applied, removed, errs = service.ApplyServices(ctx, f.Services, lf.Services, verbose)

	// Update lf.Services in memory; caller writes the lock once.
	lf.Services = service.SpecToLock(f.Services)

	return applied, removed, errs
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

// buildEditorCmd parses the editor string, validates the executable against a
// whitelist of safe editors, and returns an exec.Cmd ready to run.
func buildEditorCmd(editor, file string) (*exec.Cmd, error) {
	fields := strings.Fields(editor)
	if len(fields) == 0 {
		return exec.Command("vi", file), nil
	}

	bin := fields[0]
	base := filepath.Base(bin)
	if !safeEditors[base] {
		return nil, fmt.Errorf("editor %q is not allowed; must be one of: vi, vim, nano, emacs, code", bin)
	}

	for _, arg := range fields[1:] {
		if !safeFlags[arg] {
			return nil, fmt.Errorf("editor flag %q is not allowed; only safe flags are permitted", arg)
		}
	}

	args := append(fields[1:], file)
	return exec.Command(bin, args...), nil
}

func parseCommandWords(input string) ([]string, error) {
	var words []string
	var current strings.Builder
	inSingle := false
	inDouble := false
	escaped := false

	for i := 0; i < len(input); i++ {
		ch := input[i]

		if escaped {
			current.WriteByte(ch)
			escaped = false
			continue
		}

		if inSingle {
			if ch == '\'' {
				inSingle = false
			} else {
				current.WriteByte(ch)
			}
			continue
		}

		if inDouble {
			switch ch {
			case '"':
				inDouble = false
			case '\\':
				if i+1 < len(input) && strings.ContainsRune(`"$\`, rune(input[i+1])) {
					i++
					current.WriteByte(input[i])
				} else {
					current.WriteByte(ch)
				}
			default:
				current.WriteByte(ch)
			}
			continue
		}

		switch ch {
		case '\\':
			escaped = true
		case '\'':
			inSingle = true
		case '"':
			inDouble = true
		case ' ', '\t', '\n', '\r':
			if current.Len() > 0 {
				words = append(words, current.String())
				current.Reset()
			}
		default:
			current.WriteByte(ch)
		}
	}

	if escaped {
		return nil, errors.New("trailing escape")
	}
	if inSingle {
		return nil, errors.New("unterminated single-quoted string")
	}
	if inDouble {
		return nil, errors.New("unterminated double-quoted string")
	}
	if current.Len() > 0 {
		words = append(words, current.String())
	}
	if len(words) == 0 {
		return nil, errors.New("command must not be empty")
	}

	return words, nil
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

	cmd, err := buildEditorCmd(editor, *file)
	if err != nil {
		fprintf(os.Stderr, "genv edit: %v\n", err)
		return exitLogic
	}

	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fprintf(os.Stderr, "genv edit: %v\n", err)
		return exitLogic
	}
	return exitOK
}

// completionCmd implements `genv completion <shell>` and
// `genv completion install [shell]`.
//
// Without the install subcommand it prints the shell completion script for
// bash, zsh, or fish to stdout. With `install` it writes that script into the
// shell's standard completion directory so completions work with no manual
// setup.
func completionCmd(args []string) int {
	if len(args) > 0 && args[0] == "install" {
		return completionInstallCmd(args[1:])
	}

	fs := flag.NewFlagSet("completion", flag.ContinueOnError)
	fs.Usage = func() {
		fPrintln(os.Stderr, "usage: genv completion <shell>")
		fPrintln(os.Stderr, "       genv completion install [shell] [--dir <path>]")
		fPrintln(os.Stderr)
		fPrintln(os.Stderr, "  shell   One of: bash, zsh, fish")
		fPrintln(os.Stderr)
		fPrintln(os.Stderr, "examples:")
		fPrintln(os.Stderr, "  genv completion bash >> ~/.bashrc")
		fPrintln(os.Stderr, "  genv completion zsh  > ~/.zsh/completions/_genv")
		fPrintln(os.Stderr, "  genv completion fish > ~/.config/fish/completions/genv.fish")
		fPrintln(os.Stderr, "  genv completion install        # auto-detect the current shell")
		fPrintln(os.Stderr, "  genv completion install zsh    # install for a specific shell")
	}
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	if fs.NArg() < 1 {
		fPrintln(os.Stderr, "genv completion: missing shell argument (bash, zsh, or fish)")
		fs.Usage()
		return exitUsage
	}
	script, _, _, err := completionScriptFor(fs.Arg(0))
	if err != nil {
		fprintf(os.Stderr, "genv completion: %v\n", err)
		return exitUsage
	}
	fprint(os.Stdout, script)
	return exitOK
}

// completionScriptFor maps a shell name to its embedded completion script, the
// filename that shell expects the script to be installed as, and the default
// directory that shell auto-loads completions from. An unknown or empty shell
// name returns an error.
func completionScriptFor(shell string) (script, filename, defaultDir string, err error) {
	switch shell {
	case "bash":
		// bash-completion sources files named after the command from this dir.
		return completionBash, "genv", filepath.Join(xdgDataHome(), "bash-completion", "completions"), nil
	case "zsh":
		// site-functions is on the default $fpath in modern zsh; compinit binds
		// the "#compdef genv" tag in the _genv function file.
		return completionZsh, "_genv", filepath.Join(xdgDataHome(), "zsh", "site-functions"), nil
	case "fish":
		return completionFish, "genv.fish", filepath.Join(xdgConfigHome(), "fish", "completions"), nil
	case "":
		return "", "", "", fmt.Errorf("missing shell argument (bash, zsh, or fish)")
	default:
		return "", "", "", fmt.Errorf("unknown shell %q — supported shells are: bash, zsh, fish", shell)
	}
}

// completionInstallCmd implements `genv completion install [shell] [--dir <path>]`.
// It writes the embedded completion script into the shell's standard completion
// directory (or --dir), creating parent directories as needed. When no shell is
// given it is detected from $SHELL.
func completionInstallCmd(args []string) int {
	fs := flag.NewFlagSet("completion install", flag.ContinueOnError)
	dir := fs.String("dir", "", "Target directory (overrides the per-shell default)")
	fs.Usage = func() {
		fPrintln(os.Stderr, "usage: genv completion install [shell] [--dir <path>]")
		fPrintln(os.Stderr)
		fPrintln(os.Stderr, "  shell   One of: bash, zsh, fish (default: detected from $SHELL)")
		fPrintln(os.Stderr, "  --dir   Install into this directory instead of the shell default")
	}
	shell, flagArgs := extractPositional(args)
	if err := fs.Parse(flagArgs); err != nil {
		return exitUsage
	}
	if shell == "" {
		shell = detectShell()
		if shell == "" {
			fPrintln(os.Stderr, "genv completion install: could not detect shell from $SHELL; pass one of: bash, zsh, fish")
			return exitUsage
		}
	}

	script, filename, defaultDir, err := completionScriptFor(shell)
	if err != nil {
		fprintf(os.Stderr, "genv completion install: %v\n", err)
		return exitUsage
	}

	target := *dir
	if target == "" {
		target = defaultDir
	}
	if err := os.MkdirAll(target, 0o755); err != nil {
		fprintf(os.Stderr, "genv completion install: %v\n", err)
		return exitIO
	}
	path := filepath.Join(target, filename)
	if err := os.WriteFile(path, []byte(script), 0o644); err != nil {
		fprintf(os.Stderr, "genv completion install: %v\n", err)
		return exitIO
	}

	fprintf(os.Stdout, "Installed %s completion to %s\n", shell, path)
	if shell == "zsh" && *dir == "" {
		fprintf(os.Stdout, "Ensure %s is on your $fpath before compinit runs, then restart your shell.\n", target)
	}
	return exitOK
}

// detectShell returns "bash", "zsh", or "fish" based on the basename of $SHELL,
// or "" when it is unset or unrecognized.
func detectShell() string {
	switch filepath.Base(os.Getenv("SHELL")) {
	case "bash":
		return "bash"
	case "zsh":
		return "zsh"
	case "fish":
		return "fish"
	default:
		return ""
	}
}

// xdgDataHome returns $XDG_DATA_HOME or ~/.local/share.
func xdgDataHome() string {
	if x := os.Getenv("XDG_DATA_HOME"); x != "" {
		return x
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ".local/share"
	}
	return filepath.Join(home, ".local", "share")
}

// xdgConfigHome returns $XDG_CONFIG_HOME or ~/.config.
func xdgConfigHome() string {
	if x := os.Getenv("XDG_CONFIG_HOME"); x != "" {
		return x
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ".config"
	}
	return filepath.Join(home, ".config")
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
	lockFile := fs.String("lock-file", "", "path to genv lock file")
	dryRun := fs.Bool("dry-run", false, "print the upgrade commands without executing")
	yes := fs.Bool("yes", false, "skip the confirmation prompt")
	debug := fs.Bool("debug", false, "emit debug-level structured logs to stderr")
	hostFlag := fs.String("host", "", "host name for host-specific records (defaults to $GENV_HOST or os.Hostname())")

	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	if *debug {
		logging.Init(true)
	}

	hostName := hostForCommand(*hostFlag)
	f, err := genvfile.Read(*file)
	if err != nil {
		if errors.Is(err, genvfile.ErrNotFound) {
			fprintf(os.Stderr, "genv upgrade: %s not found — run 'genv init' to create one\n", *file)
			return exitIO
		}
		fprintf(os.Stderr, "genv upgrade: %v\n", err)
		if errors.Is(err, genvfile.ErrInvalidFile) {
			return exitValidation
		}
		return exitIO
	}
	f = host.FilterForHost(f, hostName)

	lockPath := lockPathForSpec(*file, *lockFile)
	lf, err := genvfile.ReadLock(lockPath)
	if err != nil {
		fprintf(os.Stderr, "genv upgrade: reading lock: %v\n", err)
		return exitIO
	}
	if len(lf.Packages) == 0 {
		fPrintln(os.Stdout, "no packages tracked — run 'genv add' or 'genv scan' first.")
		return exitOK
	}

	allowedIDs := make(map[string]bool, len(f.Packages))
	for _, p := range f.Packages {
		allowedIDs[p.ID] = true
	}

	plan, skipped := resolver.PlanUpgrade(lf.Packages)
	filtered := make([]resolver.UpgradeAction, 0, len(plan))
	for _, a := range plan {
		if allowedIDs[a.LP.ID] {
			filtered = append(filtered, a)
		}
	}
	plan = filtered
	for _, s := range skipped {
		fprintf(os.Stderr, "genv upgrade: adapter %q not registered for %s — skipping\n", s.Manager, s.ID)
	}

	if len(plan) == 0 {
		if !*dryRun {
			ctx := context.Background()
			failedHooks := runUpgradeHooks(ctx, "pre", f, hostName, false)
			failedHooks = append(failedHooks, runUpgradeHooks(ctx, "post", f, hostName, false)...)
			if len(failedHooks) > 0 {
				for _, e := range failedHooks {
					fprintf(os.Stderr, "genv upgrade: %s\n", e)
				}
				return exitLogic
			}
		}
		fPrintln(os.Stdout, "no upgradeable packages found.")
		return exitOK
	}

	fPrintln(os.Stdout, "upgrade plan:")
	for _, a := range plan {
		fprintf(os.Stdout, "  %s  via %s  ==> %s\n", a.LP.ID, a.LP.Manager, strings.Join(a.Cmd, " "))
	}

	if *dryRun {
		return exitOK
	}

	if !*yes && !confirm(fmt.Sprintf("\nUpgrade %d package(s)? [y/N] ", len(plan))) {
		fPrintln(os.Stdout, "Aborted.")
		return exitOK
	}

	ctx := context.Background()
	failedHooks := runUpgradeHooks(ctx, "pre", f, hostName, false)
	if len(failedHooks) > 0 {
		for _, e := range failedHooks {
			fprintf(os.Stderr, "genv upgrade: %s\n", e)
		}
		return exitLogic
	}

	execResult := resolver.ExecuteUpgrade(ctx, plan, os.Stdin, os.Stdout, os.Stderr)

	exitCode := exitOK
	if len(execResult.Errors) > 0 {
		for _, err := range execResult.Errors {
			fprintf(os.Stderr, "genv upgrade: %v\n", err)
		}
		exitCode = exitLogic
	}
	postHookErrs := runUpgradeHooks(ctx, "post", f, hostName, false)
	if len(postHookErrs) > 0 {
		for _, e := range postHookErrs {
			fprintf(os.Stderr, "genv upgrade: %s\n", e)
		}
		exitCode = exitLogic
	}

	// Build an ID→index map so each version update is O(1), not O(n).
	lockIndex := make(map[string]int, len(lf.Packages))
	for i, lp := range lf.Packages {
		lockIndex[lp.ID] = i
	}
	for _, upgraded := range execResult.Upgraded {
		if idx, ok := lockIndex[upgraded.ID]; ok {
			lf.Packages[idx].InstalledVersion = upgraded.InstalledVersion
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
	for token := range strings.SplitSeq(s, ",") {
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
  env         Manage shell environment variables (set, unset, list)
  shell       Manage shell aliases and shell config drift
  service     Manage user-space services
  pull        Fetch genv.json from the configured spec repository
  completion  Print or install the shell completion script (bash, zsh, or fish)
  validate    Validate genv.json against the schema
  upgrade     Upgrade all tracked packages to their latest versions
  init        Create a new genv.json interactively
  version     Show genv build version information
  help        Show this help text

Flags common to all commands:
  --file <path>   Path to genv.json (default: $XDG_CONFIG_HOME/genv/genv.json or ~/.config/genv/genv.json, falling back to ./genv.json)

Host-specific flags (used by apply, status, upgrade, adopt):
  --host <name>   Host name for host-specific records (default: $GENV_HOST or os.Hostname())

Add/Adopt-specific flags:
  --version <ver>              Version constraint, e.g. "0.10.*"
  --prefer <mgr>               Preferred manager, e.g. brew
  --manager <mgr:name,...>     Manager-specific package names, e.g.
                               snap:hello,brew:hello

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

// serviceCmd implements `genv service <subcommand>`.
func serviceCmd(args []string) int {
	if len(args) == 0 {
		fPrintln(os.Stderr, "usage: genv service <add|remove|list|start|stop|status> [flags]")
		fPrintln(os.Stderr)
		fPrintln(os.Stderr, "subcommands:")
		fPrintln(os.Stderr, "  add <name> --start <cmd> [--stop <cmd>] [--restart <cmd>] [--status <cmd>]   Add or update a service (raw commands)")
		fPrintln(os.Stderr, "  add <name> --brew-formula <formula>                                          Add a brew-managed service (macOS)")
		fPrintln(os.Stderr, "  remove <name>                                                              Remove a service from the spec")
		fPrintln(os.Stderr, "  list                                                                        Show all declared services")
		fPrintln(os.Stderr, "  start <name>                                                               Start a service")
		fPrintln(os.Stderr, "  stop <name>                                                                Stop a service")
		fPrintln(os.Stderr, "  status <name>                                                              Show service running status")
		return exitUsage
	}
	switch args[0] {
	case "add":
		return serviceAddCmd(args[1:])
	case "remove", "rm":
		return serviceRemoveCmd(args[1:])
	case "list", "ls":
		return serviceListCmd(args[1:])
	case "start":
		return serviceStartCmd(args[1:])
	case "stop":
		return serviceStopCmd(args[1:])
	case "status":
		return serviceStatusCmd(args[1:])
	default:
		fprintf(os.Stderr, "genv service: unknown subcommand %q\n\nRun 'genv service' for usage.\n", args[0])
		return exitUsage
	}
}

// serviceAddCmd implements `genv service add <name> --start <cmd> [--stop <cmd>] [--restart <cmd>] [--status <cmd>]`.
func serviceAddCmd(args []string) int {
	fs := flag.NewFlagSet("service add", flag.ContinueOnError)
	fs.Usage = func() {
		fPrintln(os.Stderr, "usage: genv service add <name> --start <cmd> [flags]")
		fPrintln(os.Stderr, "       genv service add <name> --brew-formula <formula> [flags]")
		fPrintln(os.Stderr)
		fPrintln(os.Stderr, "flags:")
		fs.PrintDefaults()
	}
	file := fs.String("file", defaultSpecPath(), "path to genv.json")
	start := fs.String("start", "", "command to start the service")
	stop := fs.String("stop", "", "command to stop the service")
	restart := fs.String("restart", "", "command to restart the service")
	status := fs.String("status", "", "command to check service status")
	brewFormula := fs.String("brew-formula", "", "homebrew formula to manage via `brew services` (macOS only)")

	name, flagArgs := extractPositional(args)
	if err := fs.Parse(flagArgs); err != nil {
		return exitUsage
	}
	if name == "" {
		fPrintln(os.Stderr, "genv service add: missing service name")
		fs.Usage()
		return exitUsage
	}
	if *start == "" && *brewFormula == "" {
		fPrintln(os.Stderr, "genv service add: either --start or --brew-formula is required")
		fs.Usage()
		return exitUsage
	}

	f, isNew, err := genvfile.ReadOrNew(*file)
	if err != nil {
		fprintf(os.Stderr, "genv: %v\n", err)
		if errors.Is(err, genvfile.ErrInvalidFile) {
			return exitValidation
		}
		return exitIO
	}

	var startCmd []string
	if *start != "" {
		startCmd, err = parseCommandWords(*start)
		if err != nil {
			fprintf(os.Stderr, "genv service add: invalid --start command: %v\n", err)
			return exitUsage
		}
	}
	var stopCmd, restartCmd, statusCmd []string
	if *stop != "" {
		stopCmd, err = parseCommandWords(*stop)
		if err != nil {
			fprintf(os.Stderr, "genv service add: invalid --stop command: %v\n", err)
			return exitUsage
		}
	}
	if *restart != "" {
		restartCmd, err = parseCommandWords(*restart)
		if err != nil {
			fprintf(os.Stderr, "genv service add: invalid --restart command: %v\n", err)
			return exitUsage
		}
	}
	if *status != "" {
		statusCmd, err = parseCommandWords(*status)
		if err != nil {
			fprintf(os.Stderr, "genv service add: invalid --status command: %v\n", err)
			return exitUsage
		}
	}

	if err := commands.ServiceAdd(f, name, startCmd, stopCmd, restartCmd, statusCmd, *brewFormula); err != nil {
		fprintf(os.Stderr, "genv: %v\n", err)
		return exitUsage
	}

	if err := genvfile.Write(*file, f); err != nil {
		fprintf(os.Stderr, "genv: %v\n", err)
		return exitIO
	}
	if isNew {
		fprintf(os.Stdout, "created %s\n", *file)
	}

	fprintf(os.Stdout, "added service %q\nRun 'genv apply' to reconcile services.\n", name)
	return exitOK
}

// serviceRemoveCmd implements `genv service remove <name>`.
func serviceRemoveCmd(args []string) int {
	fs := flag.NewFlagSet("service remove", flag.ContinueOnError)
	fs.Usage = func() {
		fPrintln(os.Stderr, "usage: genv service remove <name> [flags]")
		fPrintln(os.Stderr)
		fPrintln(os.Stderr, "flags:")
		fs.PrintDefaults()
	}
	file := fs.String("file", defaultSpecPath(), "path to genv.json")

	name, flagArgs := extractPositional(args)
	if err := fs.Parse(flagArgs); err != nil {
		return exitUsage
	}
	if name == "" {
		fPrintln(os.Stderr, "genv service remove: name is required")
		fs.Usage()
		return exitUsage
	}

	f, err := genvfile.Read(*file)
	if err != nil {
		if errors.Is(err, genvfile.ErrNotFound) {
			fprintf(os.Stderr, "genv: %v\n", err)
			return exitLogic
		}
		fprintf(os.Stderr, "genv: %v\n", err)
		if errors.Is(err, genvfile.ErrInvalidFile) {
			return exitValidation
		}
		return exitIO
	}

	if err := commands.ServiceRemove(f, name); err != nil {
		fprintf(os.Stderr, "genv: %v\n", err)
		if errors.Is(err, commands.ErrServiceNotFound) {
			return exitLogic
		}
		return exitUsage
	}

	if err := genvfile.Write(*file, f); err != nil {
		fprintf(os.Stderr, "genv: %v\n", err)
		return exitIO
	}

	fprintf(os.Stdout, "removed service %q\nRun 'genv apply' to stop removed services.\n", name)
	return exitOK
}

// serviceListCmd implements `genv service list`.
func serviceListCmd(args []string) int {
	fs := flag.NewFlagSet("service list", flag.ContinueOnError)
	fs.Usage = func() {
		fPrintln(os.Stderr, "usage: genv service list [flags]")
		fPrintln(os.Stderr)
		fPrintln(os.Stderr, "flags:")
		fs.PrintDefaults()
	}
	file := fs.String("file", defaultSpecPath(), "path to genv.json")

	if err := fs.Parse(args); err != nil {
		return exitUsage
	}

	f, err := genvfile.Read(*file)
	if err != nil {
		if errors.Is(err, genvfile.ErrNotFound) {
			fprintf(os.Stderr, "genv: %s not found — run 'genv service add' to create one\n", *file)
			return exitIO
		}
		fprintf(os.Stderr, "genv: %v\n", err)
		if errors.Is(err, genvfile.ErrInvalidFile) {
			return exitValidation
		}
		return exitIO
	}

	commands.ServiceList(f, os.Stdout)
	return exitOK
}

// serviceStartCmd implements `genv service start <name>`.
func serviceStartCmd(args []string) int {
	fs := flag.NewFlagSet("service start", flag.ContinueOnError)
	file := fs.String("file", defaultSpecPath(), "path to genv.json")

	name, flagArgs := extractPositional(args)
	if err := fs.Parse(flagArgs); err != nil {
		return exitUsage
	}
	if name == "" {
		fPrintln(os.Stderr, "genv service start: name is required")
		return exitUsage
	}

	f, err := genvfile.Read(*file)
	if err != nil {
		fprintf(os.Stderr, "genv: %v\n", err)
		return exitIO
	}

	// Debug: print what services we found
	if os.Getenv("GENV_DEBUG") != "" {
		fprintf(os.Stderr, "DEBUG: Looking for service %q in spec\n", name)
		fprintf(os.Stderr, "DEBUG: Services in spec: %v\n", f.Services)
	}

	svc, ok := f.Services[name]
	if !ok {
		fprintf(os.Stderr, "genv: service %q not found in spec\n", name)
		return exitLogic
	}

	if svc.BrewFormula != "" {
		fprintf(os.Stdout, "Starting service %q via brew services: %s\n", name, svc.BrewFormula)
		if err := service.BrewServicesStart(context.Background(), svc.BrewFormula); err != nil {
			fprintf(os.Stderr, "genv: %v\n", err)
			return exitLogic
		}
		return exitOK
	}

	fprintf(os.Stdout, "Starting service %q: %s\n", name, strings.Join(svc.Start, " "))
	cmd := exec.Command(svc.Start[0], svc.Start[1:]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fprintf(os.Stderr, "genv: failed to start service %q: %v\n", name, err)
		if service.IsSystemdAvailable() {
			fprintf(os.Stderr, "Tip: to view logs run: %s\n", service.SystemdLogsHint(name))
		}
		return exitLogic
	}
	return exitOK
}

// serviceStopCmd implements `genv service stop <name>`.
func serviceStopCmd(args []string) int {
	fs := flag.NewFlagSet("service stop", flag.ContinueOnError)
	file := fs.String("file", defaultSpecPath(), "path to genv.json")

	name, flagArgs := extractPositional(args)
	if err := fs.Parse(flagArgs); err != nil {
		return exitUsage
	}
	if name == "" {
		fPrintln(os.Stderr, "genv service stop: name is required")
		return exitUsage
	}

	f, err := genvfile.Read(*file)
	if err != nil {
		fprintf(os.Stderr, "genv: %v\n", err)
		return exitIO
	}

	svc, ok := f.Services[name]
	if !ok {
		fprintf(os.Stderr, "genv: service %q not found in spec\n", name)
		return exitLogic
	}

	if svc.BrewFormula != "" {
		fprintf(os.Stdout, "Stopping service %q via brew services: %s\n", name, svc.BrewFormula)
		if err := service.BrewServicesStop(context.Background(), svc.BrewFormula); err != nil {
			fprintf(os.Stderr, "genv: %v\n", err)
			return exitLogic
		}
		return exitOK
	}

	if len(svc.Stop) == 0 {
		fprintf(os.Stderr, "genv: no stop command defined for service %q\n", name)
		return exitLogic
	}

	fprintf(os.Stdout, "Stopping service %q: %s\n", name, strings.Join(svc.Stop, " "))
	cmd := exec.Command(svc.Stop[0], svc.Stop[1:]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fprintf(os.Stderr, "genv: failed to stop service %q: %v\n", name, err)
		return exitLogic
	}
	return exitOK
}

// serviceStatusCmd implements `genv service status <name>`.
func serviceStatusCmd(args []string) int {
	fs := flag.NewFlagSet("service status", flag.ContinueOnError)
	file := fs.String("file", defaultSpecPath(), "path to genv.json")

	name, flagArgs := extractPositional(args)
	if err := fs.Parse(flagArgs); err != nil {
		return exitUsage
	}
	if name == "" {
		fPrintln(os.Stderr, "genv service status: name is required")
		return exitUsage
	}

	f, err := genvfile.Read(*file)
	if err != nil {
		fprintf(os.Stderr, "genv: %v\n", err)
		return exitIO
	}

	svc, ok := f.Services[name]
	if !ok {
		fprintf(os.Stderr, "genv: service %q not found in spec\n", name)
		return exitLogic
	}

	if len(svc.Status) == 0 {
		fprintf(os.Stderr, "genv: no status command defined for service %q\n", name)
		return exitLogic
	}

	cmd := exec.Command(svc.Status[0], svc.Status[1:]...)
	if err := cmd.Run(); err != nil {
		fprintf(os.Stdout, "service %q is NOT running\n", name)
		return exitLogic
	}
	fprintf(os.Stdout, "service %q is running\n", name)
	return exitOK
}
