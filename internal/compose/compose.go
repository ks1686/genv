package compose

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ks1686/genv/internal/schema"
)

// RootDocument is the document name used for origins that come from the root
// spec rather than a module file.
const RootDocument = "genv.json"

// Composition is the result of composing a root spec with its selected modules.
type Composition struct {
	// Effective is the flattened desired state: exactly what a command should
	// reconcile for the active target. It is a fresh value; the raw spec is
	// never mutated and the flattened result is never written back over it.
	Effective *schema.GenvFile
	// Provenance records every owner of every composed resource.
	Provenance *Provenance
	// SelectedModules is the resolved selection in contribution order,
	// dependencies before dependents.
	SelectedModules []string
	// Target is the OS target this composition was built for.
	Target string
	// Fingerprint is a non-secret structural hash of the effective environment
	// plus the selection. Env values are excluded so the fingerprint is safe to
	// print, log, and record in the machine-local lock.
	Fingerprint string
}

// assetViolation records a module-relative asset path that escapes the spec root.
type assetViolation struct {
	origin Origin
	value  string
}

// contributor is one source of declarations: the root spec, or one module
// document. Each contributor is materialized on its own first (its defaults
// plus, when the target is active, its target overlay), so a module's own
// tombstone semantics keep working before anything is unioned.
type contributor struct {
	module   string
	document string
	dir      string // directory that relative asset paths resolve against
	target   string // target bucket name, for origins
	bundle   *schema.TargetBundle
	// defaults and overlay are the two source bundles this contributor merged
	// into bundle. They are kept only to attribute each declaration to the
	// block it actually lives in: a module may declare a package under
	// "defaults" while the composed value is the target overlay's, and `genv
	// explain` must not claim the package is in the overlay.
	defaults *schema.TargetBundle
	overlay  *schema.TargetBundle
}

// Resolve composes the effective environment for targetID.
//
// rootSpecPath locates the root spec (used for module discovery), sourceRoot
// relocates the whole config tree when it differs, extraSelections append to
// the spec's own module selection.
//
// The returned Composition is independent of f: the raw spec is never mutated
// and the flattened result is never written back over it.
func Resolve(rootSpecPath, sourceRoot string, f *schema.GenvFile, targetID string, extraSelections []string) (*Composition, error) {
	if f == nil {
		return nil, fmt.Errorf("nil spec")
	}
	// v8 and v9 keep their existing behavior: flatten the active target only.
	// The fingerprint is still computed so lock metadata and `genv config`
	// behave identically on a spec that does not use modules yet.
	if !hasModules(f) {
		effective, err := schema.MergeTarget(f, targetID)
		if err != nil {
			return nil, err
		}
		return &Composition{
			Effective:   effective,
			Provenance:  moduleFreeProvenance(f, effective, targetID),
			Target:      targetID,
			Fingerprint: Fingerprint(effective, nil),
		}, nil
	}

	rootDir, err := moduleRoot(rootSpecPath, sourceRoot)
	if err != nil {
		return nil, err
	}

	selections := bundleSelections(f.Defaults, nil)
	selections = bundleSelections(f.Targets[targetID], selections)
	selections = append(selections, extraSelections...)

	// Load only the selection closure: a repo may register modules this target
	// never uses, and an interactive command should not pay to read them.
	// `genv validate` is the command that checks every registration.
	docs, err := LoadSelected(rootDir, f.Modules, selections)
	if err != nil {
		return nil, err
	}
	selected, err := Select(f.Modules, docs, selections)
	if err != nil {
		return nil, err
	}

	rootBundle, err := mergeTargetForContributor(f, targetID)
	if err != nil {
		return nil, err
	}
	contributors := []contributor{{
		module:   "",
		document: RootDocument,
		dir:      "",
		target:   targetID,
		bundle:   rootBundle,
		defaults: f.Defaults,
		overlay:  f.Targets[targetID],
	}}
	for _, name := range selected {
		mod := docs[name]
		contributors = append(contributors, contributor{
			module:   name,
			document: mod.RelPath,
			dir:      path.Dir(mod.RelPath),
			target:   targetID,
			bundle:   moduleBundle(mod.Doc, targetID),
			defaults: mod.Doc.Defaults,
			overlay:  mod.Doc.Targets[targetID],
		})
	}

	acc := newAccumulator()
	for _, c := range contributors {
		if err := acc.add(c); err != nil {
			return nil, err
		}
	}
	if err := acc.unsafeAssetError(); err != nil {
		return nil, err
	}

	bundle := acc.bundle()
	selectedCopy := append([]string{}, selected...)
	effective := &schema.GenvFile{
		SchemaVersion: f.SchemaVersion,
		// Root-level blocks stay root-owned and are carried through untouched,
		// exactly as schema.MergeTarget does. Dropping them here is the trap that
		// made v8 status/upgrade silently see no packages.
		Repo:     copyRepo(f.Repo),
		Updates:  copyUpdatesConfig(f.Updates),
		Adapters: copyAdapters(f.Adapters),
		Packages: bundle.Packages,
		Env:      envToFlat(bundle.Env),
		Shell:    targetShellToFlat(bundle.Shell),
		Services: servicesToFlat(bundle.Services),
		Files:    bundle.Files,
		Hooks:    bundle.Hooks,
	}
	return &Composition{
		Effective:       effective,
		Provenance:      acc.provenance,
		SelectedModules: selectedCopy,
		Target:          targetID,
		Fingerprint:     Fingerprint(effective, selectedCopy),
	}, nil
}

// unsafeAssetError reports the first module asset path that left the spec root,
// naming the document that declared it.
func (a *accumulator) unsafeAssetError() error {
	if len(a.unsafeAsset) == 0 {
		return nil
	}
	v := a.unsafeAsset[0]
	return fmt.Errorf("%w: %s declares asset %q, which resolves outside the spec root", ErrPathEscape, v.origin, v.value)
}

// moduleFreeProvenance indexes ownership for a spec that declares no modules.
//
// Without this, `genv explain` and `genv config` read an empty index on every
// v8/v9 spec and report every resource as unowned, which reads as "genv does not
// know where this came from" rather than "the root document owns it".
//
// It attributes the merged result to the root document and picks the declaring
// block with the same rule the module path uses, so the two paths answer
// `genv explain` identically. It deliberately performs no conflict checking: a
// spec that never composes is accepted exactly as schema.MergeTarget accepts
// it, and adding a new rejection here would change v8/v9 behavior.
func moduleFreeProvenance(f, effective *schema.GenvFile, targetID string) *Provenance {
	c := contributor{
		module:   "",
		document: RootDocument,
		target:   targetID,
		defaults: f.Defaults,
		overlay:  f.Targets[targetID],
	}
	prov := newProvenance()
	add := func(id Identity, suffix string) {
		prov.add(id, c.origin(c.fieldFor(id)+suffix))
	}

	for _, pkg := range effective.Packages {
		if pkg.ID == "" {
			continue
		}
		add(packageIdentity(pkg.ID), ".packages["+pkg.ID+"]")
	}
	for _, name := range sortedKeys(effective.Env) {
		add(envIdentity(name), ".env."+name)
	}
	if effective.Shell != nil {
		for _, name := range sortedKeys(effective.Shell.Aliases) {
			add(aliasIdentity(name), ".shell.aliases."+name)
		}
		for _, name := range sortedKeys(effective.Shell.Functions) {
			add(funcIdentity(name), ".shell.functions."+name)
		}
	}
	for _, name := range sortedKeys(effective.Services) {
		add(serviceIdentity(name), ".services."+name)
	}
	if effective.Files != nil {
		for _, link := range effective.Files.Links {
			add(fileIdentity(link.Target), ".files.links["+link.Target+"]")
		}
		for _, tmpl := range effective.Files.Templates {
			add(fileIdentity(tmpl.Target), ".files.templates["+tmpl.Target+"]")
		}
		for _, dir := range effective.Files.Dirs {
			add(dirIdentity(CleanPath(dir.Target)), ".files.dirs["+dir.Target+"]")
		}
	}
	if effective.Hooks != nil {
		fallback := c.moduleField()
		for _, phase := range hookPhaseNames {
			for i := range phaseSlice(effective.Hooks, phase) {
				prov.add(hookIdentity(c.document, phase, i), c.origin(fallback+".hooks."+phase))
			}
		}
	}
	return prov
}

// hasModules reports whether a spec uses composition at all. A v10 spec without
// modules or selections takes the same fast path as v8/v9.
func hasModules(f *schema.GenvFile) bool {
	if len(f.Modules) > 0 {
		return true
	}
	if f.Defaults != nil && len(f.Defaults.UseModules) > 0 {
		return true
	}
	for _, bundle := range f.Targets {
		if bundle != nil && len(bundle.UseModules) > 0 {
			return true
		}
	}
	return false
}

// moduleRoot decides which directory holds module documents and resolves their
// relative asset paths. `--source-root` relocates the complete config tree, so
// when it is set the modules live under that directory rather than next to the
// spec file.
func moduleRoot(rootSpecPath, sourceRoot string) (string, error) {
	if sourceRoot != "" {
		return filepath.Abs(sourceRoot)
	}
	if rootSpecPath == "" {
		return filepath.Abs(".")
	}
	return filepath.Abs(filepath.Dir(rootSpecPath))
}

// mergeTargetForContributor applies the existing overlay rules to the root spec,
// keeping the same behavior commands rely on today (arrays replace, map keys
// win, tombstones delete, all scoped to the one contributor).
func mergeTargetForContributor(f *schema.GenvFile, targetID string) (*schema.TargetBundle, error) {
	bundle, ok := f.Targets[targetID]
	if !ok {
		return nil, fmt.Errorf("merge target %q: target not found", targetID)
	}
	if bundle == nil {
		return nil, fmt.Errorf("merge target %q: target is nil", targetID)
	}
	return mergeBundle(f.Defaults, bundle), nil
}

// moduleBundle materializes one module for the active target: its defaults,
// then its target overlay when that target exists. A module without the active
// target contributes only its defaults.
func moduleBundle(doc *schema.ModuleDoc, targetID string) *schema.TargetBundle {
	if doc == nil {
		return &schema.TargetBundle{}
	}
	overlay := doc.Targets[targetID]
	if overlay == nil {
		overlay = &schema.TargetBundle{}
	}
	return mergeBundle(doc.Defaults, overlay)
}

// mergeBundle overlays target on top of defaults using the same rules as
// schema.MergeTarget, minus the "target must exist" requirement that applies to
// the root spec.
func mergeBundle(defaults, overlay *schema.TargetBundle) *schema.TargetBundle {
	out := &schema.TargetBundle{UseModules: bundleSelections(defaults, nil)}
	if defaults == nil {
		defaults = &schema.TargetBundle{}
	}

	if defaults.Packages != nil {
		out.Packages = append([]schema.Package(nil), defaults.Packages...)
	}
	// Array semantics are replace, not append, and an empty array is still a
	// replacement: schema.MergeTarget treats a non-nil target array as
	// authoritative. Using len() > 0 here would make an empty target array mean
	// "keep whatever defaults said", so the same spec would behave differently
	// depending on whether it uses modules.
	if overlay.Packages != nil {
		out.Packages = append([]schema.Package(nil), overlay.Packages...)
	}

	// nil map value = tombstone; a null target entry deletes the inherited key.
	mergeEntries(defaults.Env, overlay.Env, func() map[string]*schema.EnvVar { return map[string]*schema.EnvVar{} }, &out.Env)

	mergeEntries(defaults.Services, overlay.Services, func() map[string]*schema.Service { return map[string]*schema.Service{} }, &out.Services)

	out.Shell = mergeShellBundles(defaults.Shell, overlay.Shell)

	files := &schema.FilesConfig{}
	if defaults.Files != nil {
		files.Links = append(files.Links, defaults.Files.Links...)
		files.Templates = append(files.Templates, defaults.Files.Templates...)
		files.Dirs = append(files.Dirs, defaults.Files.Dirs...)
	}
	if overlay.Files != nil {
		// Same replace-not-append rule as packages, empty array included.
		if overlay.Files.Links != nil {
			files.Links = append([]schema.FileLink(nil), overlay.Files.Links...)
		}
		if overlay.Files.Templates != nil {
			files.Templates = append([]schema.FileTemplate(nil), overlay.Files.Templates...)
		}
		if overlay.Files.Dirs != nil {
			files.Dirs = append([]schema.FileDir(nil), overlay.Files.Dirs...)
		}
	}
	out.Files = files

	out.Hooks = mergeHookBundles(defaults.Hooks, overlay.Hooks)
	return out
}

func mergeEntries[K any](defaults, overlay map[string]*K, empty func() map[string]*K, dst *map[string]*K) {
	out := empty()
	for k, v := range defaults {
		if v == nil {
			continue // defaults may not carry tombstones
		}
		out[k] = v
	}
	for k, v := range overlay {
		if v == nil {
			delete(out, k)
			continue
		}
		out[k] = v
	}
	if len(out) > 0 {
		*dst = out
	}
}

// mergeShellBundles overlays aliases and functions with the same map-wins and
// tombstone rules the root merge uses.
func mergeShellBundles(defaults, overlay *schema.TargetShellConfig) *schema.TargetShellConfig {
	out := &schema.TargetShellConfig{}
	if defaults != nil {
		out.Aliases = copyAliasMap(defaults.Aliases)
		out.Functions = copyFunctionMap(defaults.Functions)
		out.Source = append([]string(nil), defaults.Source...)
	}
	if overlay != nil {
		for k, v := range overlay.Aliases {
			if v == nil {
				delete(out.Aliases, k)
				continue
			}
			if out.Aliases == nil {
				out.Aliases = map[string]*schema.ShellAlias{}
			}
			out.Aliases[k] = v
		}
		for k, v := range overlay.Functions {
			if v == nil {
				delete(out.Functions, k)
				continue
			}
			if out.Functions == nil {
				out.Functions = map[string]*schema.ShellFunction{}
			}
			out.Functions[k] = v
		}
		// source is an array, so an overlay replaces it wholesale — including
		// replacing it with an empty array.
		if overlay.Source != nil {
			out.Source = append([]string(nil), overlay.Source...)
		}
	}
	if len(out.Aliases) == 0 && len(out.Functions) == 0 && len(out.Source) == 0 {
		return nil
	}
	return out
}

func copyAliasMap(in map[string]*schema.ShellAlias) map[string]*schema.ShellAlias {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]*schema.ShellAlias, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func copyFunctionMap(in map[string]*schema.ShellFunction) map[string]*schema.ShellFunction {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]*schema.ShellFunction, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

// mergeHookBundles keeps hook order: a contributor's defaults hooks first, then
// its target overlay's. Order matters because hooks run in sequence.
func mergeHookBundles(defaults, overlay *schema.HooksConfig) *schema.HooksConfig {
	if defaults == nil && overlay == nil {
		return nil
	}
	out := &schema.HooksConfig{}
	for _, phase := range hookPhaseNames {
		// Within one document a phase is replaced by the overlay, empty phase
		// included, matching schema.mergeHooks. Accumating *across* contributors
		// is the accumulator's job, not this merge's.
		if overlay != nil && phaseSlice(overlay, phase) != nil {
			setPhase(out, phase, append([]schema.Hook(nil), phaseSlice(overlay, phase)...))
			continue
		}
		setPhase(out, phase, append([]schema.Hook(nil), phaseSlice(defaults, phase)...))
	}
	if hooksEmpty(out) {
		return nil
	}
	return out
}

// setPhase replaces one phase's hooks. schema.HooksConfig is defined in another
// package, so the phase accessors live here rather than as methods on it.
func setPhase(h *schema.HooksConfig, phase string, hooks []schema.Hook) {
	switch phase {
	case "preApply":
		h.PreApply = hooks
	case "postApply":
		h.PostApply = hooks
	case "preAdd":
		h.PreAdd = hooks
	case "postAdd":
		h.PostAdd = hooks
	case "preRemove":
		h.PreRemove = hooks
	case "postRemove":
		h.PostRemove = hooks
	case "preUpgrade":
		h.PreUpgrade = hooks
	case "postUpgrade":
		h.PostUpgrade = hooks
	}
}

// appendPhase appends to one phase's hooks.
func appendPhase(h *schema.HooksConfig, phase string, hooks []schema.Hook) {
	setPhase(h, phase, append(phaseSlice(h, phase), hooks...))
}

func hooksEmpty(h *schema.HooksConfig) bool {
	if h == nil {
		return true
	}
	for _, phase := range hookPhaseNames {
		if len(phaseSlice(h, phase)) > 0 {
			return false
		}
	}
	return true
}

// hookPhaseNames lists hook phases in execution order.
var hookPhaseNames = []string{
	"preApply", "postApply", "preAdd", "postAdd",
	"preRemove", "postRemove", "preUpgrade", "postUpgrade",
}

func phaseSlice(h *schema.HooksConfig, phase string) []schema.Hook {
	if h == nil {
		return nil
	}
	switch phase {
	case "preApply":
		return h.PreApply
	case "postApply":
		return h.PostApply
	case "preAdd":
		return h.PreAdd
	case "postAdd":
		return h.PostAdd
	case "preRemove":
		return h.PreRemove
	case "postRemove":
		return h.PostRemove
	case "preUpgrade":
		return h.PreUpgrade
	case "postUpgrade":
		return h.PostUpgrade
	}
	return nil
}

// accumulator unions contributors by resource identity.
type accumulator struct {
	packages   map[string]schema.Package
	pkgOrder   []string
	services   map[string]schema.Service
	svcOrder   []string
	env        map[string]schema.EnvVar
	envOrder   []string
	aliases    map[string]schema.ShellAlias
	aliasOrder []string
	funcs      map[string]schema.ShellFunction
	funcOrder  []string
	// sources is the accumulated shell.source list. Sourced files accumulate
	// across contributors — a module's rc fragment must not erase the root's —
	// while a single document's own overlay still replaces its defaults.
	sources   []string
	sourceSet map[string]bool
	links     map[string]schema.FileLink
	templates map[string]schema.FileTemplate
	dirs      map[string]schema.FileDir
	fileOrder []string
	dirOrder  []string
	hooks     *schema.HooksConfig
	// unsafeAsset collects module-relative asset paths that escape the spec
	// root, reported after the union so the error names the owning document.
	unsafeAsset []assetViolation
	// destinations maps a normalized destination path to the identity kind that
	// claims it, so a link and a template cannot both own one path.
	destinations map[string]string
	provenance   *Provenance
}

func newAccumulator() *accumulator {
	return &accumulator{
		packages:     map[string]schema.Package{},
		services:     map[string]schema.Service{},
		env:          map[string]schema.EnvVar{},
		aliases:      map[string]schema.ShellAlias{},
		funcs:        map[string]schema.ShellFunction{},
		sourceSet:    map[string]bool{},
		links:        map[string]schema.FileLink{},
		templates:    map[string]schema.FileTemplate{},
		dirs:         map[string]schema.FileDir{},
		destinations: map[string]string{},
		provenance:   newProvenance(),
	}
}

// moduleField names the field prefix used in origins for this contributor.
// It is the fallback for a declaration whose owning block cannot be determined
// (for example a resource that arrived through the merge rather than either
// source block verbatim).
func (c contributor) moduleField() string {
	if c.target != "" {
		return "targets." + c.target
	}
	return "defaults"
}

// fieldFor names the block that actually declares identity key in this
// contributor: the target overlay when it has one, otherwise the defaults.
//
// Without this, every module declaration would be reported under the target
// bucket, which sends a user editing the wrong file.
func (c contributor) fieldFor(identity Identity) string {
	overlayField := "targets." + c.target
	if c.overlay != nil && declares(c.overlay, identity) {
		return overlayField
	}
	if c.defaults != nil && declares(c.defaults, identity) {
		return "defaults"
	}
	return c.moduleField()
}

// declares reports whether a bundle itself contains the identity, ignoring
// what the merge would produce.
func declares(b *schema.TargetBundle, identity Identity) bool {
	if b == nil {
		return false
	}
	switch identity.Kind {
	case KindPackage:
		for _, p := range b.Packages {
			if p.ID == identity.Key {
				return true
			}
		}
	case KindService:
		_, ok := b.Services[identity.Key]
		return ok
	case KindEnv:
		_, ok := b.Env[identity.Key]
		return ok
	case KindAlias:
		return b.Shell != nil && shellDeclares(b.Shell.Aliases, identity.Key)
	case KindFunc:
		return b.Shell != nil && shellDeclares(b.Shell.Functions, identity.Key)
	case KindFile:
		if b.Files == nil {
			return false
		}
		for _, l := range b.Files.Links {
			if CleanPath(l.Target) == CleanPath(identity.Key) {
				return true
			}
		}
		for _, t := range b.Files.Templates {
			if CleanPath(t.Target) == CleanPath(identity.Key) {
				return true
			}
		}
	case KindDir:
		if b.Files == nil {
			return false
		}
		for _, d := range b.Files.Dirs {
			if CleanPath(d.Target) == CleanPath(identity.Key) {
				return true
			}
		}
	}
	return false
}

func shellDeclares[T any](m map[string]T, key string) bool {
	if m == nil {
		return false
	}
	_, ok := m[key]
	return ok
}

// origin builds the Origin for one declaration in this contributor.
func (c contributor) origin(field string) Origin {
	return Origin{Document: c.document, Field: field, Module: c.module, Target: c.target}
}

// add unions one contributor's declarations, recording provenance and failing
// on the first identity that two contributors declare differently.
func (a *accumulator) add(c contributor) error {
	b := c.bundle
	if b == nil {
		return nil
	}
	// fallbackField names the contributor's block when the owning block of a
	// declaration cannot be determined (hooks, which are per document+phase
	// rather than per resource).
	fallbackField := c.moduleField()

	for _, pkg := range b.Packages {
		if pkg.ID == "" {
			continue
		}
		id := packageIdentity(pkg.ID)
		origin := c.origin(c.fieldFor(id) + ".packages[" + pkg.ID + "]")
		if existing, ok := a.packages[pkg.ID]; ok {
			if err := requireEqual(existing, pkg, id, a.provenance.Owners(id), origin); err != nil {
				return err
			}
			a.provenance.add(id, origin)
			continue
		}
		a.packages[pkg.ID] = pkg
		a.pkgOrder = append(a.pkgOrder, pkg.ID)
		a.provenance.add(id, origin)
	}

	names := sortedKeys(b.Services)
	for _, name := range names {
		svc := b.Services[name]
		if svc == nil {
			continue
		}
		if err := a.claimDestination(c, c.fieldFor(serviceIdentity(name))+".services."+name, svc, claimantLaunchd); err != nil {
			return err
		}
		id := serviceIdentity(name)
		origin := c.origin(c.fieldFor(id) + ".services." + name)
		if existing, ok := a.services[name]; ok {
			if err := requireEqual(existing, *svc, id, a.provenance.Owners(id), origin); err != nil {
				return err
			}
			a.provenance.add(id, origin)
			continue
		}
		a.services[name] = *a.rewriteServicePaths(svc, c)
		a.svcOrder = append(a.svcOrder, name)
		a.provenance.add(id, origin)
	}

	for _, name := range sortedKeys(b.Env) {
		entry := b.Env[name]
		if entry == nil {
			continue
		}
		id := envIdentity(name)
		origin := c.origin(c.fieldFor(id) + ".env." + name)
		if existing, ok := a.env[name]; ok {
			if err := requireEqual(existing, *entry, id, a.provenance.Owners(id), origin); err != nil {
				return err
			}
			a.provenance.add(id, origin)
			continue
		}
		a.env[name] = *entry
		a.envOrder = append(a.envOrder, name)
		a.provenance.add(id, origin)
	}

	if b.Shell != nil {
		for _, src := range b.Shell.Source {
			if src == "" || a.sourceSet[src] {
				continue
			}
			a.sourceSet[src] = true
			a.sources = append(a.sources, src)
		}
		for _, name := range sortedKeys(b.Shell.Aliases) {
			entry := b.Shell.Aliases[name]
			if entry == nil {
				continue
			}
			id := aliasIdentity(name)
			origin := c.origin(c.fieldFor(aliasIdentity(name)) + ".shell.aliases." + name)
			if existing, ok := a.aliases[name]; ok {
				if err := requireEqual(existing, *entry, id, a.provenance.Owners(id), origin); err != nil {
					return err
				}
				a.provenance.add(id, origin)
				continue
			}
			a.aliases[name] = *entry
			a.aliasOrder = append(a.aliasOrder, name)
			a.provenance.add(id, origin)
		}
		for _, name := range sortedKeys(b.Shell.Functions) {
			entry := b.Shell.Functions[name]
			if entry == nil {
				continue
			}
			id := funcIdentity(name)
			origin := c.origin(c.fieldFor(funcIdentity(name)) + ".shell.functions." + name)
			if existing, ok := a.funcs[name]; ok {
				if err := requireEqual(existing, *entry, id, a.provenance.Owners(id), origin); err != nil {
					return err
				}
				a.provenance.add(id, origin)
				continue
			}
			a.funcs[name] = *entry
			a.funcOrder = append(a.funcOrder, name)
			a.provenance.add(id, origin)
		}
	}

	if b.Files != nil {
		for _, link := range b.Files.Links {
			if link.Target == "" {
				continue
			}
			if err := a.claimDestination(c, c.fieldFor(fileIdentity(link.Target))+".files.links["+link.Target+"]", &link, claimantLink); err != nil {
				return err
			}
			resolved := a.rewriteLinkPaths(link, c)
			id := fileIdentity(resolved.Target)
			origin := c.origin(c.fieldFor(fileIdentity(resolved.Target)) + ".files.links[" + resolved.Target + "]")
			if existing, ok := a.links[id.Key]; ok {
				if err := requireEqual(existing, resolved, id, a.provenance.Owners(id), origin); err != nil {
					return err
				}
				a.provenance.add(id, origin)
				continue
			}
			a.links[id.Key] = resolved
			a.fileOrder = append(a.fileOrder, id.Key)
			a.provenance.add(id, origin)
		}
		for _, tmpl := range b.Files.Templates {
			if tmpl.Target == "" {
				continue
			}
			if err := a.claimDestination(c, c.fieldFor(fileIdentity(tmpl.Target))+".files.templates["+tmpl.Target+"]", &tmpl, claimantTemplate); err != nil {
				return err
			}
			resolved := a.rewriteTemplatePaths(tmpl, c)
			id := fileIdentity(resolved.Target)
			origin := c.origin(c.fieldFor(fileIdentity(resolved.Target)) + ".files.templates[" + resolved.Target + "]")
			if existing, ok := a.templates[id.Key]; ok {
				if err := requireEqual(existing, resolved, id, a.provenance.Owners(id), origin); err != nil {
					return err
				}
				a.provenance.add(id, origin)
				continue
			}
			a.templates[id.Key] = resolved
			a.fileOrder = append(a.fileOrder, id.Key)
			a.provenance.add(id, origin)
		}
		for _, dir := range b.Files.Dirs {
			if dir.Target == "" {
				continue
			}
			if err := a.claimDestination(c, c.fieldFor(dirIdentity(dir.Target))+".files.dirs["+dir.Target+"]", &dir, claimantDir); err != nil {
				return err
			}
			id := dirIdentity(CleanPath(dir.Target))
			origin := c.origin(c.fieldFor(dirIdentity(dir.Target)) + ".files.dirs[" + dir.Target + "]")
			if existing, ok := a.dirs[id.Key]; ok {
				if err := requireEqual(existing, dir, id, a.provenance.Owners(id), origin); err != nil {
					return err
				}
				a.provenance.add(id, origin)
				continue
			}
			a.dirs[id.Key] = dir
			a.dirOrder = append(a.dirOrder, id.Key)
			a.provenance.add(id, origin)
		}
	}

	if b.Hooks != nil {
		if a.hooks == nil {
			a.hooks = &schema.HooksConfig{}
		}
		for _, phase := range hookPhaseNames {
			// The index is per document and phase, so a hook keeps the same
			// identity no matter how many hooks other documents contribute.
			for i, hook := range phaseSlice(b.Hooks, phase) {
				resolved := a.rewriteHookPaths(hook, c)
				appendPhase(a.hooks, phase, []schema.Hook{resolved})
				a.provenance.add(hookIdentity(c.document, phase, i), c.origin(fallbackField+".hooks."+phase))
			}
		}
	}
	return nil
}

// Destination claimant kinds. These are deliberately distinct from Identity
// kinds: two links may legitimately coalesce on one destination, but a link and
// a template claiming the same path is a conflict genv cannot resolve.
const (
	claimantLink     = "files.links entry"
	claimantTemplate = "files.templates entry"
	claimantDir      = "files.dirs entry"
	claimantLaunchd  = "launchd plist"
)

// claimDestination records which declaration kind owns a filesystem destination
// and fails when a different kind already claims the same path.
func (a *accumulator) claimDestination(c contributor, field string, value any, kind string) error {
	var target string
	switch v := value.(type) {
	case *schema.FileLink:
		target = CleanPath(v.Target)
	case *schema.FileTemplate:
		target = CleanPath(v.Target)
	case *schema.FileDir:
		target = CleanPath(v.Target)
	case *schema.Service:
		if v.Launchd != nil && v.Launchd.Plist != "" {
			target = CleanPath(v.Launchd.Plist)
		}
	}
	if target == "" {
		return nil
	}
	if owner, taken := a.destinations[target]; taken && owner != kind {
		origin := c.origin(field)
		id := Identity{Kind: KindFile, Key: target}
		return &ConflictError{
			Identity: id,
			Origins:  append(a.destinationOrigins(target), origin),
			Reason:   fmt.Sprintf("destination is already claimed as a %s entry", owner),
		}
	}
	a.destinations[target] = kind
	return nil
}

func (a *accumulator) destinationOrigins(target string) []Origin {
	var out []Origin
	for _, id := range a.provenance.Identities() {
		if id.Kind == KindFile && CleanPath(id.Key) == target {
			out = append(out, a.provenance.Owners(id)...)
		}
	}
	return out
}

// requireEqual fails when two contributors declare the same identity differently.
// The comparison is on canonical JSON so any semantic difference is caught,
// while the error message names the field kinds rather than values.
func requireEqual(existing, incoming any, id Identity, owners []Origin, origin Origin) error {
	if declarationsEqual(existing, incoming) {
		return nil
	}
	origins := append([]Origin{}, owners...)
	return &ConflictError{
		Identity: id,
		Origins:  append(origins, origin),
		Reason:   "two contributors declare this resource differently",
	}
}

func declarationsEqual(a, b any) bool {
	left, err := canonicalJSON(a)
	if err != nil {
		return false
	}
	right, err := canonicalJSON(b)
	if err != nil {
		return false
	}
	return bytes.Equal(left, right)
}

func canonicalJSON(v any) ([]byte, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var generic any
	if err := json.Unmarshal(data, &generic); err != nil {
		return nil, err
	}
	return json.Marshal(generic)
}

// BundleFromEffective lifts the top-level declarations of a composed
// environment back into a single target bundle.
//
// Composition returns a flat GenvFile because that is what every consumer
// expects. Export is the one consumer that needs the inverse: it writes a
// single-target spec, and that spec's only bucket must hold the same
// declarations. This lives beside the merger so the two never drift on which
// fields belong to a bundle.
func BundleFromEffective(effective *schema.GenvFile) *schema.TargetBundle {
	if effective == nil {
		return &schema.TargetBundle{}
	}
	env := make(map[string]*schema.EnvVar, len(effective.Env))
	for name, entry := range effective.Env {
		e := entry
		env[name] = &e
	}
	shell := &schema.TargetShellConfig{}
	if effective.Shell != nil {
		shell.Aliases = make(map[string]*schema.ShellAlias, len(effective.Shell.Aliases))
		for name, entry := range effective.Shell.Aliases {
			a := entry
			shell.Aliases[name] = &a
		}
		shell.Functions = make(map[string]*schema.ShellFunction, len(effective.Shell.Functions))
		for name, entry := range effective.Shell.Functions {
			fn := entry
			shell.Functions[name] = &fn
		}
	}
	services := make(map[string]*schema.Service, len(effective.Services))
	for name, entry := range effective.Services {
		svc := entry
		services[name] = &svc
	}
	return &schema.TargetBundle{
		Packages: effective.Packages,
		Env:      env,
		Shell:    shell,
		Services: services,
		Files:    effective.Files,
		Hooks:    effective.Hooks,
	}
}

// bundle renders the accumulated resources as a flat target bundle.
func (a *accumulator) bundle() *schema.TargetBundle {
	out := &schema.TargetBundle{}
	for _, id := range a.pkgOrder {
		out.Packages = append(out.Packages, a.packages[id])
	}
	for _, name := range a.svcOrder {
		svc := a.services[name]
		if out.Services == nil {
			out.Services = make(map[string]*schema.Service, len(a.svcOrder))
		}
		out.Services[name] = &svc
	}
	if len(a.env) > 0 {
		out.Env = make(map[string]*schema.EnvVar, len(a.env))
		for _, name := range a.envOrder {
			entry := a.env[name]
			out.Env[name] = &entry
		}
	}
	if len(a.aliases) > 0 || len(a.funcs) > 0 || len(a.sources) > 0 {
		out.Shell = &schema.TargetShellConfig{}
		if len(a.sources) > 0 {
			out.Shell.Source = append([]string(nil), a.sources...)
		}
		if len(a.aliases) > 0 {
			out.Shell.Aliases = make(map[string]*schema.ShellAlias, len(a.aliases))
			for _, name := range a.aliasOrder {
				entry := a.aliases[name]
				out.Shell.Aliases[name] = &entry
			}
		}
		if len(a.funcs) > 0 {
			out.Shell.Functions = make(map[string]*schema.ShellFunction, len(a.funcs))
			for _, name := range a.funcOrder {
				entry := a.funcs[name]
				out.Shell.Functions[name] = &entry
			}
		}
	}
	if len(a.fileOrder) > 0 || len(a.dirOrder) > 0 {
		out.Files = &schema.FilesConfig{}
		for _, key := range a.fileOrder {
			if link, ok := a.links[key]; ok {
				out.Files.Links = append(out.Files.Links, link)
			}
			if tmpl, ok := a.templates[key]; ok {
				out.Files.Templates = append(out.Files.Templates, tmpl)
			}
		}
		for _, key := range a.dirOrder {
			out.Files.Dirs = append(out.Files.Dirs, a.dirs[key])
		}
	}
	out.Hooks = a.hooks
	return out
}

// targetShellToFlat converts a portable bundle's shell block into the flat
// schema.ShellConfig shape the rest of genv consumes after merging.
func targetShellToFlat(in *schema.TargetShellConfig) *schema.ShellConfig {
	if in == nil {
		return nil
	}
	out := &schema.ShellConfig{}
	if len(in.Aliases) > 0 {
		out.Aliases = make(map[string]schema.ShellAlias, len(in.Aliases))
		for name, alias := range in.Aliases {
			if alias != nil {
				out.Aliases[name] = *alias
			}
		}
	}
	if len(in.Functions) > 0 {
		out.Functions = make(map[string]schema.ShellFunction, len(in.Functions))
		for name, fn := range in.Functions {
			if fn != nil {
				out.Functions[name] = *fn
			}
		}
	}
	if len(in.Source) > 0 {
		out.Source = append([]string(nil), in.Source...)
	}
	if len(out.Aliases) == 0 && len(out.Functions) == 0 && len(out.Source) == 0 {
		return nil
	}
	return out
}

// copyRepo, copyUpdatesConfig, and copyAdapters duplicate the root-owned blocks
// so the effective spec never shares mutable state with the parsed spec.
func copyRepo(in *schema.Repo) *schema.Repo {
	if in == nil {
		return nil
	}
	out := *in
	return &out
}

func copyUpdatesConfig(in *schema.UpdatesConfig) *schema.UpdatesConfig {
	if in == nil {
		return nil
	}
	out := *in
	out.OnlyManagers = append([]string(nil), in.OnlyManagers...)
	out.SkipManagers = append([]string(nil), in.SkipManagers...)
	out.Only = append([]string(nil), in.Only...)
	out.Skip = append([]string(nil), in.Skip...)
	return &out
}

func copyAdapters(in map[string]schema.AdapterDef) map[string]schema.AdapterDef {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]schema.AdapterDef, len(in))
	for name, def := range in {
		out[name] = def
	}
	return out
}

// envToFlat converts a bundle's pointer-valued env map into the flat
// schema.EnvVar map commands read. Tombstones are already resolved during the
// union, so a nil entry here would be a bug; it is skipped defensively.
func envToFlat(in map[string]*schema.EnvVar) map[string]schema.EnvVar {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]schema.EnvVar, len(in))
	for name, v := range in {
		if v != nil {
			out[name] = *v
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// servicesToFlat does the same for services.
func servicesToFlat(in map[string]*schema.Service) map[string]schema.Service {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]schema.Service, len(in))
	for name, v := range in {
		if v != nil {
			out[name] = *v
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// moduleSafePath normalizes a module-relative asset path to a root-relative
// one, refusing anything that leaves the root. Absolute, ~, and $VAR paths are
// returned unchanged: they are already handled as non-portable by apply and
// export, and silently rewriting them here would hide that from the user.
func moduleSafePath(dir, rel string) (string, error) {
	if rel == "" {
		return rel, nil
	}
	if strings.HasPrefix(rel, "/") || strings.HasPrefix(rel, "~") || strings.ContainsRune(rel, '$') {
		return rel, nil
	}
	cleaned := path.Clean(rel)
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", fmt.Errorf("%w: module asset %q escapes the spec root", ErrPathEscape, rel)
	}
	if dir == "" || dir == "." {
		return cleaned, nil
	}
	return path.Join(dir, cleaned), nil
}

func (a *accumulator) rewriteLinkPaths(link schema.FileLink, c contributor) schema.FileLink {
	source, err := moduleSafePath(c.dir, link.Source)
	if err != nil {
		// Record the unsafe value; validateAssets reports it after the union so
		// the error names the owning document.
		a.unsafeAsset = append(a.unsafeAsset, assetViolation{origin: c.origin("files.links"), value: link.Source})
		return link
	}
	link.Source = source
	return link
}

func (a *accumulator) rewriteTemplatePaths(tmpl schema.FileTemplate, c contributor) schema.FileTemplate {
	source, err := moduleSafePath(c.dir, tmpl.Source)
	if err != nil {
		a.unsafeAsset = append(a.unsafeAsset, assetViolation{origin: c.origin("files.templates"), value: tmpl.Source})
		return tmpl
	}
	tmpl.Source = source
	return tmpl
}

func (a *accumulator) rewriteHookPaths(hook schema.Hook, c contributor) schema.Hook {
	if hook.File == "" {
		return hook
	}
	file, err := moduleSafePath(c.dir, hook.File)
	if err != nil {
		a.unsafeAsset = append(a.unsafeAsset, assetViolation{origin: c.origin("hooks"), value: hook.File})
		return hook
	}
	hook.File = file
	return hook
}

func (a *accumulator) rewriteServicePaths(svc *schema.Service, c contributor) *schema.Service {
	out := *svc
	if out.Launchd != nil && out.Launchd.Plist != "" {
		plist, err := moduleSafePath(c.dir, out.Launchd.Plist)
		if err != nil {
			a.unsafeAsset = append(a.unsafeAsset, assetViolation{origin: c.origin("services.launchd"), value: out.Launchd.Plist})
		} else {
			out.Launchd = &schema.LaunchdSpec{Plist: plist}
		}
	}
	if out.Systemd != nil && out.Systemd.Unit != "" {
		unit, err := moduleSafePath(c.dir, out.Systemd.Unit)
		if err != nil {
			a.unsafeAsset = append(a.unsafeAsset, assetViolation{origin: c.origin("services.systemd"), value: out.Systemd.Unit})
		} else {
			out.Systemd = &schema.SystemdSpec{Unit: unit}
		}
	}
	return &out
}
