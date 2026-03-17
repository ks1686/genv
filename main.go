package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings" // for parseManagerFlag and extractPositional

	"github.com/ks1686/gpm/internal/commands"
	"github.com/ks1686/gpm/internal/gpmfile"
	"github.com/ks1686/gpm/internal/resolver"
)

// Structured exit codes.
const (
	exitOK         = 0 // success
	exitUsage      = 1 // bad arguments or unknown command
	exitIO         = 2 // filesystem or serialisation error
	exitValidation = 3 // gpm.json fails schema validation
	exitLogic      = 4 // semantic error (duplicate id, not found, etc.)
)

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
	case "list", "ls":
		return listCmd(args[1:])
	case "install":
		return installCmd(args[1:])
	case "help", "--help", "-h":
		printUsage()
		return exitOK
	default:
		fmt.Fprintf(os.Stderr, "gpm: unknown command %q\n\nRun 'gpm help' for usage.\n", args[0])
		return exitUsage
	}
}

// addCmd implements `gpm add <id> [--version <ver>] [--prefer <mgr>] [--manager mgr:name,...]`.
func addCmd(args []string) int {
	fs := flag.NewFlagSet("add", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: gpm add <id> [flags]")
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "flags:")
		fs.PrintDefaults()
	}

	file := fs.String("file", gpmfile.DefaultPath, "path to gpm.json")
	version := fs.String("version", "", `version constraint, e.g. "0.10.*" (default: omitted, meaning any)`)
	prefer := fs.String("prefer", "", "preferred package manager (e.g. brew)")
	managerFlag := fs.String("manager", "", `manager-specific names, comma-separated mgr:name pairs (e.g. flatpak:org.mozilla.firefox,brew:firefox)`)

	id, flagArgs := extractPositional(args)
	if err := fs.Parse(flagArgs); err != nil {
		// flag.ContinueOnError already printed the error.
		return exitUsage
	}
	if id == "" {
		fmt.Fprintln(os.Stderr, "gpm add: missing package id")
		fs.Usage()
		return exitUsage
	}

	managers, err := parseManagerFlag(*managerFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "gpm add: --manager: %v\n", err)
		return exitUsage
	}

	f, isNew, err := gpmfile.ReadOrNew(*file)
	if err != nil {
		fmt.Fprintf(os.Stderr, "gpm: %v\n", err)
		if errors.Is(err, gpmfile.ErrInvalidFile) {
			return exitValidation
		}
		return exitIO
	}

	if err := commands.Add(f, id, *version, *prefer, managers); err != nil {
		fmt.Fprintf(os.Stderr, "gpm: %v\n", err)
		if errors.Is(err, commands.ErrAlreadyTracked) {
			return exitLogic
		}
		return exitUsage
	}

	if err := gpmfile.Write(*file, f); err != nil {
		fmt.Fprintf(os.Stderr, "gpm: %v\n", err)
		return exitIO
	}

	if isNew {
		fmt.Fprintf(os.Stdout, "created %s\n", *file)
	}
	fmt.Fprintf(os.Stdout, "added %s\n", id)
	return exitOK
}

// removeCmd implements `gpm remove <id>`.
func removeCmd(args []string) int {
	fs := flag.NewFlagSet("remove", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: gpm remove <id> [flags]")
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "flags:")
		fs.PrintDefaults()
	}

	file := fs.String("file", gpmfile.DefaultPath, "path to gpm.json")

	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	if fs.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "gpm remove: missing package id")
		fs.Usage()
		return exitUsage
	}
	id := fs.Arg(0)

	f, err := gpmfile.Read(*file)
	if err != nil {
		if errors.Is(err, gpmfile.ErrNotFound) {
			fmt.Fprintf(os.Stderr, "gpm: %s not found\n", *file)
			return exitLogic
		}
		fmt.Fprintf(os.Stderr, "gpm: %v\n", err)
		if errors.Is(err, gpmfile.ErrInvalidFile) {
			return exitValidation
		}
		return exitIO
	}

	if err := commands.Remove(f, id); err != nil {
		fmt.Fprintf(os.Stderr, "gpm: %v\n", err)
		return exitLogic
	}

	if err := gpmfile.Write(*file, f); err != nil {
		fmt.Fprintf(os.Stderr, "gpm: %v\n", err)
		return exitIO
	}

	fmt.Fprintf(os.Stdout, "removed %s\n", id)
	return exitOK
}

// listCmd implements `gpm list`.
func listCmd(args []string) int {
	fs := flag.NewFlagSet("list", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: gpm list [flags]")
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "flags:")
		fs.PrintDefaults()
	}

	file := fs.String("file", gpmfile.DefaultPath, "path to gpm.json")

	if err := fs.Parse(args); err != nil {
		return exitUsage
	}

	f, err := gpmfile.Read(*file)
	if err != nil {
		if errors.Is(err, gpmfile.ErrNotFound) {
			commands.List(nil, os.Stdout)
			return exitOK
		}
		fmt.Fprintf(os.Stderr, "gpm: %v\n", err)
		if errors.Is(err, gpmfile.ErrInvalidFile) {
			return exitValidation
		}
		return exitIO
	}

	commands.List(f, os.Stdout)
	return exitOK
}

// installCmd implements `gpm install [--dry-run] [--strict]`.
func installCmd(args []string) int {
	fs := flag.NewFlagSet("install", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: gpm install [flags]")
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "flags:")
		fs.PrintDefaults()
	}

	file := fs.String("file", gpmfile.DefaultPath, "path to gpm.json")
	dryRun := fs.Bool("dry-run", false, "print the install plan without executing")
	strict := fs.Bool("strict", false, "exit with an error if any package cannot be resolved")

	if err := fs.Parse(args); err != nil {
		return exitUsage
	}

	f, err := gpmfile.Read(*file)
	if err != nil {
		if errors.Is(err, gpmfile.ErrNotFound) {
			fmt.Fprintf(os.Stderr, "gpm: %s not found — run 'gpm add' to create it\n", *file)
			return exitIO
		}
		fmt.Fprintf(os.Stderr, "gpm: %v\n", err)
		if errors.Is(err, gpmfile.ErrInvalidFile) {
			return exitValidation
		}
		return exitIO
	}

	if len(f.Packages) == 0 {
		fmt.Fprintln(os.Stdout, "nothing to install.")
		return exitOK
	}

	available := resolver.Detect()
	actions := resolver.Plan(f, available)
	resolvedCount, unresolvedCount := resolver.PrintPlan(actions, os.Stdout)

	if unresolvedCount > 0 && *strict {
		fmt.Fprintf(os.Stderr, "gpm install: %d package(s) unresolved; aborting (--strict)\n", unresolvedCount)
		return exitLogic
	}

	if *dryRun {
		return exitOK
	}

	if resolvedCount == 0 {
		fmt.Fprintln(os.Stdout, "nothing to install.")
		return exitOK
	}

	// Confirmation prompt.
	fmt.Fprintf(os.Stdout, "This will install %d package(s). Continue? [y/N] ", resolvedCount)
	reader := bufio.NewReader(os.Stdin)
	answer, _ := reader.ReadString('\n')
	answer = strings.TrimSpace(answer)
	if answer != "y" && answer != "Y" {
		fmt.Fprintln(os.Stdout, "Aborted.")
		return exitOK
	}

	// Execute; pass the same buffered reader so any buffered bytes aren't lost
	// to child processes (e.g. sudo password prompts).
	errs := resolver.Execute(actions, reader, os.Stdout, os.Stderr)
	if len(errs) > 0 {
		for _, e := range errs {
			fmt.Fprintf(os.Stderr, "gpm install: %v\n", e)
		}
		return exitLogic
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
	for _, token := range strings.Split(s, ",") {
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
	fmt.Fprint(os.Stderr, `gpm — global package manager

Usage:
  gpm <command> [flags]

Commands:
  add <id>    Track a new package
  remove <id> Stop tracking a package    (alias: rm)
  list        List all tracked packages  (alias: ls)
  install     Install all tracked packages
  help        Show this help text

Flags common to all commands:
  --file <path>   Path to gpm.json (default: ./gpm.json)

Add-specific flags:
  --version <ver>              Version constraint, e.g. "0.10.*"
  --prefer <mgr>               Preferred manager, e.g. brew
  --manager <mgr:name,...>     Manager-specific package names, e.g.
                               flatpak:org.mozilla.firefox,brew:firefox

Install-specific flags:
  --dry-run   Print the install plan without executing
  --strict    Exit with an error if any package cannot be resolved

`)
	fmt.Fprintf(os.Stderr, "Supported package managers:\n  %s\n", commands.KnownManagerList())
}
