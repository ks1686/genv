package main

import (
	"errors"
	"flag"
	"os"

	"github.com/ks1686/genv/internal/files"
	"github.com/ks1686/genv/internal/genvfile"
	"github.com/ks1686/genv/internal/schema"
)

// filesCmd implements `genv files <subcommand>`.
func filesCmd(args []string) int {
	if len(args) == 0 {
		printFilesUsage()
		return exitUsage
	}
	if isHelpArg(args[0]) {
		printFilesUsage()
		return exitOK
	}
	switch args[0] {
	case "adopt":
		return filesAdoptCmd(args[1:])
	default:
		fprintf(os.Stderr, "genv files: unknown subcommand %q\n\nRun 'genv files' for usage.\n", args[0])
		return exitUsage
	}
}

func printFilesUsage() {
	fPrintln(os.Stderr, "usage: genv files <adopt> [flags]")
	fPrintln(os.Stderr)
	fPrintln(os.Stderr, "subcommands:")
	fPrintln(os.Stderr, "  adopt <target>   Seed missing source from the live file, back it up, and link it")
}

// filesAdoptCmd implements `genv files adopt <target>`.
func filesAdoptCmd(args []string) int {
	fs := flag.NewFlagSet("files adopt", flag.ContinueOnError)
	fs.Usage = func() {
		fPrintln(os.Stderr, "usage: genv files adopt <target> [flags]")
		fPrintln(os.Stderr)
		fPrintln(os.Stderr, "flags:")
		fs.PrintDefaults()
	}
	file := fs.String("file", defaultSpecPath(), "path to genv.json")
	lockFile := fs.String("lock-file", "", "path to genv lock file")
	hostFlag := fs.String("host", "", "host name for host-specific records (defaults to host classification)")
	targetFlag := fs.String("target", "", "portable target id for schemaVersion 8 specs")
	dryRun := fs.Bool("dry-run", false, "print the seed/backup/link steps without writing")

	want, flagArgs := extractPositional(args)
	if err := fs.Parse(flagArgs); err != nil {
		return flagParseExit(err)
	}
	if want == "" {
		fPrintln(os.Stderr, "genv files adopt: missing target path")
		fs.Usage()
		return exitUsage
	}

	f, err := genvfile.Read(*file)
	if err != nil {
		if errors.Is(err, genvfile.ErrNotFound) {
			fprintf(os.Stderr, "genv: %s not found — run 'genv init' to create it\n", *file)
			return exitIO
		}
		fprintf(os.Stderr, "genv: %v\n", err)
		if errors.Is(err, genvfile.ErrInvalidFile) {
			return exitValidation
		}
		return exitIO
	}
	filtered, _, code := materializeSpecForCommand("files adopt", *file, f, *hostFlag, *targetFlag)
	if code != exitOK {
		return code
	}

	link, err := files.FindLinkByTarget(filtered.Files, want)
	if err != nil {
		fprintf(os.Stderr, "genv files adopt: %v\n", err)
		return exitLogic
	}
	source, err := files.ResolveSource(sourceRootForSpec(*file, f), link.Source)
	if err != nil {
		fprintf(os.Stderr, "genv files adopt: source %q: %v\n", link.Source, err)
		return exitLogic
	}
	target, err := files.ExpandPath(link.Target)
	if err != nil {
		fprintf(os.Stderr, "genv files adopt: target %q: %v\n", link.Target, err)
		return exitLogic
	}

	res, err := files.Adopt(source, target, files.AdoptOptions{DryRun: *dryRun})
	if err != nil {
		fprintf(os.Stderr, "genv files adopt: %v\n", err)
		return exitLogic
	}
	for _, step := range res.Steps {
		fPrintln(os.Stdout, step)
	}
	if *dryRun {
		return exitOK
	}

	lockPath := lockPathForSpec(*file, *lockFile)
	lf, err := genvfile.ReadLock(lockPath)
	if err != nil {
		fprintf(os.Stderr, "genv: reading lock: %v\n", err)
		return exitIO
	}
	hostName := hostForCommand(*hostFlag)
	adopted := lockedFilesFromSpec(&schema.FilesConfig{Links: []schema.FileLink{link}}, hostName, sourceRootForSpec(*file, f))
	lf.Files = mergeLockedFiles(lf.Files, adopted)
	if err := genvfile.WriteLock(lockPath, lf); err != nil {
		fprintf(os.Stderr, "genv: writing lock: %v\n", err)
		return exitIO
	}
	if len(res.Steps) == 0 {
		fprintf(os.Stdout, "adopted %s — already linked\n", target)
	}
	return exitOK
}
