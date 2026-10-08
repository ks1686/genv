package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/ks1686/genv/internal/compose"
	"github.com/ks1686/genv/internal/genvfile"
	"github.com/ks1686/genv/internal/schema"
)

// genv config — what the environment is for a target, and where it came from.
//
//	genv config [--target macos] [--json]
//	genv config --kind package --name jq
//
// Without a selector it prints the summary: the selected modules, the composed
// fingerprint, and per-kind counts. With a selector it prints every contributor
// to that one resource, which is the question "why is jq here?" that a
// composed spec makes possible.
func configCmd(args []string) int {
	fs := flag.NewFlagSet("config", flag.ContinueOnError)
	fs.Usage = func() {
		fPrintln(os.Stderr, "usage: genv config [flags]")
		fPrintln(os.Stderr)
		fPrintln(os.Stderr, "Show the composed environment for a target and where each resource came from.")
		fPrintln(os.Stderr)
		fPrintln(os.Stderr, "flags:")
		fs.PrintDefaults()
	}
	file := fs.String("file", defaultSpecPath(), "path to genv.json")
	targetFlag := fs.String("target", "", targetFlagHelp)
	jsonOut := fs.Bool("json", false, "emit machine-readable JSON instead of text")
	kind := fs.String("kind", "", "resource kind to inspect (package, service, env, alias, function, file, dir)")
	name := fs.String("name", "", "resource name or path to inspect")
	registryOnly := fs.Bool("registry", false, "list registered modules without composing")

	if err := fs.Parse(args); err != nil {
		return flagParseExit(err)
	}
	if (*kind == "") != (*name == "") {
		fPrintln(os.Stderr, "genv config: --kind and --name must be given together")
		return exitUsage
	}

	f, err := genvfile.Read(*file)
	if err != nil {
		return reportSpecReadError("config", *file, err)
	}

	if *registryOnly {
		return printModuleRegistry(f, *jsonOut)
	}

	c, code := materializeComposition("config", *file, f, "", *targetFlag, composeSourceRoot(*file, ""))
	if code != exitOK {
		return code
	}

	if *kind != "" {
		return printResourceOwners(c, *kind, *name, *jsonOut)
	}
	return printComposedSummary(c, *jsonOut)
}

// printModuleRegistry lists what is registered and, where it is selected, where.
func printModuleRegistry(f *schema.GenvFile, jsonOut bool) int {
	type entry struct {
		Name     string   `json:"name"`
		Path     string   `json:"path"`
		Selected []string `json:"selectedBy,omitempty"`
	}
	selected := map[string][]string{}
	for targetID, bundle := range f.Targets {
		if bundle == nil {
			continue
		}
		for _, name := range bundle.UseModules {
			selected[name] = append(selected[name], targetID)
		}
	}
	names := make([]string, 0, len(f.Modules))
	for name := range f.Modules {
		names = append(names, name)
	}
	sort.Strings(names)

	entries := make([]entry, 0, len(names))
	for _, name := range names {
		targets := selected[name]
		sort.Strings(targets)
		entries = append(entries, entry{Name: name, Path: f.Modules[name], Selected: targets})
	}

	if jsonOut {
		return printJSON(struct {
			Target  string  `json:"target,omitempty"`
			Modules []entry `json:"modules"`
		}{Modules: entries})
	}
	if len(entries) == 0 {
		fPrintln(os.Stdout, "no modules registered")
		return exitOK
	}
	fPrintln(os.Stdout, "registered modules:")
	for _, e := range entries {
		line := fmt.Sprintf("  %-20s %s", e.Name, e.Path)
		if len(e.Selected) == 0 {
			line += "  (not selected by any target)"
		} else {
			line += "  (selected by: " + strings.Join(e.Selected, ", ") + ")"
		}
		fPrintln(os.Stdout, line)
	}
	return exitOK
}

// composedCounts is the per-kind tally shown by the summary.
type composedCounts struct {
	Packages int `json:"packages"`
	Services int `json:"services"`
	Env      int `json:"env"`
	Aliases  int `json:"aliases"`
	Funcs    int `json:"functions"`
	Files    int `json:"files"`
	Dirs     int `json:"dirs"`
	Hooks    int `json:"hooks"`
}

func countComposition(e *schema.GenvFile) composedCounts {
	c := composedCounts{
		Packages: len(e.Packages),
		Services: len(e.Services),
		Env:      len(e.Env),
	}
	if e.Shell != nil {
		c.Aliases = len(e.Shell.Aliases)
		c.Funcs = len(e.Shell.Functions)
	}
	if e.Files != nil {
		c.Files = len(e.Files.Links) + len(e.Files.Templates)
		c.Dirs = len(e.Files.Dirs)
	}
	if e.Hooks != nil {
		c.Hooks = len(e.Hooks.PreApply) + len(e.Hooks.PostApply)
	}
	return c
}

// printComposedSummary answers "what would this target actually get?".
func printComposedSummary(c *compose.Composition, jsonOut bool) int {
	counts := countComposition(c.Effective)
	modules := c.SelectedModules
	if modules == nil {
		modules = []string{}
	}
	if jsonOut {
		return printJSON(struct {
			Target      string         `json:"target,omitempty"`
			Modules     []string       `json:"modules"`
			Fingerprint string         `json:"fingerprint,omitempty"`
			Counts      composedCounts `json:"counts"`
		}{
			Target:      c.Target,
			Modules:     modules,
			Fingerprint: c.Fingerprint,
			Counts:      counts,
		})
	}

	fPrintln(os.Stdout, fmt.Sprintf("target: %s", c.Target))
	if len(modules) == 0 {
		fPrintln(os.Stdout, "modules: none")
	} else {
		fPrintln(os.Stdout, "modules: "+strings.Join(modules, ", "))
	}
	fPrintln(os.Stdout, fmt.Sprintf("fingerprint: %s", c.Fingerprint))
	fPrintln(os.Stdout, "resources:")
	fPrintln(os.Stdout, fmt.Sprintf("  packages   %d", counts.Packages))
	fPrintln(os.Stdout, fmt.Sprintf("  services   %d", counts.Services))
	fPrintln(os.Stdout, fmt.Sprintf("  env        %d", counts.Env))
	fPrintln(os.Stdout, fmt.Sprintf("  aliases    %d", counts.Aliases))
	fPrintln(os.Stdout, fmt.Sprintf("  functions  %d", counts.Funcs))
	fPrintln(os.Stdout, fmt.Sprintf("  files      %d", counts.Files))
	fPrintln(os.Stdout, fmt.Sprintf("  dirs       %d", counts.Dirs))
	fPrintln(os.Stdout, fmt.Sprintf("  hooks      %d", counts.Hooks))
	return exitOK
}

// resourceOwner is one contributor to a single resource.
type resourceOwner struct {
	Module  string `json:"module"`
	Origin  string `json:"origin"`
	Field   string `json:"field,omitempty"`
	Target  string `json:"target,omitempty"`
	Primary bool   `json:"primary,omitempty"`
}

// printResourceOwners answers "why is this resource here, and who may change it?".
func printResourceOwners(c *compose.Composition, kind, name string, jsonOut bool) int {
	if !compose.IsKnownKind(kind) {
		fprintf(os.Stderr, "genv config: unknown kind %q; expected one of %s\n", kind, strings.Join(compose.KnownKinds(), ", "))
		return exitUsage
	}
	id := compose.Identity{Kind: kind, Key: name}
	owners := c.Provenance.Owners(id)

	type payload struct {
		Target   string          `json:"target,omitempty"`
		Kind     string          `json:"kind"`
		Name     string          `json:"name"`
		Found    bool            `json:"found"`
		Owners   []resourceOwner `json:"owners"`
		Editable bool            `json:"editable"`
	}
	out := payload{Target: c.Target, Kind: kind, Name: name, Owners: []resourceOwner{}, Editable: true}

	for i, o := range owners {
		out.Owners = append(out.Owners, resourceOwner{
			Module:  o.Module,
			Origin:  o.Document,
			Field:   o.Field,
			Target:  o.Target,
			Primary: i == 0,
		})
		if o.Module != "" {
			out.Editable = false
		}
	}
	out.Found = len(owners) > 0

	if jsonOut {
		return printJSON(out)
	}
	if !out.Found {
		fPrintln(os.Stdout, fmt.Sprintf("%s %q is not part of this environment", kind, name))
		return exitOK
	}
	fPrintln(os.Stdout, fmt.Sprintf("%s %q for target %s:", kind, name, c.Target))
	for _, o := range out.Owners {
		who := "genv.json (root)"
		if o.Module != "" {
			who = fmt.Sprintf("module %q", o.Module)
		}
		line := fmt.Sprintf("  %-28s %s", who, o.Origin)
		if o.Field != "" {
			line += "  " + o.Field
		}
		if o.Target != "" {
			line += " [" + o.Target + "]"
		}
		fPrintln(os.Stdout, line)
	}
	if !out.Editable {
		fPrintln(os.Stdout, "\nthis resource is owned by a module; edit that module rather than running a mutation command")
	}
	return exitOK
}

func printJSON(v any) int {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		fprintf(os.Stderr, "genv config: %v\n", err)
		return exitIO
	}
	fprintf(os.Stdout, "%s\n", data)
	return exitOK
}

// genv explain — why a resource is here, in prose, with the alternatives.
//
// explain is config with an opinion: it names the merge that won, so a user
// reading it learns how composition decides, not just what it decided.
func explainCmd(args []string) int {
	fs := flag.NewFlagSet("explain", flag.ContinueOnError)
	fs.Usage = func() {
		fPrintln(os.Stderr, "usage: genv explain <kind> <name> [--target macos] [--json]")
		fPrintln(os.Stderr)
		fPrintln(os.Stderr, "Explain where a resource comes from and whether this command may change it.")
		fPrintln(os.Stderr)
		fPrintln(os.Stderr, "kinds: "+strings.Join(compose.KnownKinds(), ", "))
		fPrintln(os.Stderr)
		fPrintln(os.Stderr, "flags:")
		fs.PrintDefaults()
	}
	file := fs.String("file", defaultSpecPath(), "path to genv.json")
	targetFlag := fs.String("target", "", targetFlagHelp)
	jsonOut := fs.Bool("json", false, "emit machine-readable JSON instead of text")

	// Positionals are extracted first: Go's flag parser stops at the first
	// non-flag argument, so `explain package jq --target macos` would otherwise
	// silently ignore --target and compose the wrong environment.
	pos, flagArgs := splitPositionals(args, 2)
	var kind, name string
	if len(pos) > 0 {
		kind = pos[0]
	}
	if len(pos) > 1 {
		name = pos[1]
	}
	if err := fs.Parse(flagArgs); err != nil {
		return flagParseExit(err)
	}
	if kind == "" || name == "" {
		fPrintln(os.Stderr, "genv explain: expected a kind and a name, e.g. 'genv explain package jq'")
		fs.Usage()
		return exitUsage
	}
	if extra := fs.NArg(); extra > 0 {
		fprintf(os.Stderr, "genv explain: unexpected argument %q\n", fs.Arg(0))
		return exitUsage
	}

	f, err := genvfile.Read(*file)
	if err != nil {
		return reportSpecReadError("explain", *file, err)
	}
	c, code := materializeComposition("explain", *file, f, "", *targetFlag, composeSourceRoot(*file, ""))
	if code != exitOK {
		return code
	}
	if !compose.IsKnownKind(kind) {
		fprintf(os.Stderr, "genv explain: unknown kind %q; expected one of %s\n", kind, strings.Join(compose.KnownKinds(), ", "))
		return exitUsage
	}
	return printResourceOwners(c, kind, name, *jsonOut)
}

// reportSpecReadError prints a spec read failure with the same wording the
// other commands use, so `config` and `explain` behave like the rest of the CLI.
func reportSpecReadError(commandName, file string, err error) int {
	if errors.Is(err, genvfile.ErrNotFound) {
		fprintf(os.Stderr, "genv %s: %s not found\n", commandName, file)
		return exitIO
	}
	fprintf(os.Stderr, "genv %s: %v\n", commandName, err)
	if strings.Contains(err.Error(), "invalid") {
		return exitValidation
	}
	return exitIO
}
