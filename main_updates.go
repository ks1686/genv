package main

import (
	"errors"
	"flag"
	"io"
	"os"
	"strings"

	"github.com/ks1686/genv/internal/genvfile"
	"github.com/ks1686/genv/internal/output"
	"github.com/ks1686/genv/internal/resolver"
	"github.com/ks1686/genv/internal/upgrade"
)

func updatesCmd(args []string) int {
	if len(args) == 0 {
		printUpdatesUsage()
		return exitUsage
	}
	switch args[0] {
	case "help", "--help", "-h":
		printUpdatesUsage()
		return exitOK
	case "check":
		return updatesCheckCmd(args[1:])
	case "start":
		return updatesStartCmd(args[1:])
	case "stop":
		return updatesStopCmd(args[1:])
	case "status":
		return updatesStatusCmd(args[1:])
	case "__run-once":
		return updatesRunOnceCmd(args[1:])
	default:
		fprintf(os.Stderr, "genv updates: unknown subcommand %q\n\nRun 'genv updates' for usage.\n", args[0])
		return exitUsage
	}
}

func printUpdatesUsage() {
	fPrintln(os.Stderr, "usage: genv updates <check|start|stop|status> [flags]")
	fPrintln(os.Stderr)
	fPrintln(os.Stderr, "subcommands:")
	fPrintln(os.Stderr, "  check   Plan available updates for genv-tracked packages only")
	fPrintln(os.Stderr, "  start   Register the managed background updates checker")
	fPrintln(os.Stderr, "  stop    Stop and unregister the managed background updates checker")
	fPrintln(os.Stderr, "  status  Show managed background updates checker status")
}

func updatesCheckCmd(args []string) int {
	fs := flag.NewFlagSet("updates check", flag.ContinueOnError)
	fs.Usage = func() {
		fPrintln(os.Stderr, "usage: genv updates check [flags]")
		fPrintln(os.Stderr)
		fPrintln(os.Stderr, "flags:")
		fs.PrintDefaults()
	}
	file := fs.String("file", defaultSpecPath(), "path to genv.json")
	lockFile := fs.String("lock-file", "", "path to genv lock file")
	jsonOut := fs.Bool("json", false, "emit machine-readable JSON to stdout instead of human-readable text")
	hostFlag := fs.String("host", "", "host name for host-specific records (defaults to $GENV_HOST or os.Hostname())")
	targetFlag := fs.String("target", "", "portable target id for schemaVersion 8 specs (defaults to $GENV_TARGET or host classification)")
	onlyFlag := fs.String("only", "", "comma-separated list of package IDs or names to check")
	skipFlag := fs.String("skip", "", "comma-separated list of package IDs or names to skip")
	onlyManagerFlag := fs.String("only-manager", "", "comma-separated list of managers to check")
	skipManagerFlag := fs.String("skip-manager", "", "comma-separated list of managers to skip")
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}

	f, err := genvfile.Read(*file)
	if err != nil {
		return updatesCheckReadSpecError(*file, *jsonOut, err)
	}
	f, _, code := materializeSpecForCommand("updates check", *file, f, *hostFlag, *targetFlag)
	if code != exitOK {
		return code
	}

	lockPath := lockPathForSpec(*file, *lockFile)
	if _, err := os.Stat(lockPath); err != nil {
		return updatesCheckReadLockError(*jsonOut, err)
	}
	lf, err := genvfile.ReadLock(lockPath)
	if err != nil {
		return updatesCheckReadLockError(*jsonOut, err)
	}

	filters := output.UpgradeFilters{
		Only:         parseCommaList(*onlyFlag),
		Skip:         parseCommaList(*skipFlag),
		OnlyManager:  parseCommaList(*onlyManagerFlag),
		SkipManager:  parseCommaList(*skipManagerFlag),
		HooksSkipped: true,
	}
	plan, err := upgrade.BuildUpgradePlan(upgrade.UpgradeOptions{Spec: f, Lock: lf, Filters: filters})
	if err != nil {
		if *jsonOut {
			_ = writeJSON(os.Stdout, output.Envelope{Version: output.SchemaVersion, Command: "updates check", OK: false, Errors: []string{err.Error()}})
		} else {
			fprintf(os.Stderr, "genv updates check: %v\n", err)
		}
		return exitUsage
	}

	if *jsonOut {
		return updatesCheckJSON(plan.Actions, plan.Skipped, filters)
	}
	return updatesCheckHuman(os.Stdout, plan)
}

func updatesCheckReadSpecError(file string, jsonOut bool, err error) int {
	if jsonOut {
		return writeJSON(os.Stdout, output.Envelope{Version: output.SchemaVersion, Command: "updates check", OK: false, Errors: []string{err.Error()}})
	}
	if errors.Is(err, genvfile.ErrNotFound) {
		fprintf(os.Stderr, "genv updates check: %s not found — run 'genv init' to create one\n", file)
		return exitIO
	}
	fprintf(os.Stderr, "genv updates check: %v\n", err)
	if errors.Is(err, genvfile.ErrInvalidFile) {
		return exitValidation
	}
	return exitIO
}

func updatesCheckReadLockError(jsonOut bool, err error) int {
	if jsonOut {
		_ = writeJSON(os.Stdout, output.Envelope{Version: output.SchemaVersion, Command: "updates check", OK: false, Errors: []string{err.Error()}})
		return exitIO
	}
	fprintf(os.Stderr, "genv updates check: reading lock: %v\n", err)
	return exitIO
}

func updatesCheckJSON(plan []resolver.UpgradeAction, skipped []resolver.SkippedPackage, filters output.UpgradeFilters) int {
	batches := make([]output.UpgradeBatch, 0, len(plan))
	for _, a := range plan {
		batches = append(batches, upgradeBatchFromAction(a, "planned"))
	}
	return writeJSON(os.Stdout, output.Envelope{
		Version: output.SchemaVersion,
		Command: "updates check",
		OK:      true,
		Data: output.UpgradeResult{
			DryRun:  true,
			Batches: batches,
			Skipped: upgradeSkippedEntries(skipped),
			Filters: filters,
		},
	})
}

func updatesCheckHuman(w io.Writer, plan upgrade.UpgradePlan) int {
	for _, warn := range plan.Warnings {
		fprintf(os.Stderr, "genv updates check: %s\n", warn)
	}
	fPrintln(w, "updates check for genv-tracked packages only:")
	for _, skipped := range plan.Skipped {
		if skipped.Reason != "" {
			fprintf(os.Stderr, "genv updates check: %s for %s — skipping\n", skipped.Reason, skipped.ID)
		} else {
			fprintf(os.Stderr, "genv updates check: adapter %q not registered for %s — skipping\n", skipped.Manager, skipped.ID)
		}
	}
	if len(plan.Actions) == 0 {
		fPrintln(w, "no upgradeable genv-tracked packages found.")
		return exitOK
	}
	fPrintln(w, "available update plan:")
	for _, action := range plan.Actions {
		ids := make([]string, len(action.LPs))
		for i, lp := range action.LPs {
			ids[i] = lp.ID
		}
		fprintf(w, "  %s  via %s  ==> %s\n", strings.Join(ids, ", "), action.LPs[0].Manager, strings.Join(action.Cmd, " "))
	}
	return exitOK
}
