package resolver

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"

	"github.com/ks1686/genv/internal/adapter"
	"github.com/ks1686/genv/internal/genvfile"
)

// RefreshAction is one index-refresh command shown in the human plan.
// It is executed during planning, before FilterOutdated, and must not be
// re-run by ExecuteUpgrade.
type RefreshAction struct {
	Manager string
	Cmd     []string
}

// RefreshOptions controls how RefreshIndexes runs manager fetch commands.
type RefreshOptions struct {
	Context context.Context
	Stdin   io.Reader
	Stdout  io.Writer
	Stderr  io.Writer
}

// DefaultIndexRefreshTimeout bounds one index refresh so a hung
// `brew update` / `apt-get update` cannot wedge the hourly worker.
// Same family as DefaultLiveListTimeout, raised because a cold brew
// update routinely exceeds 30s. Var so tests can shorten it.
var DefaultIndexRefreshTimeout = 2 * time.Minute

// runIndexRefresh executes one refresh argv. Var so tests can inject.
var runIndexRefresh = defaultRunIndexRefresh

func defaultRunIndexRefresh(ctx context.Context, argv []string, stdin io.Reader, stdout, stderr io.Writer) error {
	if len(argv) == 0 || argv[0] == "" {
		return fmt.Errorf("empty refresh command")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	if stdin != nil {
		cmd.Stdin = stdin
	}
	if stdout != nil {
		cmd.Stdout = stdout
	}
	if stderr != nil {
		cmd.Stderr = stderr
	}
	err := cmd.Run()
	if ctx.Err() != nil {
		return fmt.Errorf("%s: timed out: %w", strings.Join(argv, " "), ctx.Err())
	}
	return err
}

// RefreshIndexes runs each IndexRefresher once for managers that still have
// candidate packages. Identical argv (brew and linuxbrew share `brew update`)
// is executed once. A failed or timed-out refresh marks every manager that
// shared that argv as keep-all, matching a failed ListOutdated.
func RefreshIndexes(packages []genvfile.LockedPackage, opts RefreshOptions) (actions []RefreshAction, keepAll map[string]bool, warnings []string) {
	keepAll = map[string]bool{}
	parent := opts.Context
	if parent == nil {
		parent = context.Background()
	}

	type pending struct {
		managers []string
		cmd      []string
	}
	seenMgr := make(map[string]bool)
	var order []string
	for _, lp := range packages {
		if seenMgr[lp.Manager] {
			continue
		}
		seenMgr[lp.Manager] = true
		order = append(order, lp.Manager)
	}

	seenArgv := make(map[string]int)
	var jobs []pending
	for _, name := range order {
		mgr := lookupAdapter(name)
		if mgr == nil || !mgr.Available() {
			if mgr != nil {
				warnings = append(warnings, fmt.Sprintf("skipping refresh for %s: manager is not available", name))
			}
			continue
		}
		refresher, ok := mgr.(adapter.IndexRefresher)
		if !ok {
			continue
		}
		cmd := refresher.PlanRefresh()
		if len(cmd) == 0 {
			continue
		}
		key := strings.Join(cmd, "\x00")
		if i, ok := seenArgv[key]; ok {
			jobs[i].managers = append(jobs[i].managers, name)
			continue
		}
		seenArgv[key] = len(jobs)
		jobs = append(jobs, pending{managers: []string{name}, cmd: cmd})
	}

	for _, job := range jobs {
		display := job.managers[0]
		actions = append(actions, RefreshAction{Manager: display, Cmd: job.cmd})
		runCtx, cancel := boundIndexRefresh(parent)
		started := time.Now()
		err := runIndexRefresh(runCtx, job.cmd, opts.Stdin, opts.Stdout, opts.Stderr)
		elapsed := time.Since(started).Round(time.Millisecond)
		cancel()
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("could not refresh %s after %s (%v) — keeping all", display, elapsed, err))
			for _, name := range job.managers {
				keepAll[name] = true
			}
			continue
		}
		warnings = append(warnings, fmt.Sprintf("refresh timing: %s took %s", display, elapsed))
	}
	return actions, keepAll, warnings
}

func boundIndexRefresh(parent context.Context) (context.Context, context.CancelFunc) {
	budget := DefaultIndexRefreshTimeout
	if dl, ok := parent.Deadline(); ok {
		if rem := time.Until(dl); rem > 0 && rem < budget {
			budget = rem
		}
	}
	return context.WithTimeout(parent, budget)
}
