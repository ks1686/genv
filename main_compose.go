package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/ks1686/genv/internal/compose"
	"github.com/ks1686/genv/internal/genvfile"
	"github.com/ks1686/genv/internal/schema"
	"github.com/ks1686/genv/internal/target"
)

// compositionFor resolves the effective environment for a command.
//
// This is the single entry point every desired-state consumer must use. It
// returns the flattened spec plus the provenance of each resource, so commands
// that reconcile (apply, status, upgrade, updates, scan) and commands that only
// read (validate, config, explain) all see exactly the same environment.
//
// v1-v7 specs keep their legacy host filtering; v8+ go through compose.Resolve,
// which performs the normal defaults/target merge and additionally composes
// selected v10 modules.
func compositionFor(file string, f *schema.GenvFile, hostName, targetFlag, sourceRoot string) (*compose.Composition, error) {
	if f == nil {
		return nil, fmt.Errorf("genv file is nil")
	}
	// Custom adapters are root-owned; bind them before anything resolves a
	// manager so prefer: <adapter-name> behaves the same composed or not.
	useSpecAdapters(f)

	if !schema.IsPortableVersion(f.SchemaVersion) {
		effective := hostFilter(f, hostName)
		return &compose.Composition{
			Effective:  effective,
			Provenance: compose.NewProvenance(),
		}, nil
	}

	targetID, err := target.Resolve(targetFlag)
	if err != nil {
		return nil, err
	}
	if f.Targets[targetID] == nil {
		return nil, fmt.Errorf("no matching targets.%s", targetID)
	}
	c, err := compose.Resolve(file, sourceRoot, f, targetID, nil)
	if err != nil {
		return nil, err
	}
	c.Effective = schema.DropInapplicable(c.Effective, runtime.GOOS)
	return c, nil
}

// materializeComposition loads nothing: it composes an already-read spec.
func materializeComposition(commandName, file string, f *schema.GenvFile, hostFlag, targetFlag, sourceRoot string) (*compose.Composition, int) {
	c, err := compositionFor(file, f, hostForCommand(hostFlag), targetFlag, sourceRoot)
	if err == nil {
		return c, exitOK
	}
	return nil, classifyComposeError(commandName, file, err)
}

// readComposition reads the spec from disk and composes it.
func readComposition(commandName, file, hostFlag, targetFlag, sourceRoot string) (*compose.Composition, int) {
	f, err := genvfile.Read(file)
	if err != nil {
		if errors.Is(err, genvfile.ErrNotFound) {
			fprintf(os.Stderr, "genv %s: %s not found\n", commandName, file)
			return nil, exitIO
		}
		fprintf(os.Stderr, "genv %s: %v\n", commandName, err)
		if errors.Is(err, genvfile.ErrInvalidFile) {
			return nil, exitValidation
		}
		return nil, exitIO
	}
	return materializeComposition(commandName, file, f, hostFlag, targetFlag, sourceRoot)
}

// classifyComposeError maps a composition failure onto genv's exit codes and
// prints the message once, at the boundary. Composition failures are authoring
// errors: a missing module document, a cycle, or two contributors that disagree
// is an invalid configuration, not an I/O accident.
func classifyComposeError(commandName, file string, err error) int {
	msg := err.Error()
	switch {
	case strings.Contains(msg, "resolve target") || strings.Contains(msg, "pass --target"):
		fprintf(os.Stderr, "genv %s: %v\n", commandName, err)
		return exitUsage
	case strings.HasPrefix(msg, "no matching targets."):
		fprintf(os.Stderr, "genv %s: %s in %s\n", commandName, msg, file)
		return exitValidation
	default:
		fprintf(os.Stderr, "genv %s: %v\n", commandName, err)
		return exitValidation
	}
}

// moduleOwnerGuard refuses a mutation aimed at a resource a module owns.
//
// A module's declarations are read-only from the CLI: `genv add ripgrep` when
// a module already declares it must fail and point at the module, not silently
// add a second declaration to the root that the module's version would win over
// (or lose to) on the next compose. The guard runs before any package manager
// subprocess and before any spec write, so a refusal costs nothing.
//
// Returns exitOK when the mutation is allowed, exitLogic when it is refused,
// and exits nothing for specs without modules (v1-v9, or v10 with no selection).
func moduleOwnerGuard(commandName string, c *compose.Composition, kind string, key string) int {
	if c == nil || c.Provenance == nil || key == "" {
		return exitOK
	}
	owners := c.Provenance.Owners(compose.Identity{Kind: kind, Key: key})
	seen := map[string]bool{}
	var modules []string
	for _, o := range owners {
		if o.Module == "" || seen[o.Module] {
			continue
		}
		seen[o.Module] = true
		modules = append(modules, o.Module)
	}
	if len(modules) == 0 {
		return exitOK
	}
	subject := string(kind) + " " + strconv.Quote(key)
	if len(modules) == 1 {
		fprintf(os.Stderr, "genv %s: %s is declared by module %q; edit modules/%s instead of running this command\n",
			commandName, subject, modules[0], modules[0])
	} else {
		fprintf(os.Stderr, "genv %s: %s is declared by modules %s; edit those modules instead of running this command\n",
			commandName, subject, strings.Join(quoteAll(modules), ", "))
	}
	return exitLogic
}

func quoteAll(names []string) []string {
	out := make([]string, len(names))
	for i, n := range names {
		out[i] = strconv.Quote(n)
	}
	return out
}

// moduleOwnerGuardFor is the file-based convenience wrapper: it composes the
// spec on demand and guards the mutation. Callers that already hold a
// composition should prefer moduleOwnerGuard to avoid loading modules twice.
func moduleOwnerGuardFor(commandName, file string, f *schema.GenvFile, hostFlag, targetFlag, sourceRoot string, kind string, key string) int {
	if f == nil || !schema.IsPortableVersion(f.SchemaVersion) || len(f.Modules) == 0 {
		return exitOK
	}
	c, code := materializeComposition(commandName, file, f, hostFlag, targetFlag, sourceRoot)
	if code != exitOK {
		return code
	}
	return moduleOwnerGuard(commandName, c, kind, key)
}

// A command's --source-root relocates the whole config tree, so modules move
// with it; otherwise they sit next to the spec file.
func composeSourceRoot(file, sourceRoot string) string {
	if sourceRoot != "" {
		return sourceRoot
	}
	if file == "" {
		return ""
	}
	return filepath.Dir(file)
}

// validateComposition is the `genv validate` check: every registered module
// document must be readable and valid, and the active target must compose.
//
// Reconciliation deliberately loads only the selection closure, so this is the
// only command that reports a broken module nobody selected.
func validateComposition(file string, f *schema.GenvFile, hostFlag, targetFlag string) []error {
	var errs []error
	if f == nil || !schema.IsPortableVersion(f.SchemaVersion) {
		return nil
	}
	if len(f.Modules) == 0 {
		return nil
	}
	root := composeSourceRoot(file, "")
	if _, err := compose.LoadAllRegistered(root, f.Modules); err != nil {
		errs = append(errs, fmt.Errorf("module registry: %w", err))
		return errs
	}
	// Compose when a target is actually resolvable. An unresolvable target is
	// reported by the caller's normal target resolution, not here.
	if _, err := compositionFor(file, f, hostForCommand(hostFlag), targetFlag, ""); err != nil {
		if strings.HasPrefix(err.Error(), "no matching targets.") || strings.Contains(err.Error(), "resolve target") {
			return errs
		}
		errs = append(errs, fmt.Errorf("module composition: %w", err))
	}
	return errs
}

// stampCompositionLock records which modules produced the applied state. It is
// machine-local metadata: a later run uses it to explain drift, never to refuse
// the lock, because editing a selection is an ordinary spec change.
func stampCompositionLock(lf *genvfile.LockFile, c *compose.Composition) {
	if lf == nil || c == nil {
		return
	}
	if len(c.SelectedModules) == 0 && c.Fingerprint == "" {
		return
	}
	lf.Modules = append([]string(nil), c.SelectedModules...)
	lf.Fingerprint = c.Fingerprint
}

// compositionDriftNotice describes how the lock's recorded composition differs
// from what the spec would produce now. It is informational: the packages in
// the effective spec are what get reconciled.
func compositionDriftNotice(lf *genvfile.LockFile, c *compose.Composition) string {
	if lf == nil || c == nil || (len(lf.Modules) == 0 && lf.Fingerprint == "") {
		return ""
	}
	var parts []string
	if strings.Join(lf.Modules, ",") != strings.Join(c.SelectedModules, ",") {
		parts = append(parts, fmt.Sprintf("module selection changed (%s -> %s)",
			strings.Join(lf.Modules, ", "), strings.Join(c.SelectedModules, ", ")))
	}
	if lf.Fingerprint != "" && c.Fingerprint != "" && lf.Fingerprint != c.Fingerprint {
		parts = append(parts, "composed environment changed since the last apply")
	}
	return strings.Join(parts, "; ")
}

// flattenedExportSpec turns a composed environment into a single-target portable
// spec for export.
//
// The snapshot is deliberately flat: it is a portable copy of one target's
// environment, and a module document is only meaningful next to its registry
// and the rest of its selection. Carrying modules would require re-rewriting
// every path in every document and re-validating the result on the far side;
// materializing is both simpler and strictly more portable. The report records
// which modules were materialized so the provenance is not lost silently.
func flattenedExportSpec(effective *schema.GenvFile, targetID string) *schema.GenvFile {
	return &schema.GenvFile{
		SchemaVersion: schema.Version8,
		Repo:          effective.Repo,
		Updates:       effective.Updates,
		Adapters:      effective.Adapters,
		Targets:       map[string]*schema.TargetBundle{targetID: compose.BundleFromEffective(effective)},
	}
}
