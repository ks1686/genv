package main

import (
	"errors"
	"flag"
	"os"
	"strings"

	exportpkg "github.com/ks1686/genv/internal/export"
	"github.com/ks1686/genv/internal/genvfile"
	"github.com/ks1686/genv/internal/schema"
)

func mapCmd(args []string) int {
	fs := flag.NewFlagSet("map", flag.ContinueOnError)
	fs.Usage = func() {
		fPrintln(os.Stderr, "usage: genv map --target <id> [flags]")
		fPrintln(os.Stderr)
		fPrintln(os.Stderr, "Print assist-only manager mapping suggestions for a destination target.")
		fPrintln(os.Stderr)
		fPrintln(os.Stderr, "flags:")
		fs.PrintDefaults()
	}

	file := fs.String("file", defaultSpecPath(), "path to genv.json")
	targetID := fs.String("target", "", "destination target id")

	if err := fs.Parse(args); err != nil {
		return flagParseExit(err)
	}
	if fs.NArg() != 0 {
		fs.Usage()
		return exitUsage
	}
	if strings.TrimSpace(*targetID) == "" {
		fPrintln(os.Stderr, "genv map: --target is required")
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
	if f.SchemaVersion != schema.Version8 {
		fprintf(os.Stderr, "genv map: schemaVersion %q is not mappable; run 'genv migrate --write' first\n", f.SchemaVersion)
		return exitUsage
	}

	report := exportpkg.Suggest(f, *targetID)
	printMapSuggestions(*targetID, report)
	return exitOK
}

func printMapSuggestions(targetID string, report []exportpkg.ReportItem) {
	if len(report) == 0 {
		fprintf(os.Stdout, "No mapping suggestions for target %s.\n", targetID)
		return
	}
	fprintf(os.Stdout, "Mapping suggestions for target %s:\n", targetID)
	for _, item := range report {
		if item.PackageID == "" {
			fprintf(os.Stdout, "- [%s] %s\n", item.Code, item.Message)
			continue
		}
		fprintf(os.Stdout, "- [%s] %s: %s\n", item.Code, item.PackageID, item.Message)
	}
}
