// Package migrate converts legacy genv.json shapes to newer schema versions.
package migrate

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	"github.com/ks1686/genv/internal/schema"
)

// ToV8 converts a v1-v7 genv spec with top-level records and host predicates
// into the v8 defaults/targets shape. The input is not mutated.
func ToV8(in *schema.GenvFile) (*schema.GenvFile, []string, error) {
	if in == nil {
		return nil, nil, errors.New("cannot migrate nil spec")
	}
	if in.SchemaVersion == schema.Version8 {
		out, err := cloneGenvFile(in)
		return out, nil, err
	}
	if !isLegacyVersion(in.SchemaVersion) {
		return nil, nil, fmt.Errorf("cannot migrate unsupported schemaVersion %q", in.SchemaVersion)
	}

	var warnings []string
	observedTargets, observedWarnings, err := inferObservedTargets(in)
	if err != nil {
		return nil, nil, err
	}
	warnings = append(warnings, observedWarnings...)

	fallbackLinux := len(observedTargets) == 0
	if fallbackLinux {
		observedTargets = map[string]bool{"linux": true}
		warnings = append(warnings, "could not infer a concrete target from host predicates; placed empty-host entries in targets.linux")
	}

	out := &schema.GenvFile{
		SchemaVersion: schema.Version8,
		Repo:          copyRepo(in.Repo),
		Updates:       copyUpdatesConfig(in.Updates),
		Targets:       make(map[string]*schema.TargetBundle),
	}
	if defaults := migrateDefaults(in); !targetBundleEmpty(defaults) {
		out.Defaults = defaults
	}

	targetIDs := sortedTargetIDs(observedTargets)
	for _, target := range targetIDs {
		out.Targets[target] = &schema.TargetBundle{}
	}

	if err := bucketPackages(out.Targets, targetIDs, in.Packages); err != nil {
		return nil, nil, err
	}
	if err := bucketServices(out.Targets, targetIDs, in.Services); err != nil {
		return nil, nil, err
	}
	if err := bucketFiles(out.Targets, targetIDs, in.Files); err != nil {
		return nil, nil, err
	}
	if err := bucketHooks(out.Targets, targetIDs, in.Hooks); err != nil {
		return nil, nil, err
	}

	return out, warnings, nil
}

func isLegacyVersion(version string) bool {
	switch version {
	case schema.Version, schema.Version2, schema.Version3, schema.Version4, schema.Version5, schema.Version6, schema.Version7:
		return true
	default:
		return false
	}
}

func inferObservedTargets(f *schema.GenvFile) (map[string]bool, []string, error) {
	targets := make(map[string]bool)
	var warnings []string
	add := func(hosts schema.HostPredicate) error {
		if len(hosts) == 0 {
			return nil
		}
		ids, hostWarnings, err := targetsForHostPredicate(hosts)
		if err != nil {
			return err
		}
		warnings = append(warnings, hostWarnings...)
		for _, id := range ids {
			targets[id] = true
		}
		return nil
	}

	for _, pkg := range f.Packages {
		if err := add(pkg.Host); err != nil {
			return nil, nil, fmt.Errorf("packages.%s.host: %w", pkg.ID, err)
		}
	}
	for name, svc := range f.Services {
		if err := add(svc.Host); err != nil {
			return nil, nil, fmt.Errorf("services.%s.host: %w", name, err)
		}
	}
	if f.Files != nil {
		for i, link := range f.Files.Links {
			if err := add(link.Host); err != nil {
				return nil, nil, fmt.Errorf("files.links[%d].host: %w", i, err)
			}
		}
		for i, tpl := range f.Files.Templates {
			if err := add(tpl.Host); err != nil {
				return nil, nil, fmt.Errorf("files.templates[%d].host: %w", i, err)
			}
		}
		for i, dir := range f.Files.Dirs {
			if err := add(dir.Host); err != nil {
				return nil, nil, fmt.Errorf("files.dirs[%d].host: %w", i, err)
			}
		}
	}
	if f.Hooks != nil {
		for _, phase := range hookPhases(f.Hooks) {
			for i, hook := range phase.hooks {
				if err := add(hook.Host); err != nil {
					return nil, nil, fmt.Errorf("hooks.%s[%d].host: %w", phase.name, i, err)
				}
			}
		}
	}

	return targets, uniqueStrings(warnings), nil
}

func targetsForHostPredicate(hosts schema.HostPredicate) ([]string, []string, error) {
	targets := make(map[string]bool)
	var hasBareWSL bool
	for _, host := range hosts {
		switch host {
		case "macos", "windows", "arch", "ubuntu", "linux", "wsl-arch":
			targets[host] = true
		case "wsl2":
			targets["wsl-arch"] = true
			hasBareWSL = true
		default:
			return nil, nil, fmt.Errorf("cannot map legacy host %q to a schema v8 target", host)
		}
	}
	var warnings []string
	if hasBareWSL && len(hosts) == 1 {
		warnings = append(warnings, `host "wsl2" was migrated to targets.wsl-arch; Ubuntu WSL users must rebucket those entries to targets.ubuntu`)
	}
	return sortedTargetIDs(targets), warnings, nil
}

func migrateDefaults(in *schema.GenvFile) *schema.TargetBundle {
	defaults := &schema.TargetBundle{}
	if len(in.Env) > 0 {
		defaults.Env = make(map[string]*schema.EnvVar, len(in.Env))
		for name, envVar := range in.Env {
			v := envVar
			defaults.Env[name] = &v
		}
	}
	if in.Shell != nil {
		defaults.Shell = copyShellToTarget(in.Shell)
	}
	return defaults
}

func bucketPackages(targets map[string]*schema.TargetBundle, defaultTargetIDs []string, packages []schema.Package) error {
	for _, pkg := range packages {
		ids, err := targetIDsForRecord(pkg.Host, defaultTargetIDs)
		if err != nil {
			return fmt.Errorf("packages.%s.host: %w", pkg.ID, err)
		}
		pkg.Host = nil
		pkg.Managers = copyStringMap(pkg.Managers)
		for _, id := range ids {
			targets[id].Packages = append(targets[id].Packages, pkg)
		}
	}
	return nil
}

func bucketServices(targets map[string]*schema.TargetBundle, defaultTargetIDs []string, services map[string]schema.Service) error {
	for name, svc := range services {
		ids, err := targetIDsForRecord(svc.Host, defaultTargetIDs)
		if err != nil {
			return fmt.Errorf("services.%s.host: %w", name, err)
		}
		svc.Host = nil
		svc.Start = copyStrings(svc.Start)
		svc.Stop = copyStrings(svc.Stop)
		svc.Restart = copyStrings(svc.Restart)
		svc.Status = copyStrings(svc.Status)
		for _, id := range ids {
			if targets[id].Services == nil {
				targets[id].Services = make(map[string]*schema.Service)
			}
			v := svc
			targets[id].Services[name] = &v
		}
	}
	return nil
}

func bucketFiles(targets map[string]*schema.TargetBundle, defaultTargetIDs []string, files *schema.FilesConfig) error {
	if files == nil {
		return nil
	}
	for i, link := range files.Links {
		ids, err := targetIDsForRecord(link.Host, defaultTargetIDs)
		if err != nil {
			return fmt.Errorf("files.links[%d].host: %w", i, err)
		}
		link.Host = nil
		for _, id := range ids {
			files := ensureFiles(targets[id])
			files.Links = append(files.Links, link)
		}
	}
	for i, tpl := range files.Templates {
		ids, err := targetIDsForRecord(tpl.Host, defaultTargetIDs)
		if err != nil {
			return fmt.Errorf("files.templates[%d].host: %w", i, err)
		}
		tpl.Host = nil
		for _, id := range ids {
			files := ensureFiles(targets[id])
			files.Templates = append(files.Templates, tpl)
		}
	}
	for i, dir := range files.Dirs {
		ids, err := targetIDsForRecord(dir.Host, defaultTargetIDs)
		if err != nil {
			return fmt.Errorf("files.dirs[%d].host: %w", i, err)
		}
		dir.Host = nil
		for _, id := range ids {
			files := ensureFiles(targets[id])
			files.Dirs = append(files.Dirs, dir)
		}
	}
	return nil
}

func bucketHooks(targets map[string]*schema.TargetBundle, defaultTargetIDs []string, hooks *schema.HooksConfig) error {
	if hooks == nil {
		return nil
	}
	for _, phase := range hookPhases(hooks) {
		for i, hook := range phase.hooks {
			ids, err := targetIDsForRecord(hook.Host, defaultTargetIDs)
			if err != nil {
				return fmt.Errorf("hooks.%s[%d].host: %w", phase.name, i, err)
			}
			hook.Host = nil
			for _, id := range ids {
				appendHook(ensureHooks(targets[id]), phase.name, hook)
			}
		}
	}
	return nil
}

func targetIDsForRecord(hosts schema.HostPredicate, defaultTargetIDs []string) ([]string, error) {
	if len(hosts) == 0 {
		return defaultTargetIDs, nil
	}
	ids, _, err := targetsForHostPredicate(hosts)
	return ids, err
}

func ensureFiles(bundle *schema.TargetBundle) *schema.FilesConfig {
	if bundle.Files == nil {
		bundle.Files = &schema.FilesConfig{}
	}
	return bundle.Files
}

func ensureHooks(bundle *schema.TargetBundle) *schema.HooksConfig {
	if bundle.Hooks == nil {
		bundle.Hooks = &schema.HooksConfig{}
	}
	return bundle.Hooks
}

func appendHook(hooks *schema.HooksConfig, phase string, hook schema.Hook) {
	switch phase {
	case "preApply":
		hooks.PreApply = append(hooks.PreApply, hook)
	case "postApply":
		hooks.PostApply = append(hooks.PostApply, hook)
	case "preAdd":
		hooks.PreAdd = append(hooks.PreAdd, hook)
	case "postAdd":
		hooks.PostAdd = append(hooks.PostAdd, hook)
	case "preRemove":
		hooks.PreRemove = append(hooks.PreRemove, hook)
	case "postRemove":
		hooks.PostRemove = append(hooks.PostRemove, hook)
	case "preUpgrade":
		hooks.PreUpgrade = append(hooks.PreUpgrade, hook)
	case "postUpgrade":
		hooks.PostUpgrade = append(hooks.PostUpgrade, hook)
	}
}

type hookPhase struct {
	name  string
	hooks []schema.Hook
}

func hookPhases(hooks *schema.HooksConfig) []hookPhase {
	return []hookPhase{
		{name: "preApply", hooks: hooks.PreApply},
		{name: "postApply", hooks: hooks.PostApply},
		{name: "preAdd", hooks: hooks.PreAdd},
		{name: "postAdd", hooks: hooks.PostAdd},
		{name: "preRemove", hooks: hooks.PreRemove},
		{name: "postRemove", hooks: hooks.PostRemove},
		{name: "preUpgrade", hooks: hooks.PreUpgrade},
		{name: "postUpgrade", hooks: hooks.PostUpgrade},
	}
}

func targetBundleEmpty(bundle *schema.TargetBundle) bool {
	if bundle == nil {
		return true
	}
	return len(bundle.Packages) == 0 &&
		len(bundle.Env) == 0 &&
		bundle.Shell == nil &&
		len(bundle.Services) == 0 &&
		bundle.Files == nil &&
		bundle.Hooks == nil
}

func sortedTargetIDs(targets map[string]bool) []string {
	ids := make([]string, 0, len(targets))
	for id := range targets {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func uniqueStrings(in []string) []string {
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}

func cloneGenvFile(in *schema.GenvFile) (*schema.GenvFile, error) {
	data, err := json.Marshal(in)
	if err != nil {
		return nil, fmt.Errorf("copying spec: %w", err)
	}
	var out schema.GenvFile
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("copying spec: %w", err)
	}
	return &out, nil
}

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
	out.OnlyManagers = copyStrings(in.OnlyManagers)
	out.SkipManagers = copyStrings(in.SkipManagers)
	out.Only = copyStrings(in.Only)
	out.Skip = copyStrings(in.Skip)
	return &out
}

func copyShellToTarget(in *schema.ShellConfig) *schema.TargetShellConfig {
	if in == nil {
		return nil
	}
	out := &schema.TargetShellConfig{
		Source: copyStrings(in.Source),
	}
	if len(in.Aliases) > 0 {
		out.Aliases = make(map[string]*schema.ShellAlias, len(in.Aliases))
		for name, alias := range in.Aliases {
			v := alias
			out.Aliases[name] = &v
		}
	}
	if len(in.Functions) > 0 {
		out.Functions = make(map[string]*schema.ShellFunction, len(in.Functions))
		for name, fn := range in.Functions {
			v := fn
			out.Functions[name] = &v
		}
	}
	return out
}

func copyStrings(in []string) []string {
	if in == nil {
		return nil
	}
	out := make([]string, len(in))
	copy(out, in)
	return out
}

func copyStringMap(in map[string]string) map[string]string {
	if in == nil {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
