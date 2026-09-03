// Package adapter defines the Adapter interface that every package manager
// must implement, along with the ordered registry of all known adapters.
package adapter

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Searchable is an optional extension of Adapter for managers that support
// searching their package repositories by keyword. Not all managers expose
// a useful search command, so this is a separate interface checked via
// type assertion rather than a required method on Adapter.
type Searchable interface {
	// Search returns package names from this manager's repository that match
	// query. Implementations should filter to names containing query
	// (case-insensitive) where the underlying command doesn't already do so.
	// Returns nil, nil when no results are found or the manager is unavailable.
	Search(query string) ([]string, error)
}

// ContextSearchable lets latency-sensitive callers cancel repository searches.
type ContextSearchable interface {
	SearchContext(ctx context.Context, query string) ([]string, error)
}

// NameLister is an optional extension for managers that can dump installable
// package names quickly enough for shell completion (Homebrew-style).
type NameLister interface {
	ListNames() ([]string, error)
}

// ContextNameLister lets latency-sensitive callers cancel repository dumps.
type ContextNameLister interface {
	ListNamesContext(ctx context.Context) ([]string, error)
}

// CompletionNamer is an optional extension for managers whose Tab labels should
// differ from Search/install package names (e.g. mas app names vs product IDs).
type CompletionNamer interface {
	CompletionNames(prefix string) ([]string, error)
}

// ContextCompletionNamer lets latency-sensitive callers cancel label lookups.
type ContextCompletionNamer interface {
	CompletionNamesContext(ctx context.Context, prefix string) ([]string, error)
}

// VersionLister is an optional extension of Adapter for managers whose list
// command reports installed package versions in the same call.
type VersionLister interface {
	ListInstalledVersions() (map[string]string, error)
}

// BatchUpgrader is an optional extension of Adapter for managers that can
// upgrade multiple named packages in a single command while leaving untracked
// packages alone. When implemented, the resolver groups tracked packages by
// manager and invokes PlanUpgradeBatch instead of running one command per
// package.
type BatchUpgrader interface {
	PlanUpgradeBatch(pkgNames []string) []string
}

// OutdatedLister is an optional extension of Adapter for managers that can
// report which installed packages have a newer version available. pkgNames are
// the manager-native names genv is tracking; implementations MAY use them to
// scope the query or ignore them and report every outdated package. Returns a
// map of manager-native package name -> latest available version; a package is
// considered up to date when it is absent from the map. Returns nil, nil when
// nothing is outdated or the manager is unavailable.
type OutdatedLister interface {
	ListOutdated(pkgNames []string) (map[string]string, error)
}

// TrackOnly is an optional marker for adapters that record a package in the
// lock without spawning an installer (official/manual installers).
type TrackOnly interface {
	TrackOnly()
}

// Adapter is the capability contract every package manager must satisfy.
// Each method maps to one of the four resolver operations: detect, query,
// plan install, and normalize package IDs.
type Adapter interface {
	// Name returns the canonical manager identifier used in genv.json
	// (e.g. "pacman", "brew", "winget").
	Name() string

	// Available reports whether this manager's binary exists in PATH.
	Available() bool

	// NormalizeID returns the concrete package name for this manager.
	// managers is the optional per-manager name overrides from the genv.json
	// "managers" field. Returns the resolved name and true if an explicit
	// mapping was found; returns id and false when falling back to the ID.
	NormalizeID(id string, managers map[string]string) (name string, explicit bool)

	// PlanInstall returns the command argv to install pkgName via this manager.
	PlanInstall(pkgName string) []string

	// PlanUninstall returns the command argv to uninstall pkgName via this manager.
	PlanUninstall(pkgName string) []string

	// PlanUpgrade returns the command argv to upgrade pkgName to the latest
	// version satisfying the active constraints. For managers where the install
	// command already upgrades (e.g. pacman -S), this may equal PlanInstall.
	PlanUpgrade(pkgName string) []string

	// PlanClean returns zero or more commands to purge cached data for this
	// manager. Each inner slice is an independent command (argv). Returns nil
	// when the manager has no meaningful cache-clean operation.
	PlanClean() [][]string

	// Query reports whether pkgName is already installed on this host.
	// Returns false, nil when the package is absent (not an error condition).
	Query(pkgName string) (bool, error)

	// ListInstalled returns the concrete package names of all packages currently
	// installed via this manager. Returns nil, nil when the manager is unavailable
	// or no packages are installed. Names are manager-specific identifiers, not genv IDs.
	ListInstalled() ([]string, error)

	// QueryVersion returns the installed version string for pkgName.
	// Returns "", nil when the package is not installed or the version cannot be
	// determined. Version strings are manager-specific and not normalized.
	QueryVersion(pkgName string) (string, error)
}

// All is the ordered registry of every known adapter.
// The slice order determines fallback priority: when no preference is
// specified in genv.json the first available adapter wins.
var All = []Adapter{
	Brew{},
	Mas{},
	Pacman{},
	Paru{},
	Yay{},
	Apt{},
	Dnf{},
	Apk{},
	Snap{},
	Linuxbrew{},
	Bun{},
	Npm{},
	Pnpm{},
	Yarn{},
	Deno{},
	Volta{},
	Uv{},
	Pipx{},
	PipUser{},
	Poetry{},
	Conda{},
	Mamba{},
	Pixi{},
	Cargo{},
	Go{},
	Rustup{},
	Gem{},
	Composer{},
	DotnetTool{},
	Ghcup{},
	Stack{},
	Opam{},
	Juliaup{},
	Sdkman{},
	Asdf{},
	Mise{},
	Krew{},
	Helm{},
	Vscode{},
	Winget{},
	Scoop{},
	Choco{},
	External{},
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

// Absent reports whether pkgName is not installed via a. Query errors are not
// treated as absent — the caller should still attempt the planned mutation.
func Absent(a Adapter, pkgName string) bool {
	if a == nil {
		return false
	}
	installed, err := a.Query(pkgName)
	return err == nil && !installed
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

// probeTimeout bounds every read-only inventory subprocess (list, query,
// version, outdated, availability probes). Package managers are external
// programs that can block indefinitely — winget famously stalls for minutes
// on fresh profiles while it initializes its sources — and genv must stay
// responsive instead of wedging behind them. Mutating commands (install,
// uninstall, upgrade) are deliberately NOT bounded by this constant: they
// legitimately run long and are governed by the caller-supplied context.
// Var so tests can shorten the deadline.
var probeTimeout = 30 * time.Second

// probeWaitDelay bounds how long a probe waits for inherited output pipes to
// close after the process itself is gone. Without it, a killed manager's
// grandchildren (shims, `sh -c` wrappers, .cmd→bash chains on Windows) keep
// the stdout pipe open and the probe still blocks until they exit.
var probeWaitDelay = 2 * time.Second

// runProbe runs cmd with args under probeTimeout and returns raw stdout.
// A non-zero child exit code is reported as *exec.ExitError so callers can
// apply their usual exit-code conventions; a deadline overrun is reported as
// a distinct timeout error that is never an *exec.ExitError.
func runProbe(cmd string, args ...string) ([]byte, error) {
	return runProbeContext(context.Background(), cmd, args...)
}

// runProbeCombined is runProbe but returns combined stdout+stderr, for
// managers that print their inventory on stderr (e.g. dnf check-update).
func runProbeCombined(cmd string, args ...string) ([]byte, error) {
	return runProbeCombinedContext(context.Background(), cmd, args...)
}

// runProbeContext is runProbe under a caller-owned context. When that context
// carries no deadline, probeTimeout is applied — every probe gets SOME bound,
// whichever entry point it arrives through.
func runProbeContext(parent context.Context, cmd string, args ...string) ([]byte, error) {
	ctx, cancel, budget := boundProbe(parent)
	defer cancel()
	cmdEx := exec.CommandContext(ctx, cmd, args...)
	cmdEx.WaitDelay = probeWaitDelay
	out, err := cmdEx.Output()
	return checkProbeErr(ctx, budget, cmd, args, out, err)
}

// runProbeCombinedContext is runProbeCombined under a caller-owned context,
// with the same no-deadline-means-probeTimeout rule.
func runProbeCombinedContext(parent context.Context, cmd string, args ...string) ([]byte, error) {
	ctx, cancel, budget := boundProbe(parent)
	defer cancel()
	cmdEx := exec.CommandContext(ctx, cmd, args...)
	cmdEx.WaitDelay = probeWaitDelay
	out, err := cmdEx.CombinedOutput()
	return checkProbeErr(ctx, budget, cmd, args, out, err)
}

// boundProbe applies probeTimeout when parent has no deadline. It returns the
// context to run under, a cancel the caller must defer, and the effective
// budget for error messages. The cancel is a no-op when parent already had a
// deadline — cancelling an inherited parent is never the probe's call.
func boundProbe(parent context.Context) (context.Context, context.CancelFunc, time.Duration) {
	if dl, ok := parent.Deadline(); ok {
		budget := probeTimeout
		if remaining := time.Until(dl); remaining > 0 {
			budget = remaining
		}
		return parent, func() {}, budget
	}
	ctx, cancel := context.WithTimeout(parent, probeTimeout)
	return ctx, cancel, probeTimeout
}

// checkProbeErr reclassifies a context-driven kill as a timeout error.
// CommandContext kills the child on deadline, which surfaces as an
// ExitError ("signal: killed") rather than context.DeadlineExceeded, so the
// owning context is the only reliable signal. Without this reclassification,
// a hung manager would be indistinguishable from "not installed".
func checkProbeErr(ctx context.Context, budget time.Duration, cmd string, args []string, out []byte, err error) ([]byte, error) {
	if err == nil {
		return out, nil
	}
	if ctx.Err() != nil || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return nil, fmt.Errorf("%s %s: timed out after %s", cmd, strings.Join(args, " "), budget)
	}
	return out, err
}

// runQuery executes cmd with args and interprets the exit status as an
// installed/absent signal. A non-zero exit code means "not installed"
// (false, nil). Only an OS-level execution failure is returned as an error.
// Bounded by probeTimeout so a hung manager cannot stall detection.
func runQuery(cmd string, args ...string) (bool, error) {
	_, err := runProbe(cmd, args...)
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
	return runListOutputContext(context.Background(), cmd, args...)
}

// runListOutputContext is runListOutput with subprocess cancellation. When the
// caller supplies no deadline, probeTimeout still applies.
func runListOutputContext(ctx context.Context, cmd string, args ...string) ([]string, error) {
	out, err := runProbeContext(ctx, cmd, args...)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return nil, nil
		}
		return nil, err
	}
	return trimmedNonEmptyLines(string(out)), nil
}

// runVersionOutput runs cmd and returns trimmed stdout as the version string.
// A non-zero exit code is treated as "not installed" ("", nil), not an error.
func runVersionOutput(cmd string, args ...string) (string, error) {
	out, err := runProbe(cmd, args...)
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return "", nil
		}
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// parsePacmanSearch parses the output of pacman/paru/yay -Ss and returns
// package names whose name contains query (case-insensitive).
// Output alternates: "repo/name version ..." package lines and indented
// description lines; only package lines are examined.
func parsePacmanSearch(lines []string, query string) []string {
	var names []string
	for _, line := range lines {
		if strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
			continue
		}
		pkgField := strings.Fields(line)[0]
		if _, name, ok := strings.Cut(pkgField, "/"); ok {
			if containsFold(name, query) {
				names = append(names, name)
			}
		}
	}
	return names
}

// parseMgrQueryVersion extracts the version from "pkgname version" output,
// as produced by pacman-style query commands (pacman -Q, paru -Q, yay -Q).
func parseMgrQueryVersion(out string) string {
	if parts := strings.SplitN(out, " ", 2); len(parts) == 2 {
		return parts[1]
	}
	return ""
}
