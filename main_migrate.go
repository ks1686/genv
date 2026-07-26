package main

import (
	"encoding/json"
	"errors"
	"flag"
	"os"

	"github.com/ks1686/genv/internal/genvfile"
	"github.com/ks1686/genv/internal/migrate"
)

// migrateCmd implements `genv migrate [--file] [--write]`.
func migrateCmd(args []string) int {
	fs := flag.NewFlagSet("migrate", flag.ContinueOnError)
	fs.Usage = func() {
		fPrintln(os.Stderr, "usage: genv migrate [flags]")
		fPrintln(os.Stderr)
		fPrintln(os.Stderr, "Convert a legacy genv.json with host predicates to schemaVersion 8 targets.")
		fPrintln(os.Stderr)
		fPrintln(os.Stderr, "flags:")
		fs.PrintDefaults()
	}

	file := fs.String("file", defaultSpecPath(), "path to genv.json")
	write := fs.Bool("write", false, "overwrite genv.json with the migrated schemaVersion 8 spec")

	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	if fs.NArg() != 0 {
		fs.Usage()
		return exitUsage
	}
	if *file == "-" && *write {
		fPrintln(os.Stderr, "genv migrate: cannot use --write with --file -")
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

	out, warnings, err := migrate.ToV8(f)
	if err != nil {
		fprintf(os.Stderr, "genv migrate: %v\n", err)
		return exitLogic
	}
	for _, warning := range warnings {
		fprintf(os.Stderr, "genv migrate: warning: %s\n", warning)
	}

	if *write {
		if err := genvfile.Write(*file, out); err != nil {
			fprintf(os.Stderr, "genv migrate: writing spec: %v\n", err)
			if errors.Is(err, genvfile.ErrInvalidFile) {
				return exitValidation
			}
			return exitIO
		}
		fprintf(os.Stdout, "migrated %s to schemaVersion 8\n", *file)
		return exitOK
	}

	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		fprintf(os.Stderr, "genv migrate: serializing migrated spec: %v\n", err)
		return exitIO
	}
	data = append(data, '\n')
	if _, err := os.Stdout.Write(data); err != nil {
		fprintf(os.Stderr, "genv migrate: writing output: %v\n", err)
		return exitIO
	}
	return exitOK
}
