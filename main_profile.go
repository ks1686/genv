package main

import (
	"flag"
	"os"

	"github.com/ks1686/genv/internal/genvfile"
	"github.com/ks1686/genv/internal/profile"
)

func profileCmd(args []string) int {
	if len(args) == 0 {
		printProfileUsage()
		return exitUsage
	}
	if isHelpArg(args[0]) {
		printProfileUsage()
		return exitOK
	}
	switch args[0] {
	case "list", "ls":
		return profileListCmd(args[1:])
	case "create":
		return profileCreateCmd(args[1:])
	case "switch":
		return profileSwitchCmd(args[1:])
	default:
		fprintf(os.Stderr, "genv profile: unknown subcommand %q\n\nRun 'genv profile' for usage.\n", args[0])
		return exitUsage
	}
}

func printProfileUsage() {
	fPrintln(os.Stderr, "usage: genv profile <list|create|switch> [flags]")
}

func profileListCmd(args []string) int {
	fs := flag.NewFlagSet("profile list", flag.ContinueOnError)
	file := fs.String("file", defaultSpecPath(), "path to genv.json")
	lockFile := fs.String("lock-file", "", "path to genv lock file")
	if err := fs.Parse(args); err != nil {
		return flagParseExit(err)
	}

	lockPath := lockPathForSpec(*file, *lockFile)
	lf, err := genvfile.ReadLock(lockPath)
	if err != nil {
		fprintf(os.Stderr, "genv profile list: reading lock: %v\n", err)
		return exitIO
	}
	active := lf.ActiveProfile
	if active == "" {
		active = "base"
	}

	profiles, err := profile.List(*file)
	if err != nil {
		fprintf(os.Stderr, "genv profile list: %v\n", err)
		return exitIO
	}

	if active == "base" {
		fPrintln(os.Stdout, "* base")
	} else {
		fPrintln(os.Stdout, "  base")
	}

	for _, p := range profiles {
		if p == active {
			fprintf(os.Stdout, "* %s\n", p)
		} else {
			fprintf(os.Stdout, "  %s\n", p)
		}
	}
	return exitOK
}

func profileCreateCmd(args []string) int {
	fs := flag.NewFlagSet("profile create", flag.ContinueOnError)
	file := fs.String("file", defaultSpecPath(), "path to genv.json")
	name, flagArgs := extractPositional(args)
	if err := fs.Parse(flagArgs); err != nil {
		return flagParseExit(err)
	}
	if name == "" {
		fPrintln(os.Stderr, "genv profile create: missing profile name")
		return exitUsage
	}
	if name == "base" {
		fPrintln(os.Stderr, "genv profile create: 'base' is a reserved profile name")
		return exitUsage
	}

	if err := profile.Create(*file, name); err != nil {
		fprintf(os.Stderr, "genv profile create: %v\n", err)
		return exitIO
	}
	fprintf(os.Stdout, "Created profile %q\n", name)
	return exitOK
}

func profileSwitchCmd(args []string) int {
	fs := flag.NewFlagSet("profile switch", flag.ContinueOnError)
	opts := applyOptions{}
	fs.StringVar(&opts.File, "file", defaultSpecPath(), "path to genv.json")
	fs.StringVar(&opts.LockFile, "lock-file", "", "path to genv lock file")
	fs.StringVar(&opts.StateDir, "state-dir", "", "directory for lock and env/shell fragments (default: directory of --file)")
	fs.BoolVar(&opts.DryRun, "dry-run", false, "print the reconcile plan without executing")
	fs.BoolVar(&opts.Force, "force", false, "overwrite mismatched managed files")
	fs.BoolVar(&opts.Strict, "strict", false, "exit with an error if any package cannot be resolved")
	fs.BoolVar(&opts.Yes, "yes", false, "skip the confirmation prompt (for CI and scripts)")
	fs.BoolVar(&opts.Quiet, "quiet", false, "suppress plan output (useful in scripts)")
	fs.BoolVar(&opts.JSONOut, "json", false, "emit machine-readable JSON to stdout instead of human-readable text")
	fs.DurationVar(&opts.Timeout, "timeout", 0, "per-subprocess timeout")
	fs.BoolVar(&opts.Debug, "debug", false, "emit debug-level structured logs to stderr")
	fs.StringVar(&opts.Host, "host", "", "host name for host-specific records")

	name, flagArgs := extractPositional(args)
	if err := fs.Parse(flagArgs); err != nil {
		return flagParseExit(err)
	}
	if name == "" {
		fPrintln(os.Stderr, "genv profile switch: missing profile name")
		return exitUsage
	}
	opts.TargetProfile = name

	return runApply(opts)
}
