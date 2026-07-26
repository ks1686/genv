package main

import (
	"errors"
	"flag"
	"os"
	"path/filepath"
	"strings"

	exportpkg "github.com/ks1686/genv/internal/export"
	"github.com/ks1686/genv/internal/genvfile"
	"github.com/ks1686/genv/internal/migrate"
	"github.com/ks1686/genv/internal/schema"
)

func exportCmd(args []string) int {
	fs := flag.NewFlagSet("export", flag.ContinueOnError)
	fs.Usage = func() {
		fPrintln(os.Stderr, "usage: genv export --target <id> --out <dir> [flags]")
		fPrintln(os.Stderr)
		fPrintln(os.Stderr, "Build a portable schemaVersion 8 snapshot for one target plus report.json.")
		fPrintln(os.Stderr)
		fPrintln(os.Stderr, "flags:")
		fs.PrintDefaults()
	}

	file := fs.String("file", defaultSpecPath(), "path to genv.json")
	targetID := fs.String("target", "", "target id to export")
	outDir := fs.String("out", "", "directory to write genv.json and report.json")
	strict := fs.Bool("strict", false, "exit nonzero when the report contains error-class items")
	fromV7 := fs.Bool("from-v7", false, "migrate a v1-v7 spec to schemaVersion 8 in memory before exporting")

	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	if fs.NArg() != 0 {
		fs.Usage()
		return exitUsage
	}
	if strings.TrimSpace(*targetID) == "" {
		fPrintln(os.Stderr, "genv export: --target is required")
		return exitUsage
	}
	if strings.TrimSpace(*outDir) == "" {
		fPrintln(os.Stderr, "genv export: --out is required")
		return exitUsage
	}

	f, err := genvfile.Read(*file)
	if err != nil {
		if errors.Is(err, genvfile.ErrNotFound) {
			fprintf(os.Stderr, "genv: %s not found\n", *file)
			return exitIO
		}
		fprintf(os.Stderr, "genv: %v\n", err)
		if errors.Is(err, genvfile.ErrInvalidFile) {
			return exitValidation
		}
		return exitIO
	}
	if *fromV7 {
		migrated, warnings, err := migrate.ToV8(f)
		if err != nil {
			fprintf(os.Stderr, "genv export: %v\n", err)
			return exitLogic
		}
		for _, warning := range warnings {
			fprintf(os.Stderr, "genv export: warning: %s\n", warning)
		}
		f = migrated
	} else if f.SchemaVersion != schema.Version8 {
		fprintf(os.Stderr, "genv export: schemaVersion %q is not exportable; rerun with --from-v7 to migrate in memory\n", f.SchemaVersion)
		return exitUsage
	}

	report, err := exportpkg.BuildWithOptions(f, *targetID, *outDir, exportpkg.Options{BaseDir: filepath.Dir(*file)})
	if err != nil {
		fprintf(os.Stderr, "genv export: %v\n", err)
		if errors.Is(err, genvfile.ErrInvalidFile) {
			return exitValidation
		}
		return exitIO
	}
	fprintf(os.Stdout, "exported target %s to %s\n", *targetID, *outDir)
	if *strict && report.HasErrors() {
		fprintf(os.Stderr, "genv export: report contains error-class items\n")
		return exitLogic
	}
	return exitOK
}
