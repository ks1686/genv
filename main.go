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
	"runtime"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/ks1686/genv/internal/adapter"
	"github.com/ks1686/genv/internal/commands"
	"github.com/ks1686/genv/internal/complete"
	genvenv "github.com/ks1686/genv/internal/env"
	"github.com/ks1686/genv/internal/files"
	"github.com/ks1686/genv/internal/genvfile"
	"github.com/ks1686/genv/internal/hooks"
	"github.com/ks1686/genv/internal/host"
	"github.com/ks1686/genv/internal/lockgate"
	"github.com/ks1686/genv/internal/logging"
	"github.com/ks1686/genv/internal/output"
	"github.com/ks1686/genv/internal/profile"
	"github.com/ks1686/genv/internal/profilebackend"
	"github.com/ks1686/genv/internal/resolver"
	"github.com/ks1686/genv/internal/schema"
	"github.com/ks1686/genv/internal/search"
	"github.com/ks1686/genv/internal/service"
	"github.com/ks1686/genv/internal/shellcfg"
	"github.com/ks1686/genv/internal/target"
	"github.com/ks1686/genv/internal/upgrade"
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

//go:embed completions/genv.ps1
var completionPowerShell string

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
	case "profile":
		return profileCmd(args[1:])
	case "completion":
		return completionCmd(args[1:])
	case "validate":
		return validateCmd(args[1:])
	case "upgrade":
		return upgradeCmd(args[1:])
	case "updates":
		return updatesCmd(args[1:])
	case "pull":
		return pullCmd(args[1:])
	case "migrate":
		return migrateCmd(args[1:])
	case "export":
		return exportCmd(args[1:])
	case "map":
		return mapCmd(args[1:])
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

func resolveMutationTarget(commandName, file string, f *schema.GenvFile, targetFlag string) (string, int) {
	if f.SchemaVersion != schema.Version8 {
		return "", exitOK
	}
	targetID, err := target.Resolve(targetFlag)
	if err != nil {
		fprintf(os.Stderr, "genv %s: %v\n", commandName, err)
		return "", exitUsage
	}
	if _, err := commands.ActiveBundle(f, targetID); err != nil {
		fprintf(os.Stderr, "genv %s: %v in %s\n", commandName, err, file)
		return "", exitValidation
	}
	return targetID, exitOK
}

// resolveEffectiveSpec flattens a schemaVersion 8 target (MergeTarget) or applies
// legacy host filtering so callers can read top-level packages/env/shell/services.
func resolveEffectiveSpec(f *schema.GenvFile, hostName, targetFlag string) (*schema.GenvFile, string, error) {
	if f == nil {
		return nil, "", fmt.Errorf("genv file is nil")
	}
	if f.SchemaVersion == schema.Version8 {
		targetID, err := target.Resolve(targetFlag)
		if err != nil {
			return nil, "", err
		}
		if f.Targets[targetID] == nil {
			return nil, "", fmt.Errorf("no matching targets.%s", targetID)
		}
		effective, err := schema.MergeTarget(f, targetID)
		if err != nil {
			return nil, "", err
		}
		return effective, targetID, nil
	}
	return host.FilterForHost(f, hostName), "", nil
}

// materializeSpecForCommand resolves the effective flat spec for read paths
// (status, upgrade, updates) using the same Resolve+MergeTarget path as apply.
func materializeSpecForCommand(commandName, file string, f *schema.GenvFile, hostFlag, targetFlag string) (*schema.GenvFile, string, int) {
	effective, targetID, err := resolveEffectiveSpec(f, hostForCommand(hostFlag), targetFlag)
	if err == nil {
		return effective, targetID, exitOK
	}
	msg := err.Error()
	switch {
	case strings.Contains(msg, "resolve target") || strings.Contains(msg, "pass --target"):
		fprintf(os.Stderr, "genv %s: %v\n", commandName, err)
		return nil, "", exitUsage
	case strings.HasPrefix(msg, "no matching targets."):
		fprintf(os.Stderr, "genv %s: %s in %s\n", commandName, msg, file)
		return nil, "", exitValidation
	default:
		fprintf(os.Stderr, "genv %s: %v\n", commandName, err)
		return nil, "", exitValidation
	}
}

// readMaterializedSpec loads genv.json and flattens the active v8 target (or
// applies legacy host filtering) so callers can read top-level fields.
func readMaterializedSpec(commandName, file, hostFlag, targetFlag string) (*schema.GenvFile, int) {
	f, err := genvfile.Read(file)
	if err != nil {
		if errors.Is(err, genvfile.ErrNotFound) {
			fprintf(os.Stderr, "genv %s: %s not found\n", commandName, file)
			return nil, exitIO
		}
		fprintf(os.Stderr, "genv %s: %v\n", commandName, err)
		if errors.Is(err, genvfile.ErrInvalidFile) {
			return nil, exitValidation
		}
		return nil, exitIO
	}
	effective, _, code := materializeSpecForCommand(commandName, file, f, hostFlag, targetFlag)
	return effective, code
}

// materializedHooks returns the effective hooks block for lifecycle commands.
// Returns nil hooks (not an error) when the spec is missing or has no hooks.
func materializedHooks(commandName, file, hostFlag, targetFlag string) (*schema.HooksConfig, int) {
	f, err := genvfile.Read(file)
	if err != nil {
		return nil, exitOK
	}
	effective, _, code := materializeSpecForCommand(commandName, file, f, hostFlag, targetFlag)
	if code != exitOK {
		return nil, code
	}
	if effective == nil {
		return nil, exitOK
	}
	return effective.Hooks, exitOK
}

// prepareAddSpec reads or creates the spec in memory and records the package
// without writing. Callers persist with writePreparedAdd after a successful
// install so unresolved/failed adds leave the file unchanged.
func prepareAddSpec(file, id, version, prefer string, managers map[string]string, targetFlag string) (*schema.GenvFile, bool, int) {
	f, isNew, err := genvfile.ReadOrNew(file)
	if err != nil {
		fprintf(os.Stderr, "genv: %v\n", err)
		if errors.Is(err, genvfile.ErrInvalidFile) {
			return nil, false, exitValidation
		}
		return nil, false, exitIO
	}
	targetID, exit := resolveMutationTarget("add", file, f, targetFlag)
	if exit != exitOK {
		return nil, false, exit
	}
	if err := commands.Add(f, id, version, prefer, managers, targetID); err != nil {
		fprintf(os.Stderr, "genv: %v\n", err)
		if errors.Is(err, commands.ErrAlreadyTracked) {
			return nil, false, exitLogic
		}
		return nil, false, exitUsage
	}
	return f, isNew, exitOK
}

func writePreparedAdd(file string, f *schema.GenvFile, isNew bool) int {
	if err := genvfile.Write(file, f); err != nil {
		fprintf(os.Stderr, "genv: %v\n", err)
		return exitIO
	}
	if isNew {
		fprintf(os.Stdout, "created %s\n", file)
	}
	return exitOK
}

// addToSpec records a package and writes immediately. Used by adopt, which has
// already verified the package is installed.
func addToSpec(file, id, version, prefer string, managers map[string]string, targetFlag string) int {
	f, isNew, exit := prepareAddSpec(file, id, version, prefer, managers, targetFlag)
	if exit != exitOK {
		return exit
	}
	return writePreparedAdd(file, f, isNew)
}

// appendLockEntry reads the lock at lockPath, appends lp, and writes it back.
// Returns an exit code; exitOK means success.
func lockPathForSpec(file, override string) string {
	if override != "" {
		return override
	}
	return genvfile.LockPathFrom(file)
}

func stampLockTarget(lf *genvfile.LockFile, targetID string) {
	if lf == nil || targetID == "" {
		return
	}
	lf.Target = targetID
	lf.GOOS = runtime.GOOS
}

func appendLockEntry(lockPath string, lp genvfile.LockedPackage, targetID string) int {
	lf, err := genvfile.ReadLock(lockPath)
	if err != nil {
		fprintf(os.Stderr, "genv: reading lock: %v\n", err)
		return exitIO
	}
	lf.Packages = append(lf.Packages, lp)
	stampLockTarget(lf, targetID)
	if err := genvfile.WriteLock(lockPath, lf); err != nil {
		fprintf(os.Stderr, "genv: writing lock: %v\n", err)
		return exitIO
	}
	return exitOK
}

// removeFromSpecAndReadLock reads the spec at file, removes id from it, writes
// it back, then reads and returns the lock file. Returns the lock, the lock
// path, and an exit code. exitOK means all steps succeeded.
func removeFromSpecAndReadLock(file, id, lockFile, targetFlag string) (*genvfile.LockFile, string, int) {
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
	targetID, exit := resolveMutationTarget("remove", file, f, targetFlag)
	if exit != exitOK {
		return nil, "", exit
	}
	if err := commands.Remove(f, id, targetID); err != nil {
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
// Resolves and installs the package first, then records it in genv.json and
// the lock. Unresolved or failed installs leave the spec unchanged.
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
	noHooks := fs.Bool("no-hooks", false, "skip pre-add and post-add hooks")
	hookTimeout := fs.Duration("hook-timeout", 0, "per-hook timeout, e.g. 5m or 30s (0 means no timeout)")
	hostFlag := fs.String("host", "", "host name for host-specific records (defaults to $GENV_HOST or os.Hostname())")
	targetFlag := fs.String("target", "", "portable target id for schemaVersion 8 specs")

	id, flagArgs := extractPositional(args)
	if err := fs.Parse(flagArgs); err != nil {
		return exitUsage
	}
	if *hookTimeout < 0 {
		fPrintln(os.Stderr, "genv add: --hook-timeout must be non-negative")
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

	hostName := hostForCommand(*hostFlag)
	lockPath := lockPathForSpec(*file, *lockFile)
	lf, _ := genvfile.ReadLock(lockPath)
	if !*noHooks {
		hooksCfg, code := materializedHooks("add", *file, *hostFlag, *targetFlag)
		if code != exitOK {
			return code
		}
		if hooksCfg != nil && len(hooksCfg.PreAdd) > 0 {
			profileName := ""
			if lf != nil {
				profileName = lf.ActiveProfile
			}
			errs := runHookPhase(context.Background(), hookPhaseRun{
				Hooks:   hooksCfg.PreAdd,
				Context: hookContext{Event: "add", Phase: "pre-add", Host: hostName, Profile: profileName, Installed: []string{id}},
				Timeout: *hookTimeout, Stdout: os.Stdout, Stderr: os.Stderr,
			})
			if len(errs) > 0 {
				fprintf(os.Stderr, "genv add: %s\n", errs[0])
				return exitLogic
			}
		}
	}

	// 1. Validate and stage the spec in memory. Do not write until install succeeds.
	prepared, isNew, exit := prepareAddSpec(*file, id, *version, *prefer, managers, *targetFlag)
	if exit != exitOK {
		return exit
	}

	// 2. Resolve and install. Track-only is `genv adopt`.
	pkg := schema.Package{ID: id, Version: *version, Prefer: *prefer, Managers: managers}
	action := resolver.ResolveOne(pkg, available)
	if !action.Resolved() {
		fprintf(os.Stderr, "genv add: no manager available to install %q; spec unchanged (use 'genv adopt' to track without installing)\n", id)
		return exitLogic
	}

	fprintf(os.Stdout, "installing %s via %s\n", id, action.Manager)
	fprintf(os.Stdout, "\n==> %s\n", strings.Join(action.Cmd, " "))
	if err := runForegroundCommand(action.Cmd); err != nil {
		fprintf(os.Stderr, "genv add: installation failed: %v\n", err)
		fPrintln(os.Stderr, "Spec unchanged. Fix the install error and retry, or use 'genv adopt' to track an already-installed package.")
		return exitLogic
	}

	// 3. Persist only after a successful install.
	if exit := writePreparedAdd(*file, prepared, isNew); exit != exitOK {
		return exit
	}

	// 4. Update lock file.
	targetID, code := resolveMutationTarget("add", *file, prepared, *targetFlag)
	if code != exitOK {
		return code
	}
	exit = appendLockEntry(lockPath, genvfile.LockedPackage{
		ID:      action.Pkg.ID,
		Manager: action.Manager,
		PkgName: action.PkgName,
	}, targetID)
	if exit != exitOK || *noHooks {
		return exit
	}
	hooksCfg, code := materializedHooks("add", *file, *hostFlag, *targetFlag)
	if code != exitOK {
		return code
	}
	if hooksCfg == nil || len(hooksCfg.PostAdd) == 0 {
		return exit
	}
	profileName := ""
	if lf != nil {
		profileName = lf.ActiveProfile
	}
	errs := runHookPhase(context.Background(), hookPhaseRun{
		Hooks:   hooksCfg.PostAdd,
		Context: hookContext{Event: "add", Phase: "post-add", Host: hostName, Profile: profileName, Installed: []string{id}},
		Timeout: *hookTimeout, Stdout: os.Stdout, Stderr: os.Stderr,
	})
	if len(errs) > 0 {
		fprintf(os.Stderr, "genv add: %s\n", errs[0])
		return exitLogic
	}
	return exit
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
	noHooks := fs.Bool("no-hooks", false, "skip pre-remove and post-remove hooks")
	hookTimeout := fs.Duration("hook-timeout", 0, "per-hook timeout, e.g. 5m or 30s (0 means no timeout)")
	hostFlag := fs.String("host", "", "host name for host-specific records (defaults to $GENV_HOST or os.Hostname())")
	targetFlag := fs.String("target", "", "portable target id for schemaVersion 8 specs")

	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	if *hookTimeout < 0 {
		fPrintln(os.Stderr, "genv remove: --hook-timeout must be non-negative")
		return exitUsage
	}
	if fs.NArg() < 1 {
		fPrintln(os.Stderr, "genv remove: missing package id")
		fs.Usage()
		return exitUsage
	}
	id := fs.Arg(0)

	return runRemove(removeOptions{File: *file, ID: id, LockFile: *lockFile, NoHooks: *noHooks, HookTimeout: *hookTimeout, Host: *hostFlag, Target: *targetFlag})
}

type removeOptions struct {
	File        string
	ID          string
	LockFile    string
	Host        string
	Target      string
	NoHooks     bool
	HookTimeout time.Duration
}

func runRemove(opts removeOptions) int {
	file := opts.File
	id := opts.ID
	// 0. When stdin is a terminal and id has no exact match in the spec,
	//    fall back to substring matching so users can type short names
	//    (e.g. "firefox" resolving to a tracked id like "org.mozilla.firefox").
	if isTerminal() {
		if f, err := genvfile.Read(file); err == nil {
			packages := f.Packages
			if f.SchemaVersion == schema.Version8 {
				effective, _, exit := materializeSpecForCommand("remove", file, f, opts.Host, opts.Target)
				if exit != exitOK {
					return exit
				}
				packages = effective.Packages
			}
			idLower := strings.ToLower(id)
			exact := false
			var matches []string
			for _, p := range packages {
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
	hostName := hostForCommand(opts.Host)
	lockPath := lockPathForSpec(file, opts.LockFile)
	lfBefore, _ := genvfile.ReadLock(lockPath)
	if !opts.NoHooks {
		hooksCfg, code := materializedHooks("remove", file, opts.Host, opts.Target)
		if code != exitOK {
			return code
		}
		if hooksCfg != nil && len(hooksCfg.PreRemove) > 0 {
			profileName := ""
			if lfBefore != nil {
				profileName = lfBefore.ActiveProfile
			}
			errs := runHookPhase(context.Background(), hookPhaseRun{
				Hooks:   hooksCfg.PreRemove,
				Context: hookContext{Event: "remove", Phase: "pre-remove", Host: hostName, Profile: profileName, Removed: []string{id}},
				Timeout: opts.HookTimeout, Stdout: os.Stdout, Stderr: os.Stderr,
			})
			if len(errs) > 0 {
				fprintf(os.Stderr, "genv remove: %s\n", errs[0])
				return exitLogic
			}
		}
	}

	// 1. Update genv.json and read lock.
	lf, lockPath, exit := removeFromSpecAndReadLock(file, id, opts.LockFile, opts.Target)
	if exit != exitOK {
		return exit
	}
	unlock, err := genvfile.LockMutation(lockPath)
	if err != nil {
		fprintf(os.Stderr, "genv: locking %s: %v\n", lockPath, err)
		return exitIO
	}
	defer unlock()
	lf, err = genvfile.ReadLock(lockPath)
	if err != nil {
		fprintf(os.Stderr, "genv: reading lock: %v\n", err)
		return exitIO
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
		return exitLogic
	}

	// Cache clean.
	for _, cleanCmd := range mgr.PlanClean() {
		fprintf(os.Stdout, "\n==> %s\n", strings.Join(cleanCmd, " "))
		if err := runForegroundCommand(cleanCmd); err != nil {
			fprintf(os.Stderr, "genv: cache clean warning: %v\n", err)
		}
	}

	lf.Packages = remaining
	if err := genvfile.WriteLock(lockPath, lf); err != nil {
		fprintf(os.Stderr, "genv: writing lock: %v\n", err)
		return exitIO
	}
	if !opts.NoHooks {
		hooksCfg, code := materializedHooks("remove", file, opts.Host, opts.Target)
		if code != exitOK {
			return code
		}
		if hooksCfg != nil && len(hooksCfg.PostRemove) > 0 {
			profileName := ""
			if lfBefore != nil {
				profileName = lfBefore.ActiveProfile
			}
			errs := runHookPhase(context.Background(), hookPhaseRun{
				Hooks:   hooksCfg.PostRemove,
				Context: hookContext{Event: "remove", Phase: "post-remove", Host: hostName, Profile: profileName, Removed: []string{id}},
				Timeout: opts.HookTimeout, Stdout: os.Stdout, Stderr: os.Stderr,
			})
			if len(errs) > 0 {
				fprintf(os.Stderr, "genv remove: %s\n", errs[0])
				return exitLogic
			}
		}
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
	targetFlag := fs.String("target", "", "portable target id for schemaVersion 8 specs")
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
		return adoptFilesCmd(*file, *lockFile, *hostFlag, *targetFlag, *jsonOut)
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
	if exit := addToSpec(*file, id, *version, *prefer, managers, *targetFlag); exit != exitOK {
		return exit
	}

	// 4. Update lock file.
	targetID := ""
	if prepared, err := genvfile.Read(*file); err == nil {
		var code int
		targetID, code = resolveMutationTarget("adopt", *file, prepared, *targetFlag)
		if code != exitOK {
			return code
		}
	}
	if exit := appendLockEntry(lockPathForSpec(*file, *lockFile), genvfile.LockedPackage{
		ID:      action.Pkg.ID,
		Manager: action.Manager,
		PkgName: action.PkgName,
	}, targetID); exit != exitOK {
		return exit
	}

	fprintf(os.Stdout, "adopted %s — now tracked via %s (already installed)\n", id, action.Manager)
	return exitOK
}

func adoptFilesCmd(file, lockFile, hostFlag, targetFlag string, jsonOut bool) int {
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
	filtered, _, code := materializeSpecForCommand("adopt", file, f, hostFlag, targetFlag)
	if code != exitOK {
		return code
	}
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
	targetFlag := fs.String("target", "", "portable target id for schemaVersion 8 specs")

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
	lf, lockPath, exit := removeFromSpecAndReadLock(*file, id, *lockFile, *targetFlag)
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
	File          string
	LockFile      string
	Host          string
	DryRun        bool
	Strict        bool
	Yes           bool
	Quiet         bool
	JSONOut       bool
	Force         bool
	Backup        bool
	Timeout       time.Duration
	Debug         bool
	TargetProfile string
	Target        string
	ForceNewLock  bool
	NoHooks       bool
	HookTimeout   time.Duration
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
	fs.BoolVar(&opts.Backup, "backup", false, "back up mismatched files before overwrite (implies keeping originals as *.backup.*)")
	fs.BoolVar(&opts.Strict, "strict", false, "exit with an error if any package cannot be resolved")
	fs.BoolVar(&opts.Yes, "yes", false, "skip the confirmation prompt (for CI and scripts)")
	fs.BoolVar(&opts.Quiet, "quiet", false, "suppress plan output (useful in scripts)")
	fs.BoolVar(&opts.JSONOut, "json", false, "emit machine-readable JSON to stdout instead of human-readable text")
	fs.DurationVar(&opts.Timeout, "timeout", 0, "per-subprocess timeout, e.g. 5m or 30s (0 means no timeout)")
	fs.DurationVar(&opts.HookTimeout, "hook-timeout", 0, "per-hook timeout, e.g. 5m or 30s (0 means no timeout)")
	fs.BoolVar(&opts.NoHooks, "no-hooks", false, "skip lifecycle hooks without skipping apply")
	fs.BoolVar(&opts.Debug, "debug", false, "emit debug-level structured logs to stderr")
	fs.StringVar(&opts.Host, "host", "", "host name for host-specific records (defaults to $GENV_HOST or os.Hostname())")
	fs.StringVar(&opts.Target, "target", "", "portable target id for schemaVersion 8 specs (defaults to $GENV_TARGET or host classification)")
	fs.BoolVar(&opts.ForceNewLock, "force-new-lock", false, "back up a foreign lock file and start with a new local lock")

	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	if opts.HookTimeout < 0 {
		fPrintln(os.Stderr, "genv apply: --hook-timeout must be non-negative")
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
		ctx = resolver.WithSubprocessTimeout(ctx, opts.Timeout)
	}

	lockPath := lockPathForSpec(opts.File, opts.LockFile)
	unlock, err := genvfile.LockMutation(lockPath)
	if err != nil {
		fprintf(os.Stderr, "genv: locking %s: %v\n", lockPath, err)
		return exitIO
	}
	defer unlock()
	lf, err := genvfile.ReadLock(lockPath)
	if err != nil {
		fprintf(os.Stderr, "genv: reading lock: %v\n", err)
		return exitIO
	}

	activeProfile := lf.ActiveProfile
	if opts.TargetProfile != "" {
		activeProfile = opts.TargetProfile
	}

	f, err := profile.LoadMerged(opts.File, activeProfile)
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

	return runApplyWithSpecAndLock(ctx, opts, f, lf, lockPath)
}

func applyLockGate(cmd, lockPath string, lf *genvfile.LockFile, activeTarget string, available map[string]bool, requireMeta, forceNew, dryRun bool, forceHint string) (*genvfile.LockFile, int) {
	var decision lockgate.Decision
	if requireMeta {
		decision = lockgate.CheckStrict(lf, activeTarget, runtime.GOOS, available)
	} else {
		decision = lockgate.Check(lf, activeTarget, runtime.GOOS, available)
	}
	for _, mgr := range decision.Unavailable {
		fprintf(os.Stderr, "genv %s: warning: lock uses unavailable manager %q; skipping its packages\n", cmd, mgr)
	}
	if !decision.Foreign {
		return lf, exitOK
	}
	if !forceNew {
		fprintf(os.Stderr, "genv %s: foreign lock refused: %s\n", cmd, decision.Reason)
		if forceHint != "" {
			fprintf(os.Stderr, "Back up or remove %s, or rerun with %s to move it aside and create a new local lock.\n", lockPath, forceHint)
		} else {
			fprintf(os.Stderr, "Back up or remove %s and rerun.\n", lockPath)
		}
		return lf, exitLogic
	}
	if !dryRun {
		if err := genvfile.RotateBackup(lockPath); err != nil {
			fprintf(os.Stderr, "genv %s: could not back up foreign lock %s: %v\n", cmd, lockPath, err)
			return lf, exitIO
		}
	}
	return &genvfile.LockFile{SchemaVersion: schema.Version8}, exitOK
}

func runApplyWithSpecAndLock(ctx context.Context, opts applyOptions, f *schema.GenvFile, lf *genvfile.LockFile, lockPath string) int {
	available := resolver.Detect()
	if lf == nil {
		lf = &genvfile.LockFile{SchemaVersion: schema.Version}
	}
	isV8 := f.SchemaVersion == schema.Version8
	effective, activeTarget, code := materializeSpecForCommand("apply", opts.File, f, opts.Host, opts.Target)
	if code != exitOK {
		return code
	}
	if isV8 {
		reset, code := applyLockGate("apply", lockPath, lf, activeTarget, available, true, opts.ForceNewLock, opts.DryRun, "--force-new-lock")
		if code != exitOK {
			return code
		}
		lf = reset
	}
	f = effective
	opts.Target = activeTarget
	live, liveWarns := resolver.LoadLiveSet(available)
	for _, w := range liveWarns {
		fprintf(os.Stderr, "genv apply: warning: %s\n", w)
	}
	result := resolver.ReconcileWith(f.Packages, lf.Packages, available, live)

	if opts.JSONOut {
		return runApplyJSON(ctx, opts, lockPath, f, lf, result)
	}
	return runApplyText(ctx, opts, lockPath, f, lf, result)
}

func runApplyJSON(ctx context.Context, opts applyOptions, lockPath string, f *schema.GenvFile, lf *genvfile.LockFile, result resolver.ReconcileResult) int {
	printReconcileWarnings(result)
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

	hostName := hostForCommand(opts.Host)
	if !opts.NoHooks {
		preErrs := runApplyHookPhase(ctx, f, hookContext{Event: "apply", Phase: "pre-apply", Host: hostName, Profile: lf.ActiveProfile, Yes: opts.Yes, Installed: plannedInstallIDs(result), Removed: plannedRemoveIDs(result)}, opts.HookTimeout, true)
		if len(preErrs) > 0 {
			return writeJSON(os.Stdout, output.Envelope{Version: output.SchemaVersion, Command: "apply", OK: false, Data: output.ApplyResult{FailedHooks: preErrs}, Errors: preErrs})
		}
	}
	execResult := resolver.ExecuteApply(ctx, result, os.Stdin, os.Stderr, os.Stderr)
	errs := errStrings(execResult.Errors)

	var envApplied, envRemoved, shellApplied, shellRemoved []string
	failedHooks := []string(nil)
	filePlan := &files.ApplyResult{}
	filePlanErr := error(nil)
	if len(errs) == 0 {
		var envErr, shellErr error
		envApplied, envRemoved, envErr = applyEnvVars(f, lf, false)
		if envErr != nil {
			errs = append(errs, envErr.Error())
		} else {
			shellApplied, shellRemoved, shellErr = applyShellCfg(f, lf, false)
			if shellErr != nil {
				errs = append(errs, shellErr.Error())
			}
		}
		if len(errs) == 0 {
			_, _, svcErrs := applyServices(ctx, f, lf, false)
			if len(svcErrs) > 0 {
				errs = append(errs, errStrings(svcErrs)...)
			}
		}
	}
	if len(errs) == 0 {
		filePlan, filePlanErr = applyFiles(ctx, opts, f, lf)
		if filePlanErr != nil {
			errs = append(errs, filePlanErr.Error())
			if !opts.NoHooks && hasPostApplyHooks(f) {
				skipMsg := "skipping post-apply hooks due to unresolved file mismatches"
				errs = append(errs, skipMsg)
				fprintf(os.Stderr, "genv apply: %s\n", skipMsg)
			}
		} else if !opts.NoHooks {
			failedHooks = runApplyHookPhase(ctx, f, hookContext{Event: "apply", Phase: "post-apply", Host: hostName, Profile: lf.ActiveProfile, Yes: opts.Yes, Installed: lockedPackageIDs(execResult.Installed), Removed: execResult.Uninstalled, Failed: applyFailedIDs(execResult.Errors)}, opts.HookTimeout, true)
			errs = append(errs, failedHooks...)
		}
	}
	success := len(errs) == 0
	if err := writeLockAfterApply(lockPath, lf, result, execResult, opts.TargetProfile, opts.Target, success); err != nil {
		fprintf(os.Stderr, "genv: writing lock: %v\n", err)
		errs = append(errs, err.Error())
		installed := make([]string, len(execResult.Installed))
		for i, lp := range execResult.Installed {
			installed[i] = lp.ID
		}
		return writeJSON(os.Stdout, output.Envelope{
			Version: output.SchemaVersion,
			Command: "apply",
			OK:      false,
			Data: output.ApplyResult{
				Installed:    installed,
				Uninstalled:  execResult.Uninstalled,
				EnvApplied:   envApplied,
				EnvRemoved:   envRemoved,
				ShellApplied: shellApplied,
				ShellRemoved: shellRemoved,
			},
			Errors: errs,
		})
	}

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
	printReconcileWarnings(result)
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
	for _, e := range service.ServiceStatus(f.Services, lf.Services, false) {
		if e.Kind != service.ServiceStatusOK {
			serviceChanges++
		}
	}
	planOpts := opts
	planOpts.DryRun = true
	filePlan, _ := applyFiles(ctx, planOpts, f, lf)
	fileChanges := 0
	if filePlan != nil {
		fileChanges = len(filePlan.Created) + len(filePlan.Updated) + len(filePlan.Mismatched)
	}

	if toInstall == 0 && toRemove == 0 && envChanges == 0 && shellChanges == 0 && serviceChanges == 0 && fileChanges == 0 {
		if !opts.Quiet {
			fPrintln(os.Stdout, "already up to date.")
		}
		if !opts.DryRun {
			if !opts.NoHooks {
				hostName := hostForCommand(opts.Host)
				hookErrs := runApplyHookPhase(ctx, f, hookContext{Event: "apply", Phase: "pre-apply", Host: hostName, Profile: lf.ActiveProfile, Yes: opts.Yes}, opts.HookTimeout, false)
				hookErrs = append(hookErrs, runApplyHookPhase(ctx, f, hookContext{Event: "apply", Phase: "post-apply", Host: hostName, Profile: lf.ActiveProfile, Yes: opts.Yes}, opts.HookTimeout, false)...)
				if len(hookErrs) > 0 {
					for _, e := range hookErrs {
						fprintf(os.Stderr, "genv apply: %s\n", e)
					}
					return exitLogic
				}
			}
			if err := writeLockAfterApply(lockPath, lf, result, resolver.ApplyExecution{}, opts.TargetProfile, opts.Target, true); err != nil {
				fprintf(os.Stderr, "genv: writing lock: %v\n", err)
				return exitIO
			}
		}
		return exitOK
	}

	if unresolvedCount > 0 && opts.Strict {
		fprintf(os.Stderr, "genv apply: %d package(s) unresolved; aborting (--strict)\n", unresolvedCount)
		return exitLogic
	}

	if fileChanges > 0 && !opts.Quiet {
		writeFilePlan(planOut, filePlan)
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

	hostName := hostForCommand(opts.Host)
	if !opts.NoHooks {
		hookErrs := runApplyHookPhase(ctx, f, hookContext{Event: "apply", Phase: "pre-apply", Host: hostName, Profile: lf.ActiveProfile, Yes: opts.Yes, Installed: plannedInstallIDs(result), Removed: plannedRemoveIDs(result)}, opts.HookTimeout, false)
		if len(hookErrs) > 0 {
			for _, e := range hookErrs {
				fprintf(os.Stderr, "genv apply: %s\n", e)
			}
			return exitLogic
		}
	}

	execResult := resolver.ExecuteApply(ctx, result, os.Stdin, os.Stdout, os.Stderr)

	var svcErrs []error
	var fileErrs []error
	var appliedFiles *files.ApplyResult
	if len(execResult.Errors) == 0 {
		if _, _, err := applyEnvVars(f, lf, !opts.Quiet); err != nil {
			fileErrs = append(fileErrs, err)
		} else if _, _, err := applyShellCfg(f, lf, !opts.Quiet); err != nil {
			fileErrs = append(fileErrs, err)
		} else {
			_, _, svcErrs = applyServices(ctx, f, lf, !opts.Quiet)
			if len(svcErrs) == 0 {
				var filePlanErr error
				appliedFiles, filePlanErr = applyFiles(ctx, opts, f, lf)
				if filePlanErr != nil {
					fileErrs = append(fileErrs, filePlanErr)
				}
				if appliedFiles != nil && len(appliedFiles.Mismatched) > 0 {
					writeFileMismatchGuidance(os.Stderr, appliedFiles)
					if !opts.NoHooks && hasPostApplyHooks(f) {
						fPrintln(os.Stderr, "genv apply: skipping post-apply hooks due to unresolved file mismatches")
					}
				}
			}
		}
	}
	var hookErrs []string
	if len(execResult.Errors) == 0 && len(svcErrs) == 0 && len(fileErrs) == 0 && !opts.NoHooks {
		hookErrs = runApplyHookPhase(ctx, f, hookContext{Event: "apply", Phase: "post-apply", Host: hostName, Profile: lf.ActiveProfile, Yes: opts.Yes, Installed: lockedPackageIDs(execResult.Installed), Removed: execResult.Uninstalled, Failed: applyFailedIDs(execResult.Errors)}, opts.HookTimeout, false)
	}
	success := len(execResult.Errors) == 0 && len(svcErrs) == 0 && len(fileErrs) == 0 && len(hookErrs) == 0
	if err := writeLockAfterApply(lockPath, lf, result, execResult, opts.TargetProfile, opts.Target, success); err != nil {
		fprintf(os.Stderr, "genv: writing lock: %v\n", err)
		return exitIO
	}

	if !success {
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

func printReconcileWarnings(result resolver.ReconcileResult) {
	for _, w := range result.Warnings {
		fprintf(os.Stderr, "genv apply: warning: %s\n", w)
	}
}

// writeLockAfterApply updates the lock file to reflect what actually succeeded.
// Called from both the JSON and human-readable paths of applyCmd.
func writeLockAfterApply(lockPath string, lf *genvfile.LockFile, result resolver.ReconcileResult, execResult resolver.ApplyExecution, targetProfile, activeTarget string, success bool) error {
	if success && targetProfile != "" {
		if targetProfile == "base" {
			lf.ActiveProfile = ""
		} else {
			lf.ActiveProfile = targetProfile
		}
	}
	if success && activeTarget != "" {
		lf.Target = activeTarget
		lf.GOOS = runtime.GOOS
	}
	uninstalledSet := make(map[string]bool, len(execResult.Uninstalled))
	for _, id := range execResult.Uninstalled {
		uninstalledSet[id] = true
	}
	installedSet := make(map[string]bool, len(execResult.Installed))
	for _, lp := range execResult.Installed {
		installedSet[lp.ID] = true
	}
	prevByID := make(map[string]genvfile.LockedPackage, len(lf.Packages))
	for _, lp := range lf.Packages {
		prevByID[lp.ID] = lp
	}
	newPkgs := make([]genvfile.LockedPackage, 0, len(result.Unchanged)+len(result.Adopted)+len(execResult.Installed)+len(result.ToRemove)+len(result.ToInstall))
	newPkgs = append(newPkgs, result.Unchanged...)
	newPkgs = append(newPkgs, result.Adopted...)
	newPkgs = append(newPkgs, execResult.Installed...)
	for _, a := range result.ToInstall {
		if installedSet[a.Pkg.ID] {
			continue
		}
		if prev, ok := prevByID[a.Pkg.ID]; ok {
			newPkgs = append(newPkgs, prev)
		}
	}
	for _, a := range result.ToRemove {
		if !uninstalledSet[a.Pkg.ID] {
			if prev, ok := prevByID[a.Pkg.ID]; ok {
				newPkgs = append(newPkgs, prev)
			} else {
				newPkgs = append(newPkgs, genvfile.LockedPackage{
					ID:      a.Pkg.ID,
					Manager: a.Manager,
					PkgName: a.PkgName,
				})
			}
		}
	}
	lf.Packages = newPkgs
	if err := genvfile.WriteLock(lockPath, lf); err != nil {
		return err
	}
	return nil
}

// applyEnvVars writes managed env fragments via selected profile backends,
// updates lf.Env in memory, and returns lists of applied and removed variable
// names. The caller is responsible for persisting the lock file (avoiding a
// double-write when packages and env vars are both applied in the same run).
// If verbose is true, it prints progress lines to stdout.
func applyEnvVars(f *schema.GenvFile, lf *genvfile.LockFile, verbose bool) (applied, removed []string, err error) {
	if len(f.Env) == 0 && len(lf.Env) == 0 {
		return nil, nil, nil
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

	if warn := profilebackend.MissingEngineWarning(runtime.GOOS); warn != "" {
		fprintf(os.Stderr, "genv: warning: %s\n", warn)
	}

	backends := profilebackend.SelectBackends(runtime.GOOS)
	var lastFrag string
	for _, b := range backends {
		if err := b.ApplyEnv(f.Env); err != nil {
			fprintf(os.Stderr, "genv: writing env fragment (%s): %v\n", b.Name(), err)
			return applied, removed, err
		}
		lastFrag = b.Name()
	}
	if lastFrag == "" && (len(applied) > 0 || len(removed) > 0 || len(f.Env) > 0) {
		// No backend ran (e.g. Windows without PowerShell and without POSIX rc).
		fprintf(os.Stderr, "genv: warning: no profile backend available to write env fragment\n")
	}

	if verbose {
		for _, name := range applied {
			fprintf(os.Stdout, "  env: set %s\n", name)
		}
		for _, name := range removed {
			fprintf(os.Stdout, "  env: removed %s\n", name)
		}
		if len(applied) > 0 || len(removed) > 0 {
			if fragPath, err := genvenv.FragmentPath(); err == nil {
				fprintf(os.Stdout, "env fragment written (%s backends) e.g. %s\n", lastFrag, fragPath)
			}
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

	return applied, removed, nil
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
	adopted := make([]output.PlanPackage, 0, len(result.Adopted))
	for _, lp := range result.Adopted {
		adopted = append(adopted, output.PlanPackage{ID: lp.ID, Manager: lp.Manager})
	}

	var toStart, toStop []string
	for _, e := range service.ServiceStatus(f.Services, lf.Services, false) {
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
		Adopted:         adopted,
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
	// Treat POSIX and Windows absolute paths as absolute even when the host
	// filepath.IsAbs disagrees (e.g. "/abs" on Windows).
	if filepath.IsAbs(expanded) || strings.HasPrefix(expanded, "/") || sourceRoot == "" {
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

func writeFilePlan(w io.Writer, res *files.ApplyResult) {
	if res == nil {
		return
	}
	entries := filePlanEntries(res)
	if len(entries) == 0 {
		return
	}
	fPrintln(w, "files:")
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	for _, e := range entries {
		fprintf(tw, "  %s\t%s\n", e.Kind, e.Target)
	}
	_ = tw.Flush()
}

func writeFileMismatchGuidance(w io.Writer, res *files.ApplyResult) {
	if res == nil || len(res.Mismatched) == 0 {
		return
	}
	for _, target := range res.Mismatched {
		fprintf(w, "genv apply: mismatch: %s\n", target)
	}
	fPrintln(w, "Hint: re-run with genv apply --force to overwrite, and add --backup (or per-entry backup: true) to preserve the existing file as *.backup.*")
}

func hasPostApplyHooks(f *schema.GenvFile) bool {
	return f != nil && f.Hooks != nil && len(f.Hooks.PostApply) > 0
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
		Backup:     opts.Backup,
	})
	if err == nil && !opts.DryRun {
		lf.Files = lockedFilesFromSpec(f.Files)
	}
	return res, err
}

type upgradeHookOptions struct {
	Phase    string
	Host     string
	Profile  string
	DryRun   bool
	Yes      bool
	Timeout  time.Duration
	Plan     []resolver.UpgradeAction
	Skipped  []resolver.SkippedPackage
	Upgraded []genvfile.LockedPackage
	Failed   []string
}

func runUpgradeHooks(ctx context.Context, f *schema.GenvFile, opts upgradeHookOptions) []string {
	if f == nil || f.Hooks == nil {
		return nil
	}
	exec := hooks.NewExecutor(os.Stdout, os.Stderr)
	runOpts := hooks.RunOptions{
		Host:    opts.Host,
		DryRun:  opts.DryRun,
		Env:     upgradeHookEnv(opts),
		Timeout: opts.Timeout,
		Stdin:   os.Stdin,
	}
	var err error
	switch opts.Phase {
	case "pre":
		if len(f.Hooks.PreUpgrade) > 0 {
			err = exec.PreUpgradeWithOptions(ctx, f.Hooks.PreUpgrade, runOpts)
		}
	case "post":
		if len(f.Hooks.PostUpgrade) > 0 {
			err = exec.PostUpgradeWithOptions(ctx, f.Hooks.PostUpgrade, runOpts)
		}
	}
	if err != nil {
		return []string{err.Error()}
	}
	return nil
}

func upgradeHookEnv(opts upgradeHookOptions) []string {
	phase := "pre-upgrade"
	if opts.Phase == "post" {
		phase = "post-upgrade"
	}
	return hookEnv(hookContext{Event: "upgrade", Phase: phase, Host: opts.Host, Profile: opts.Profile, DryRun: opts.DryRun, Yes: opts.Yes, Upgraded: upgradePackageIDs(opts.Upgraded), Failed: opts.Failed, Skipped: upgradeSkippedIDs(opts.Skipped), UpgradeManagers: upgradePlanManagers(opts.Plan)})
}

func upgradePlanManagers(plan []resolver.UpgradeAction) []string {
	seen := make(map[string]bool, len(plan))
	var managers []string
	for _, action := range plan {
		if len(action.LPs) == 0 {
			continue
		}
		manager := action.LPs[0].Manager
		if seen[manager] {
			continue
		}
		seen[manager] = true
		managers = append(managers, manager)
	}
	return managers
}

func upgradePackageIDs(pkgs []genvfile.LockedPackage) []string {
	ids := make([]string, len(pkgs))
	for i, pkg := range pkgs {
		ids[i] = pkg.ID
	}
	return ids
}

func upgradeSkippedIDs(skipped []resolver.SkippedPackage) []string {
	ids := make([]string, len(skipped))
	for i, item := range skipped {
		ids[i] = item.ID
	}
	return ids
}

func upgradeFailedIDs(plan []resolver.UpgradeAction, errs []error) []string {
	if len(errs) == 0 {
		return nil
	}
	var ids []string
	for _, action := range plan {
		if upgradeActionError(action, errs) == "" {
			continue
		}
		for _, pkg := range action.LPs {
			ids = append(ids, pkg.ID)
		}
	}
	return ids
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
	targetFlag := fs.String("target", "", "portable target id for schemaVersion 8 specs")

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

	targetID, exit := resolveMutationTarget("env set", *file, f, *targetFlag)
	if exit != exitOK {
		return exit
	}
	if err := commands.EnvSet(f, name, value, *sensitive, targetID); err != nil {
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
	targetFlag := fs.String("target", "", "portable target id for schemaVersion 8 specs")

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

	targetID, exit := resolveMutationTarget("env unset", *file, f, *targetFlag)
	if exit != exitOK {
		return exit
	}
	if err := commands.EnvUnset(f, name, targetID); err != nil {
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
	targetFlag := fs.String("target", "", "portable target id for schemaVersion 8 specs (defaults to $GENV_TARGET or host classification)")

	if err := fs.Parse(args); err != nil {
		return exitUsage
	}

	f, code := readMaterializedSpec("env list", *file, "", *targetFlag)
	if code != exitOK {
		return code
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
	targetFlag := fs.String("target", "", "portable target id for schemaVersion 8 specs")

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

	targetID, exit := resolveMutationTarget("shell alias set", *file, f, *targetFlag)
	if exit != exitOK {
		return exit
	}
	if err := commands.ShellAliasSet(f, name, value, *shell, targetID); err != nil {
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
	targetFlag := fs.String("target", "", "portable target id for schemaVersion 8 specs")

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

	targetID, exit := resolveMutationTarget("shell alias unset", *file, f, *targetFlag)
	if exit != exitOK {
		return exit
	}
	if err := commands.ShellAliasUnset(f, name, targetID); err != nil {
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
	targetFlag := fs.String("target", "", "portable target id for schemaVersion 8 specs (defaults to $GENV_TARGET or host classification)")

	if err := fs.Parse(args); err != nil {
		return exitUsage
	}

	f, code := readMaterializedSpec("shell status", *file, "", *targetFlag)
	if code != exitOK {
		return code
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

// applyShellCfg writes managed shell fragments via selected profile backends,
// updates lf.Shell in memory, and returns lists of applied and removed entry
// names. The caller writes the lock. If verbose is true, it prints progress.
func applyShellCfg(f *schema.GenvFile, lf *genvfile.LockFile, verbose bool) (applied, removed []string, err error) {
	if f.Shell == nil && lf.Shell == nil {
		return nil, nil, nil
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

	if warn := profilebackend.MissingEngineWarning(runtime.GOOS); warn != "" {
		// Avoid duplicate warning when applyEnvVars already printed it in the same run;
		// still useful when only shell is applied.
		if len(f.Env) == 0 && len(lf.Env) == 0 {
			fprintf(os.Stderr, "genv: warning: %s\n", warn)
		}
	}

	var cfg *schema.ShellConfig
	if f.Shell != nil {
		cfg = f.Shell
	}
	backends := profilebackend.SelectBackends(runtime.GOOS)
	for _, b := range backends {
		if err := b.ApplyShell(cfg); err != nil {
			fprintf(os.Stderr, "genv: writing shell fragment (%s): %v\n", b.Name(), err)
			return applied, removed, err
		}
	}

	if verbose {
		for _, name := range applied {
			fprintf(os.Stdout, "  shell: set %s\n", name)
		}
		for _, name := range removed {
			fprintf(os.Stdout, "  shell: removed %s\n", name)
		}
		if len(applied) > 0 || len(removed) > 0 {
			if fragPath, err := shellcfg.FragmentPath(); err == nil {
				fprintf(os.Stdout, "shell fragment written to %s\n", fragPath)
			}
		}
	}
	if hasFishEntries {
		fragPath, _ := shellcfg.FragmentPath()
		fprintf(os.Stdout, "note: fish-specific shell entries are not auto-applied.\n")
		fprintf(os.Stdout, "      Add '. %s' to ~/.config/fish/config.fish to source them.\n", fragPath)
	}

	// Update lf.Shell in memory; caller writes the lock once.
	lf.Shell = shellcfg.SpecToLock(f.Shell)

	return applied, removed, nil
}

// scanCmd implements `genv scan`.
// Discovers packages currently installed via available managers and adopts
// them into genv.json and the lock file. Use --dry-run to preview; text mode
// confirms unless --yes is set (JSON writes without a prompt, matching apply).
var scanGOOS = runtime.GOOS

type scanCandidate struct {
	id      string
	manager string
	pkgName string
	version string
}

func scanCmd(args []string) int {
	fs := flag.NewFlagSet("scan", flag.ContinueOnError)
	fs.Usage = func() {
		fPrintln(os.Stderr, "usage: genv scan [flags]")
		fPrintln(os.Stderr)
		fPrintln(os.Stderr, "Discover installed packages and adopt them into genv.json.")
		fPrintln(os.Stderr, "Preview with --dry-run. Text mode prompts unless --yes is set.")
		fPrintln(os.Stderr)
		fPrintln(os.Stderr, "flags:")
		fs.PrintDefaults()
	}

	file := fs.String("file", defaultSpecPath(), "path to genv.json")
	lockFile := fs.String("lock-file", "", "path to genv lock file")
	targetFlag := fs.String("target", "", "target ID for schemaVersion 8 (default: $GENV_TARGET or detected host target)")
	jsonOut := fs.Bool("json", false, "emit machine-readable JSON to stdout instead of human-readable text")
	dryRun := fs.Bool("dry-run", false, "list packages that would be adopted without writing the spec or lock")
	yes := fs.Bool("yes", false, "skip the confirmation prompt (for CI and scripts)")
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
	targetID, exit := resolveMutationTarget("scan", *file, f, *targetFlag)
	if exit != exitOK {
		return exit
	}

	available := resolver.Detect()
	if len(available) == 0 {
		if *jsonOut {
			return writeJSON(os.Stdout, output.Envelope{
				Version: output.SchemaVersion,
				Command: "scan",
				OK:      true,
				Data:    output.ScanResult{Added: 0, Skipped: 0, DryRun: *dryRun},
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

	// Build sets of already-tracked IDs so we can skip them. Besides the
	// friendly p.ID, register every manager-specific name from p.Managers:
	// adapters like mas report installed packages by their manager name
	// (a numeric App Store product ID) rather than the friendly ID, so a
	// package tracked as {"id":"xcode","managers":{"mas":"497799835"}} would
	// otherwise be re-adopted as a duplicate bare-numeric entry.
	trackedPackages := f.Packages
	if f.SchemaVersion == schema.Version8 {
		active, err := schema.MergeTarget(f, targetID)
		if err != nil {
			fprintf(os.Stderr, "genv scan: %v in %s\n", err, *file)
			return exitValidation
		}
		trackedPackages = active.Packages
	}
	trackedInSpec := make(map[string]bool, len(trackedPackages))
	for _, p := range trackedPackages {
		trackedInSpec[p.ID] = true
		for _, managerName := range p.Managers {
			trackedInSpec[managerName] = true
		}
	}

	seen := make(map[string]bool)
	var candidates []scanCandidate
	var skipped int

	for _, a := range scanAdaptersOnGOOS(available, scanGOOS) {
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
				continue
			}

			c := scanCandidate{id: pkgName, manager: a.Name(), pkgName: pkgName}
			if v, ok := versions[pkgName]; ok {
				c.version = v
			} else if v, err := a.QueryVersion(pkgName); err == nil {
				c.version = v
			}
			candidates = append(candidates, c)
			trackedInSpec[pkgName] = true // prevent duplicate IDs across managers in this pass
		}
	}

	if *dryRun {
		ids := make([]string, len(candidates))
		for i, c := range candidates {
			ids[i] = c.id
		}
		if *jsonOut {
			return writeJSON(os.Stdout, output.Envelope{
				Version: output.SchemaVersion,
				Command: "scan",
				OK:      true,
				Data:    output.ScanResult{Added: len(candidates), Skipped: skipped, DryRun: true, Packages: ids},
			})
		}
		if len(candidates) == 0 && skipped == 0 {
			fPrintln(os.Stdout, "no packages found.")
			return exitOK
		}
		fprintf(os.Stdout, "scan dry-run: would adopt %d package(s), %d already tracked\n", len(candidates), skipped)
		for _, c := range candidates {
			fprintf(os.Stdout, "  + %s  via %s\n", c.id, c.manager)
		}
		return exitOK
	}

	if len(candidates) > 0 && !*jsonOut && !*yes {
		if !confirm(fmt.Sprintf("This will adopt %d package(s) into genv.json. Continue? [y/N] ", len(candidates))) {
			fPrintln(os.Stdout, "Aborted.")
			return exitOK
		}
	}

	var added int
	for _, c := range candidates {
		if err := commands.Add(f, c.id, "", "", nil, targetID); err != nil {
			// ErrAlreadyTracked can race with trackedInSpec; skip silently.
			skipped++
			continue
		}
		lp := genvfile.LockedPackage{
			ID:               c.id,
			Manager:          c.manager,
			PkgName:          c.pkgName,
			InstalledVersion: c.version,
		}
		lf.Packages = append(lf.Packages, lp)
		added++
	}

	if added > 0 {
		if err := genvfile.Write(*file, f); err != nil {
			fprintf(os.Stderr, "genv: writing spec: %v\n", err)
			return exitIO
		}
		stampLockTarget(lf, targetID)
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

func scanAdaptersOnGOOS(available map[string]bool, goos string) []adapter.Adapter {
	selected := make([]adapter.Adapter, 0, len(adapter.All))
	for _, a := range adapter.All {
		if available[a.Name()] && adapter.AutomaticOnGOOS(a.Name(), goos) {
			selected = append(selected, a)
		}
	}
	return selected
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
	targetFlag := fs.String("target", "", "portable target id for schemaVersion 8 specs (defaults to $GENV_TARGET or host classification)")

	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	if *debug {
		logging.Init(true)
	}

	lockPath := lockPathForSpec(*file, *lockFile)
	lf, _ := genvfile.ReadLock(lockPath)
	activeProfile := ""
	if lf != nil {
		activeProfile = lf.ActiveProfile
	}

	f, err := profile.LoadMerged(*file, activeProfile)
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
	f, _, code := materializeSpecForCommand("status", *file, f, *hostFlag, *targetFlag)
	if code != exitOK {
		return code
	}

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

	if lf == nil {
		lf, err = genvfile.ReadLock(lockPathForSpec(*file, *lockFile))
		if err != nil {
			fprintf(os.Stderr, "genv: reading lock: %v\n", err)
			return exitIO
		}
	}

	entries := commands.Status(f, lf)
	envEntries := genvenv.EnvStatus(f.Env, lf.Env)
	shellEntries := shellcfg.ShellStatus(f.Shell, lf.Shell)
	serviceEntries := service.ServiceStatus(f.Services, lf.Services, true)

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
			Data:    output.StatusResult{ActiveProfile: activeProfile, Entries: jsonEntries, EnvEntries: jsonEnvEntries, ShellEntries: jsonShellEntries, ServiceEntries: jsonServiceEntries},
		})
	}

	if activeProfile != "" && activeProfile != profile.BaseProfileName {
		fprintf(os.Stdout, "Active profile: %s\n\n", activeProfile)
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
	if len(errs) == 0 {
		lf.Services = service.SpecToLock(f.Services)
	}

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
		fPrintln(os.Stderr, "  shell   One of: bash, zsh, fish, powershell")
		fPrintln(os.Stderr)
		fPrintln(os.Stderr, "examples:")
		fPrintln(os.Stderr, "  genv completion bash >> ~/.bashrc")
		fPrintln(os.Stderr, "  genv completion zsh  > ~/.zsh/completions/_genv")
		fPrintln(os.Stderr, "  genv completion fish > ~/.config/fish/completions/genv.fish")
		fPrintln(os.Stderr, "  genv completion powershell > ~/.config/genv/completions/genv.ps1")
		fPrintln(os.Stderr, "  genv completion install        # auto-detect the current shell")
		fPrintln(os.Stderr, "  genv completion install zsh    # install for a specific shell")
	}
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	if fs.NArg() < 1 {
		fPrintln(os.Stderr, "genv completion: missing shell argument (bash, zsh, fish, or powershell)")
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
	case "powershell":
		// Default under the genv config dir; users dot-source from their profile.
		dir, derr := genvfile.DefaultDir()
		if derr != nil {
			dir = filepath.Join(xdgConfigHome(), "genv")
		}
		return completionPowerShell, "genv.ps1", filepath.Join(dir, "completions"), nil
	case "":
		return "", "", "", fmt.Errorf("missing shell argument (bash, zsh, fish, or powershell)")
	default:
		return "", "", "", fmt.Errorf("unknown shell %q — supported shells are: bash, zsh, fish, powershell", shell)
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
		fPrintln(os.Stderr, "  shell   One of: bash, zsh, fish, powershell (default: detected from $SHELL)")
		fPrintln(os.Stderr, "  --dir   Install into this directory instead of the shell default")
	}
	shell, flagArgs := extractPositional(args)
	if err := fs.Parse(flagArgs); err != nil {
		return exitUsage
	}
	if shell == "" {
		shell = detectShell()
		if shell == "" {
			fPrintln(os.Stderr, "genv completion install: could not detect shell from $SHELL; pass one of: bash, zsh, fish, powershell")
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
	if shell == "powershell" {
		fprintf(os.Stdout, "Add this line to your PowerShell profile to enable completions:\n")
		fprintf(os.Stdout, "  . %s\n", path)
	}
	return exitOK
}

// detectShell returns "bash", "zsh", "fish", or "powershell" based on the
// basename of $SHELL, or "" when it is unset or unrecognized.
func detectShell() string {
	switch filepath.Base(os.Getenv("SHELL")) {
	case "bash":
		return "bash"
	case "zsh":
		return "zsh"
	case "fish":
		return "fish"
	case "pwsh", "powershell":
		return "powershell"
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
//   - repo-packages [prefix]    — repository package names (for add/adopt)
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
		effective, _, err := resolveEffectiveSpec(f, hostForCommand(""), "")
		if err != nil {
			return exitOK
		}
		for _, p := range effective.Packages {
			fPrintln(os.Stdout, p.ID)
		}
	case "managers":
		available := resolver.Detect()
		for _, a := range adapter.All {
			if available[a.Name()] {
				fPrintln(os.Stdout, a.Name())
			}
		}
	case "repo-packages":
		prefix := ""
		if len(args) > 1 {
			prefix = args[1]
		}
		available := resolver.Detect()
		for _, name := range complete.RepoPackages(prefix, available) {
			fPrintln(os.Stdout, name)
		}
	default:
		return exitUsage
	}
	return exitOK
}

// validateCmd implements `genv validate`.
// Reads and validates genv.json, then fails if any genv-managed launchd/systemd
// agent points at a missing or non-executable ProgramArguments[0]/ExecStart.
func validateCmd(args []string) int {
	fs := flag.NewFlagSet("validate", flag.ContinueOnError)
	fs.Usage = func() {
		fPrintln(os.Stderr, "usage: genv validate [flags]")
		fPrintln(os.Stderr)
		fPrintln(os.Stderr, "Validates genv.json and checks genv-managed supervisor agents for dangling executables.")
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
	if home, homeErr := managedAgentHomeDir(); homeErr == nil {
		if issues := service.ListManagedAgentProgramIssues(home); len(issues) > 0 {
			for _, issue := range issues {
				fprintf(os.Stderr, "genv validate: %s\n", issue.Detail)
			}
			fPrintln(os.Stderr, "Hint: re-run genv updates start (and re-apply services) so supervisor artifacts point at a live executable")
			return exitValidation
		}
	}
	return exitOK
}

// upgradeCmd implements `genv upgrade [--dry-run] [--yes] [--no-hooks] [--debug] [--all]`.
// By default plans only packages with a detected update; pass --all to plan every
// unconstrained tracked package without outdated filtering.
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
	noHooks := fs.Bool("no-hooks", false, "skip pre-upgrade and post-upgrade hooks")
	jsonOut := fs.Bool("json", false, "emit machine-readable JSON to stdout instead of human-readable text")
	debug := fs.Bool("debug", false, "emit debug-level structured logs to stderr")
	hostFlag := fs.String("host", "", "host name for host-specific records (defaults to $GENV_HOST or os.Hostname())")
	targetFlag := fs.String("target", "", "portable target id for schemaVersion 8 specs (defaults to $GENV_TARGET or host classification)")
	onlyFlag := fs.String("only", "", "comma-separated list of package IDs or names to upgrade")
	skipFlag := fs.String("skip", "", "comma-separated list of package IDs or names to skip")
	onlyManagerFlag := fs.String("only-manager", "", "comma-separated list of managers to upgrade")
	skipManagerFlag := fs.String("skip-manager", "", "comma-separated list of managers to skip")
	all := fs.Bool("all", false, "upgrade every unconstrained tracked package (skip outdated detection)")
	hookTimeoutFlag := fs.String("hook-timeout", "", "per-hook deadline, e.g. 5m or 30s (default: no hook timeout)")

	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	if *debug {
		logging.Init(true)
	}
	hookTimeout, err := parseOptionalDuration(*hookTimeoutFlag)
	if err != nil {
		fprintf(os.Stderr, "genv upgrade: invalid --hook-timeout %q: %v\n", *hookTimeoutFlag, err)
		return exitUsage
	}

	hostName := hostForCommand(*hostFlag)
	lockPath := lockPathForSpec(*file, *lockFile)
	lfPreview, _ := genvfile.ReadLock(lockPath)
	profileName := ""
	if lfPreview != nil {
		profileName = lfPreview.ActiveProfile
	}
	f, err := profile.LoadMerged(*file, profileName)
	if err != nil {
		if errors.Is(err, genvfile.ErrNotFound) {
			if *jsonOut {
				return writeJSON(os.Stdout, output.Envelope{
					Version: output.SchemaVersion,
					Command: "upgrade",
					OK:      false,
					Errors:  []string{err.Error()},
				})
			}
			fprintf(os.Stderr, "genv upgrade: %s not found — run 'genv init' to create one\n", *file)
			return exitIO
		}
		if *jsonOut {
			code := exitIO
			if errors.Is(err, genvfile.ErrInvalidFile) {
				code = exitValidation
			}
			_ = writeJSON(os.Stdout, output.Envelope{
				Version: output.SchemaVersion,
				Command: "upgrade",
				OK:      false,
				Errors:  []string{err.Error()},
			})
			return code
		}
		fprintf(os.Stderr, "genv upgrade: %v\n", err)
		if errors.Is(err, genvfile.ErrInvalidFile) {
			return exitValidation
		}
		return exitIO
	}
	f, activeTarget, code := materializeSpecForCommand("upgrade", *file, f, *hostFlag, *targetFlag)
	if code != exitOK {
		return code
	}

	unlock, err := genvfile.LockMutation(lockPath)
	if err != nil {
		if *jsonOut {
			_ = writeJSON(os.Stdout, output.Envelope{
				Version: output.SchemaVersion,
				Command: "upgrade",
				OK:      false,
				Errors:  []string{err.Error()},
			})
			return exitIO
		}
		fprintf(os.Stderr, "genv upgrade: locking %s: %v\n", lockPath, err)
		return exitIO
	}
	defer unlock()

	lf, err := genvfile.ReadLock(lockPath)
	if err != nil {
		if *jsonOut {
			_ = writeJSON(os.Stdout, output.Envelope{
				Version: output.SchemaVersion,
				Command: "upgrade",
				OK:      false,
				Errors:  []string{err.Error()},
			})
			return exitIO
		}
		fprintf(os.Stderr, "genv upgrade: reading lock: %v\n", err)
		return exitIO
	}
	if f.SchemaVersion == schema.Version8 {
		available := resolver.Detect()
		_, code := applyLockGate("upgrade", lockPath, lf, activeTarget, available, true, false, *dryRun, "")
		if code != exitOK {
			if *jsonOut {
				_ = writeJSON(os.Stdout, output.Envelope{
					Version: output.SchemaVersion,
					Command: "upgrade",
					OK:      false,
					Errors:  []string{"foreign lock refused"},
				})
			}
			return code
		}
	}
	only := parseCommaList(*onlyFlag)
	skip := parseCommaList(*skipFlag)
	onlyManager := parseCommaList(*onlyManagerFlag)
	skipManager := parseCommaList(*skipManagerFlag)

	for _, m := range onlyManager {
		if !schema.KnownManagers[m] {
			fprintf(os.Stderr, "genv upgrade: unknown manager %q in --only-manager\n", m)
			return exitUsage
		}
	}
	for _, m := range skipManager {
		if !schema.KnownManagers[m] {
			fprintf(os.Stderr, "genv upgrade: unknown manager %q in --skip-manager\n", m)
			return exitUsage
		}
	}

	filters := output.UpgradeFilters{
		Only:         only,
		Skip:         skip,
		OnlyManager:  onlyManager,
		SkipManager:  skipManager,
		HooksSkipped: *noHooks,
		All:          *all,
	}

	if len(lf.Packages) == 0 {
		if *jsonOut {
			return writeJSON(os.Stdout, output.Envelope{
				Version: output.SchemaVersion,
				Command: "upgrade",
				OK:      true,
				Data: output.UpgradeResult{
					DryRun:  *dryRun,
					Batches: []output.UpgradeBatch{},
					Filters: filters,
				},
			})
		}
		fPrintln(os.Stdout, "no packages tracked — run 'genv add' or 'genv scan' first.")
		return exitOK
	}

	planResult, err := upgrade.BuildUpgradePlan(upgrade.UpgradeOptions{
		Spec:    f,
		Lock:    lf,
		Filters: filters,
	})
	if err != nil {
		fprintf(os.Stderr, "genv upgrade: %v\n", err)
		return exitUsage
	}
	plan := planResult.Actions
	skipped := planResult.Skipped

	for _, w := range planResult.Warnings {
		if *dryRun && !*jsonOut {
			fprintf(os.Stderr, "genv upgrade: %s\n", w)
		}
	}

	if *jsonOut {
		return upgradeJSON(*dryRun, *yes, hostName, lockPath, hookTimeout, f, lf, plan, skipped, filters)
	}

	for _, s := range skipped {
		if s.Reason != "" {
			fprintf(os.Stderr, "genv upgrade: %s for %s — skipping\n", s.Reason, s.ID)
		} else {
			fprintf(os.Stderr, "genv upgrade: adapter %q not registered for %s — skipping\n", s.Manager, s.ID)
		}
	}

	if len(plan) == 0 {
		if !*dryRun && !*noHooks {
			ctx := context.Background()
			hookOpts := upgradeHookOptions{Host: hostName, Profile: lf.ActiveProfile, Yes: *yes, Timeout: hookTimeout, Plan: plan, Skipped: skipped}
			hookOpts.Phase = "pre"
			failedHooks := runUpgradeHooks(ctx, f, hookOpts)
			hookOpts.Phase = "post"
			failedHooks = append(failedHooks, runUpgradeHooks(ctx, f, hookOpts)...)
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
		ids := make([]string, len(a.LPs))
		for i, lp := range a.LPs {
			ids[i] = lp.ID
		}
		fprintf(os.Stdout, "  %s  via %s  ==> %s\n", strings.Join(ids, ", "), a.LPs[0].Manager, strings.Join(a.Cmd, " "))
	}

	if *dryRun {
		return exitOK
	}

	if !*yes && !confirm(fmt.Sprintf("\nUpgrade %d package(s)? [y/N] ", len(plan))) {
		fPrintln(os.Stdout, "Aborted.")
		return exitOK
	}

	ctx := context.Background()
	if !*noHooks {
		preHookOpts := upgradeHookOptions{Phase: "pre", Host: hostName, Profile: lf.ActiveProfile, Yes: *yes, Timeout: hookTimeout, Plan: plan, Skipped: skipped}
		failedHooks := runUpgradeHooks(ctx, f, preHookOpts)
		if len(failedHooks) > 0 {
			for _, e := range failedHooks {
				fprintf(os.Stderr, "genv upgrade: %s\n", e)
			}
			return exitLogic
		}
	}

	runResult := upgrade.RunUpgrade(ctx, upgrade.UpgradeRunOptions{
		Plan:     upgrade.UpgradePlan{Actions: plan, Skipped: skipped, Warnings: planResult.Warnings},
		Lock:     lf,
		LockPath: lockPath,
		Stdin:    os.Stdin,
		Stdout:   os.Stdout,
		Stderr:   os.Stderr,
	})

	exitCode := exitOK
	if len(runResult.Errors) > 0 {
		for _, err := range runResult.Errors {
			fprintf(os.Stderr, "genv upgrade: %v\n", err)
		}
		exitCode = exitLogic
	}
	if !*noHooks {
		postHookErrs := runUpgradeHooks(ctx, f, upgradeHookOptions{
			Phase:    "post",
			Host:     hostName,
			Profile:  lf.ActiveProfile,
			Yes:      *yes,
			Timeout:  hookTimeout,
			Plan:     plan,
			Skipped:  skipped,
			Upgraded: runResult.Upgraded,
			Failed:   upgradeFailedIDs(plan, runResult.Errors),
		})
		if len(postHookErrs) > 0 {
			for _, e := range postHookErrs {
				fprintf(os.Stderr, "genv upgrade: %s\n", e)
			}
			exitCode = exitLogic
		}
	}

	if runResult.LockWriteError != nil {
		fprintf(os.Stderr, "genv upgrade: %v\n", runResult.LockWriteError)
		return exitIO
	}
	if exitCode == exitOK {
		adviseUpdatesReregister(os.Stdout, runResult.Upgraded)
	}
	return exitCode
}

// upgradeBatchFromAction builds a machine-readable batch descriptor from a
// resolved upgrade action, with the given lifecycle status.
func upgradeBatchFromAction(a resolver.UpgradeAction, status string) output.UpgradeBatch {
	ids := make([]string, len(a.LPs))
	pkgNames := make([]string, len(a.LPs))
	for i, lp := range a.LPs {
		ids[i] = lp.ID
		pkgNames[i] = lp.PkgName
	}
	manager := ""
	if len(a.LPs) > 0 {
		manager = a.LPs[0].Manager
	}
	return output.UpgradeBatch{
		Manager:  manager,
		IDs:      ids,
		PkgNames: pkgNames,
		Cmd:      strings.Join(a.Cmd, " "),
		Status:   status,
	}
}

// upgradeSkippedEntries converts resolver skip records into JSON payload entries.
func upgradeSkippedEntries(skipped []resolver.SkippedPackage) []output.UpgradeSkipped {
	if len(skipped) == 0 {
		return nil
	}
	out := make([]output.UpgradeSkipped, len(skipped))
	for i, s := range skipped {
		out[i] = output.UpgradeSkipped{
			ID:      s.ID,
			Manager: s.Manager,
			Reason:  s.Reason,
		}
	}
	return out
}

// upgradeJSON emits a single JSON envelope for `genv upgrade --json`. In dry-run
// it plans only — no hooks, subprocesses, or lock write. In wet-run it routes
// all subprocess and hook output to stderr so stdout stays one JSON object,
// then reports executed batches, refreshed versions, and failed hooks while
// preserving the human path's exit codes.
func upgradeJSON(dryRun, yes bool, hostName, lockPath string, hookTimeout time.Duration, f *schema.GenvFile, lf *genvfile.LockFile, plan []resolver.UpgradeAction, skipped []resolver.SkippedPackage, filters output.UpgradeFilters) int {
	skippedEntries := upgradeSkippedEntries(skipped)

	if dryRun {
		batches := make([]output.UpgradeBatch, 0, len(plan))
		for _, a := range plan {
			batches = append(batches, upgradeBatchFromAction(a, "planned"))
		}
		return writeJSON(os.Stdout, output.Envelope{
			Version: output.SchemaVersion,
			Command: "upgrade",
			OK:      true,
			Data: output.UpgradeResult{
				DryRun:  true,
				Batches: batches,
				Skipped: skippedEntries,
				Filters: filters,
			},
		})
	}

	if len(plan) == 0 {
		ctx := context.Background()
		var failedHooks []output.UpgradeHookResult
		if !filters.HooksSkipped {
			hookOpts := upgradeHookOptions{Host: hostName, Profile: lf.ActiveProfile, Yes: yes, Timeout: hookTimeout, Plan: plan, Skipped: skipped}
			hookOpts.Phase = "pre"
			failedHooks = runUpgradeHooksJSON(ctx, f, hookOpts)
			hookOpts.Phase = "post"
			failedHooks = append(failedHooks, runUpgradeHooksJSON(ctx, f, hookOpts)...)
		}
		return writeJSON(os.Stdout, output.Envelope{
			Version: output.SchemaVersion,
			Command: "upgrade",
			OK:      len(failedHooks) == 0,
			Data: output.UpgradeResult{
				DryRun:      false,
				Batches:     []output.UpgradeBatch{},
				Skipped:     skippedEntries,
				FailedHooks: failedHooks,
				Filters:     filters,
			},
			Errors: upgradeHookErrorStrings(failedHooks),
		})
	}

	ctx := context.Background()
	var errs []string
	var preHooks []output.UpgradeHookResult
	if !filters.HooksSkipped {
		preHooks = runUpgradeHooksJSON(ctx, f, upgradeHookOptions{Phase: "pre", Host: hostName, Profile: lf.ActiveProfile, Yes: yes, Timeout: hookTimeout, Plan: plan, Skipped: skipped})
	}
	if len(preHooks) > 0 {
		errs = append(errs, upgradeHookErrorStrings(preHooks)...)
		return writeJSON(os.Stdout, output.Envelope{
			Version: output.SchemaVersion,
			Command: "upgrade",
			OK:      false,
			Data: output.UpgradeResult{
				DryRun:      false,
				Batches:     []output.UpgradeBatch{},
				Skipped:     skippedEntries,
				FailedHooks: preHooks,
				Filters:     filters,
			},
			Errors: errs,
		})
	}

	// Route subprocess stdout+stderr to stderr so stdout stays one JSON object.
	runResult := upgrade.RunUpgrade(ctx, upgrade.UpgradeRunOptions{
		Plan:     upgrade.UpgradePlan{Actions: plan, Skipped: skipped},
		Lock:     lf,
		LockPath: lockPath,
		Stdin:    os.Stdin,
		Stdout:   os.Stderr,
		Stderr:   os.Stderr,
	})
	batches := make([]output.UpgradeBatch, 0, len(plan))
	for _, a := range plan {
		status := "ok"
		if actionErr := upgradeActionError(a, runResult.Errors); actionErr != "" {
			status = "failed"
			batch := upgradeBatchFromAction(a, status)
			batch.Error = actionErr
			batches = append(batches, batch)
			continue
		}
		batches = append(batches, upgradeBatchFromAction(a, status))
	}
	if len(runResult.Errors) > 0 {
		errs = append(errs, errStrings(runResult.Errors)...)
	}

	var postHooks []output.UpgradeHookResult
	if !filters.HooksSkipped {
		postHooks = runUpgradeHooksJSON(ctx, f, upgradeHookOptions{
			Phase:    "post",
			Host:     hostName,
			Profile:  lf.ActiveProfile,
			Yes:      yes,
			Timeout:  hookTimeout,
			Plan:     plan,
			Skipped:  skipped,
			Upgraded: runResult.Upgraded,
			Failed:   upgradeFailedIDs(plan, runResult.Errors),
		})
	}
	errs = append(errs, upgradeHookErrorStrings(postHooks)...)
	if runResult.LockWriteError != nil {
		errs = append(errs, runResult.LockWriteError.Error())
	}

	updated := make([]output.UpgradePackage, 0, len(runResult.Upgraded))
	for _, u := range runResult.Upgraded {
		updated = append(updated, output.UpgradePackage{
			ID:         u.ID,
			Manager:    u.Manager,
			NewVersion: u.InstalledVersion,
		})
	}

	if len(errs) == 0 {
		adviseUpdatesReregister(os.Stderr, runResult.Upgraded)
	}

	return writeJSON(os.Stdout, output.Envelope{
		Version: output.SchemaVersion,
		Command: "upgrade",
		OK:      len(errs) == 0,
		Data: output.UpgradeResult{
			DryRun:      false,
			Batches:     batches,
			Updated:     updated,
			Skipped:     skippedEntries,
			FailedHooks: postHooks,
			Filters:     filters,
		},
		Errors: errs,
	})
}

// runUpgradeHooksJSON runs upgrade hooks for one phase with all hook output
// routed to stderr, returning a structured result per failed phase so the
// caller can embed it in the JSON payload.
func runUpgradeHooksJSON(ctx context.Context, f *schema.GenvFile, opts upgradeHookOptions) []output.UpgradeHookResult {
	if f == nil || f.Hooks == nil {
		return nil
	}
	exec := hooks.NewExecutor(os.Stderr, os.Stderr)
	runOpts := hooks.RunOptions{
		Host:    opts.Host,
		Env:     upgradeHookEnv(opts),
		Timeout: opts.Timeout,
		Stdin:   os.Stdin,
	}
	var err error
	switch opts.Phase {
	case "pre":
		if len(f.Hooks.PreUpgrade) > 0 {
			err = exec.PreUpgradeWithOptions(ctx, f.Hooks.PreUpgrade, runOpts)
		}
	case "post":
		if len(f.Hooks.PostUpgrade) > 0 {
			err = exec.PostUpgradeWithOptions(ctx, f.Hooks.PostUpgrade, runOpts)
		}
	}
	if err != nil {
		return []output.UpgradeHookResult{{Phase: opts.Phase, Error: err.Error()}}
	}
	return nil
}

// upgradeHookErrorStrings flattens hook results to the envelope-level errors slice.
func upgradeHookErrorStrings(results []output.UpgradeHookResult) []string {
	if len(results) == 0 {
		return nil
	}
	out := make([]string, len(results))
	for i, r := range results {
		out[i] = r.Error
	}
	return out
}

// upgradeActionError returns the first execution error naming this action.
func upgradeActionError(a resolver.UpgradeAction, errs []error) string {
	needle := upgradeActionIDKey(a)
	for _, e := range errs {
		if strings.Contains(e.Error(), needle) {
			return e.Error()
		}
	}
	return ""
}

// upgradeActionIDKey reproduces the %q-formatted id slice ExecuteUpgrade uses in
// its wrapped errors, so a batch can be correlated to its failure.
func upgradeActionIDKey(a resolver.UpgradeAction) string {
	ids := make([]string, len(a.LPs))
	for i, lp := range a.LPs {
		ids[i] = lp.ID
	}
	return fmt.Sprintf("%q", ids)
}

func parseCommaList(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	var out []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func parseOptionalDuration(raw string) (time.Duration, error) {
	if raw == "" {
		return 0, nil
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		return 0, err
	}
	if d < 0 {
		return 0, fmt.Errorf("duration must be non-negative")
	}
	return d, nil
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
	targetID, err := target.Resolve("")
	if err != nil {
		fprintf(os.Stderr, "genv init: %v\n", err)
		return exitUsage
	}
	reader := bufio.NewReader(os.Stdin)
	for {
		fprint(os.Stdout, "  package id (or Enter to finish): ")
		line, _ := reader.ReadString('\n')
		id := strings.TrimSpace(line)
		if id == "" {
			break
		}
		if err := commands.Add(f, id, "", "", nil, targetID); err != nil {
			if errors.Is(err, commands.ErrAlreadyTracked) {
				fprintf(os.Stdout, "  (skipping %q — already added)\n", id)
				continue
			}
			fprintf(os.Stderr, "genv init: %v\n", err)
			return exitLogic
		}
		fprintf(os.Stdout, "  added %s\n", id)
	}

	n := 0
	if b := f.Targets[targetID]; b != nil {
		n = len(b.Packages)
	}
	if n == 0 {
		fPrintln(os.Stdout, "\nNo packages entered. Run 'genv add <id>' to add packages later.")
	}

	if err := genvfile.Write(*file, f); err != nil {
		fprintf(os.Stderr, "genv init: %v\n", err)
		return exitIO
	}
	fprintf(os.Stdout, "\ncreated %s with %d package(s).\n", *file, n)
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
			if !strings.Contains(arg, "=") && !positionalBoolFlag(arg) && i+1 < len(args) {
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

func positionalBoolFlag(arg string) bool {
	name := strings.TrimLeft(arg, "-")
	switch name {
	case "no-search", "no-hooks", "dry-run", "force", "strict", "yes", "quiet", "json", "debug", "files", "sensitive":
		return true
	default:
		return false
	}
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
  scan        Discover installed packages and bulk-adopt them (use --dry-run / --yes)
  status      Show diff between genv.json, the lock file, and recorded versions
  clean       Clear the cache of all detected package managers
  edit        Open genv.json in $EDITOR
  env         Manage shell environment variables (set, unset, list)
  shell       Manage shell aliases and shell config drift
  service     Manage user-space services
  pull        Fetch genv.json from the configured spec repository
  migrate     Convert legacy host predicates to schemaVersion 8 targets
  completion  Print or install the shell completion script (bash, zsh, fish, or powershell)
  validate    Validate genv.json against the schema
  upgrade     Upgrade outdated tracked packages (--all for every unconstrained package)
  updates     Check for available updates to genv-tracked packages
  export      Build a single-target portable snapshot and report
  map         Print assist-only manager mapping suggestions for a target
  profile     Named overlays (list/create/switch; refused on schemaVersion 8)
  init        Create a new genv.json interactively
  version     Show genv build version information
  help        Show this help text

Flags common to all commands:
  --file <path>   Path to genv.json (default: $XDG_CONFIG_HOME/genv/genv.json or ~/.config/genv/genv.json, falling back to ./genv.json)

Host-specific flags (used by apply, status, upgrade, updates check/start, adopt):
  --host <name>   Legacy host filter for v1–v7 records (default: host classification, not hostname)
  --target <id>   Portable target id for schemaVersion 8 specs (default: $GENV_TARGET or host classification)

Add/Adopt-specific flags:
  --version <ver>              Version constraint, e.g. "0.10.*"
  --prefer <mgr>               Preferred manager, e.g. brew
  --manager <mgr:name,...>     Manager-specific package names, e.g.
                               snap:hello,brew:hello
  --no-hooks                   Skip add lifecycle hooks without skipping install
  --hook-timeout <duration>    Per-hook timeout, e.g. 5m or 30s
  --target <id>                Portable target id for schemaVersion 8 specs

Apply-specific flags:
  --dry-run            Print the reconcile plan without executing
  --strict             Exit with an error if any package cannot be resolved
  --yes                Skip the confirmation prompt (for CI and scripts)
  --quiet              Suppress plan output (useful in scripts)
  --json               Emit machine-readable JSON to stdout
  --timeout <duration> Per-subprocess timeout, e.g. 5m or 30s (0 = none)
  --no-hooks           Skip apply lifecycle hooks without skipping apply
  --hook-timeout <duration> Per-hook timeout, e.g. 5m or 30s
  --debug              Emit debug-level structured logs to stderr
  --target <id>        Portable target id for schemaVersion 8 specs
  --force-new-lock     Back up a foreign lock and start a new local lock

Export-specific flags:
  --target <id>        Target id to export
  --out <dir>          Directory to write genv.json, report.json, and report.md
  --strict             Exit nonzero if the report contains errors
  --from-v7            Migrate v1-v7 input to schemaVersion 8 in memory first

Map-specific flags:
  --target <id>        Destination target id for suggestions

Remove-specific flags:
  --no-hooks                Skip remove lifecycle hooks without skipping uninstall
  --hook-timeout <duration> Per-hook timeout, e.g. 5m or 30s
  --target <id>             Portable target id for schemaVersion 8 specs

Upgrade-specific flags:
  --dry-run                 Print the upgrade commands without executing
  --yes                     Skip the confirmation prompt
  --all                     Upgrade every unconstrained tracked package (skip outdated detection)
  --no-hooks                Skip pre-upgrade and post-upgrade hooks
  --json                    Emit machine-readable JSON to stdout
  --only <ids>              Comma-separated package IDs or names to upgrade
  --skip <ids>              Comma-separated package IDs or names to skip
  --only-manager <mgrs>     Comma-separated managers to upgrade
  --skip-manager <mgrs>     Comma-separated managers to skip
  --hook-timeout <duration> Per-hook timeout, e.g. 5m or 30s
  --debug                   Emit debug-level structured logs to stderr
  --target <id>             Portable target id for schemaVersion 8 specs

Updates-specific flags:
  check                         Plan available updates for genv-tracked packages only
  start                         Register the managed background checker (no auto-apply unless updates.autoApply:true)
  stop                          Stop and unregister the managed background checker
  status                        Show managed background checker status
  check --json                  Emit machine-readable dry-run JSON to stdout
  check --only <ids>            Comma-separated package IDs or names to check
  check --skip <ids>            Comma-separated package IDs or names to skip
  check --only-manager <mgrs>   Comma-separated managers to check
  check --skip-manager <mgrs>   Comma-separated managers to skip
  check --host <name>           Host name for host-specific records
  check --target <id>           Portable target id for schemaVersion 8 specs
  check --lock-file <path>      Path to genv lock file
  start --target <id>           Portable target id for schemaVersion 8 specs

Status-specific flags:
  --json    Emit machine-readable JSON to stdout
  --debug   Emit debug-level structured logs to stderr

Scan-specific flags:
  --dry-run   List packages that would be adopted without writing
  --yes       Skip the confirmation prompt
  --json      Emit machine-readable JSON to stdout
  --debug     Emit debug-level structured logs to stderr

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
	targetFlag := fs.String("target", "", "portable target id for schemaVersion 8 specs")

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

	targetID, exit := resolveMutationTarget("service add", *file, f, *targetFlag)
	if exit != exitOK {
		return exit
	}
	if err := commands.ServiceAdd(f, name, startCmd, stopCmd, restartCmd, statusCmd, *brewFormula, targetID); err != nil {
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
	targetFlag := fs.String("target", "", "portable target id for schemaVersion 8 specs")

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

	targetID, exit := resolveMutationTarget("service remove", *file, f, *targetFlag)
	if exit != exitOK {
		return exit
	}
	if err := commands.ServiceRemove(f, name, targetID); err != nil {
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
	targetFlag := fs.String("target", "", "portable target id for schemaVersion 8 specs (defaults to $GENV_TARGET or host classification)")

	if err := fs.Parse(args); err != nil {
		return exitUsage
	}

	f, code := readMaterializedSpec("service list", *file, "", *targetFlag)
	if code != exitOK {
		return code
	}

	commands.ServiceList(f, os.Stdout)
	return exitOK
}

// serviceStartCmd implements `genv service start <name>`.
func serviceStartCmd(args []string) int {
	fs := flag.NewFlagSet("service start", flag.ContinueOnError)
	file := fs.String("file", defaultSpecPath(), "path to genv.json")
	targetFlag := fs.String("target", "", "portable target id for schemaVersion 8 specs (defaults to $GENV_TARGET or host classification)")

	name, flagArgs := extractPositional(args)
	if err := fs.Parse(flagArgs); err != nil {
		return exitUsage
	}
	if name == "" {
		fPrintln(os.Stderr, "genv service start: name is required")
		return exitUsage
	}

	f, code := readMaterializedSpec("service start", *file, "", *targetFlag)
	if code != exitOK {
		return code
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
	targetFlag := fs.String("target", "", "portable target id for schemaVersion 8 specs (defaults to $GENV_TARGET or host classification)")

	name, flagArgs := extractPositional(args)
	if err := fs.Parse(flagArgs); err != nil {
		return exitUsage
	}
	if name == "" {
		fPrintln(os.Stderr, "genv service stop: name is required")
		return exitUsage
	}

	f, code := readMaterializedSpec("service stop", *file, "", *targetFlag)
	if code != exitOK {
		return code
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
	targetFlag := fs.String("target", "", "portable target id for schemaVersion 8 specs (defaults to $GENV_TARGET or host classification)")

	name, flagArgs := extractPositional(args)
	if err := fs.Parse(flagArgs); err != nil {
		return exitUsage
	}
	if name == "" {
		fPrintln(os.Stderr, "genv service status: name is required")
		return exitUsage
	}

	f, code := readMaterializedSpec("service status", *file, "", *targetFlag)
	if code != exitOK {
		return code
	}

	svc, ok := f.Services[name]
	if !ok {
		fprintf(os.Stderr, "genv: service %q not found in spec\n", name)
		return exitLogic
	}

	if svc.BrewFormula != "" {
		if service.BrewServicesRunning(svc.BrewFormula) {
			fprintf(os.Stdout, "service %q is running\n", name)
			return exitOK
		}
		fprintf(os.Stdout, "service %q is NOT running\n", name)
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
