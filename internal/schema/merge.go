package schema

import "fmt"

// MergeTarget materializes a v8 target by overlaying targets[targetID] on top
// of defaults. The returned document is flat so existing apply paths can consume
// the usual top-level fields without understanding v8 buckets.
func MergeTarget(f *GenvFile, targetID string) (*GenvFile, error) {
	if f == nil {
		return nil, fmt.Errorf("merge target %q: genv file is nil", targetID)
	}
	target, ok := f.Targets[targetID]
	if !ok {
		return nil, fmt.Errorf("merge target %q: target not found", targetID)
	}
	if target == nil {
		return nil, fmt.Errorf("merge target %q: target is nil", targetID)
	}

	out := &GenvFile{
		SchemaVersion: Version8,
		Repo:          copyRepo(f.Repo),
		Updates:       copyUpdatesConfig(f.Updates),
		Adapters:      copyAdapters(f.Adapters),
	}
	defaults := f.Defaults
	out.Packages = mergePackages(defaults, target)
	out.Env = mergeEnv(defaults, target)
	out.Shell = mergeShell(defaults, target)
	out.Services = mergeServices(defaults, target)
	out.Files = mergeFiles(defaults, target)
	out.Hooks = mergeHooks(defaults, target)
	return out, nil
}

func mergePackages(defaults, target *TargetBundle) []Package {
	if target != nil && target.Packages != nil {
		return copyPackages(target.Packages)
	}
	if defaults != nil {
		return copyPackages(defaults.Packages)
	}
	return nil
}

func mergeEnv(defaults, target *TargetBundle) map[string]EnvVar {
	var out map[string]EnvVar
	if defaults != nil {
		for k, v := range defaults.Env {
			if v == nil {
				continue
			}
			if out == nil {
				out = make(map[string]EnvVar, len(defaults.Env))
			}
			out[k] = *v
		}
	}
	if target != nil {
		for k, v := range target.Env {
			if v == nil {
				delete(out, k)
				continue
			}
			if out == nil {
				out = make(map[string]EnvVar, len(target.Env))
			}
			out[k] = *v
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func mergeShell(defaults, target *TargetBundle) *ShellConfig {
	var out *ShellConfig
	if defaults != nil {
		out = copyTargetShellToFlat(defaults.Shell)
	}
	if target != nil && target.Shell != nil {
		out = overlayTargetShell(out, target.Shell)
	}
	return normalizeShell(out)
}

func copyTargetShellToFlat(in *TargetShellConfig) *ShellConfig {
	if in == nil {
		return nil
	}
	out := &ShellConfig{}
	if in.Aliases != nil {
		out.Aliases = make(map[string]ShellAlias, len(in.Aliases))
		for k, v := range in.Aliases {
			if v != nil {
				out.Aliases[k] = *v
			}
		}
	}
	if in.Functions != nil {
		out.Functions = make(map[string]ShellFunction, len(in.Functions))
		for k, v := range in.Functions {
			if v != nil {
				out.Functions[k] = *v
			}
		}
	}
	if in.Source != nil {
		out.Source = copyStrings(in.Source)
	}
	return out
}

func overlayTargetShell(out *ShellConfig, target *TargetShellConfig) *ShellConfig {
	if target == nil {
		return out
	}
	if out == nil {
		out = &ShellConfig{}
	}
	if target.Aliases != nil {
		if out.Aliases == nil {
			out.Aliases = make(map[string]ShellAlias, len(target.Aliases))
		}
		for k, v := range target.Aliases {
			if v == nil {
				delete(out.Aliases, k)
				continue
			}
			out.Aliases[k] = *v
		}
	}
	if target.Functions != nil {
		if out.Functions == nil {
			out.Functions = make(map[string]ShellFunction, len(target.Functions))
		}
		for k, v := range target.Functions {
			if v == nil {
				delete(out.Functions, k)
				continue
			}
			out.Functions[k] = *v
		}
	}
	if target.Source != nil {
		out.Source = copyStrings(target.Source)
	}
	return out
}

func normalizeShell(in *ShellConfig) *ShellConfig {
	if in == nil {
		return nil
	}
	if len(in.Aliases) == 0 {
		in.Aliases = nil
	}
	if len(in.Functions) == 0 {
		in.Functions = nil
	}
	if len(in.Source) == 0 {
		in.Source = nil
	}
	if in.Aliases == nil && in.Functions == nil && in.Source == nil {
		return nil
	}
	return in
}

func mergeServices(defaults, target *TargetBundle) map[string]Service {
	var out map[string]Service
	if defaults != nil {
		for k, v := range defaults.Services {
			if v == nil {
				continue
			}
			if out == nil {
				out = make(map[string]Service, len(defaults.Services))
			}
			out[k] = copyService(*v)
		}
	}
	if target != nil {
		for k, v := range target.Services {
			if v == nil {
				delete(out, k)
				continue
			}
			if out == nil {
				out = make(map[string]Service, len(target.Services))
			}
			out[k] = copyService(*v)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func mergeFiles(defaults, target *TargetBundle) *FilesConfig {
	var out *FilesConfig
	if defaults != nil {
		out = copyFilesConfig(defaults.Files)
	}
	if target == nil || target.Files == nil {
		return out
	}
	if out == nil {
		out = &FilesConfig{}
	}
	if target.Files.Links != nil {
		out.Links = copyFileLinks(target.Files.Links)
	}
	if target.Files.Templates != nil {
		out.Templates = copyFileTemplates(target.Files.Templates)
	}
	if target.Files.Dirs != nil {
		out.Dirs = copyFileDirs(target.Files.Dirs)
	}
	return out
}

func mergeHooks(defaults, target *TargetBundle) *HooksConfig {
	var out *HooksConfig
	if defaults != nil {
		out = copyHooksConfig(defaults.Hooks)
	}
	if target == nil || target.Hooks == nil {
		return out
	}
	if out == nil {
		out = &HooksConfig{}
	}
	if target.Hooks.PreApply != nil {
		out.PreApply = copyHooks(target.Hooks.PreApply)
	}
	if target.Hooks.PostApply != nil {
		out.PostApply = copyHooks(target.Hooks.PostApply)
	}
	if target.Hooks.PreAdd != nil {
		out.PreAdd = copyHooks(target.Hooks.PreAdd)
	}
	if target.Hooks.PostAdd != nil {
		out.PostAdd = copyHooks(target.Hooks.PostAdd)
	}
	if target.Hooks.PreRemove != nil {
		out.PreRemove = copyHooks(target.Hooks.PreRemove)
	}
	if target.Hooks.PostRemove != nil {
		out.PostRemove = copyHooks(target.Hooks.PostRemove)
	}
	if target.Hooks.PreUpgrade != nil {
		out.PreUpgrade = copyHooks(target.Hooks.PreUpgrade)
	}
	if target.Hooks.PostUpgrade != nil {
		out.PostUpgrade = copyHooks(target.Hooks.PostUpgrade)
	}
	return out
}

func copyRepo(in *Repo) *Repo {
	if in == nil {
		return nil
	}
	out := *in
	return &out
}

func copyAdapters(in map[string]AdapterDef) map[string]AdapterDef {
	if in == nil {
		return nil
	}
	out := make(map[string]AdapterDef, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func copyUpdatesConfig(in *UpdatesConfig) *UpdatesConfig {
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

func copyPackages(in []Package) []Package {
	if in == nil {
		return nil
	}
	out := make([]Package, len(in))
	for i, pkg := range in {
		out[i] = pkg
		out[i].Managers = copyStringMap(pkg.Managers)
		out[i].Host = copyHostPredicate(pkg.Host)
	}
	return out
}

func copyService(in Service) Service {
	out := in
	out.Start = copyStrings(in.Start)
	out.Stop = copyStrings(in.Stop)
	out.Restart = copyStrings(in.Restart)
	out.Status = copyStrings(in.Status)
	out.Host = copyHostPredicate(in.Host)
	if in.Launchd != nil {
		v := *in.Launchd
		out.Launchd = &v
	}
	if in.Systemd != nil {
		v := *in.Systemd
		out.Systemd = &v
	}
	return out
}

func copyFilesConfig(in *FilesConfig) *FilesConfig {
	if in == nil {
		return nil
	}
	return &FilesConfig{
		Links:     copyFileLinks(in.Links),
		Templates: copyFileTemplates(in.Templates),
		Dirs:      copyFileDirs(in.Dirs),
	}
}

func copyFileLinks(in []FileLink) []FileLink {
	if in == nil {
		return nil
	}
	out := make([]FileLink, len(in))
	for i, v := range in {
		out[i] = v
		out[i].Host = copyHostPredicate(v.Host)
	}
	return out
}

func copyFileTemplates(in []FileTemplate) []FileTemplate {
	if in == nil {
		return nil
	}
	out := make([]FileTemplate, len(in))
	for i, v := range in {
		out[i] = v
		out[i].Host = copyHostPredicate(v.Host)
	}
	return out
}

func copyFileDirs(in []FileDir) []FileDir {
	if in == nil {
		return nil
	}
	out := make([]FileDir, len(in))
	for i, v := range in {
		out[i] = v
		out[i].Host = copyHostPredicate(v.Host)
	}
	return out
}

func copyHooksConfig(in *HooksConfig) *HooksConfig {
	if in == nil {
		return nil
	}
	return &HooksConfig{
		PreApply:    copyHooks(in.PreApply),
		PostApply:   copyHooks(in.PostApply),
		PreAdd:      copyHooks(in.PreAdd),
		PostAdd:     copyHooks(in.PostAdd),
		PreRemove:   copyHooks(in.PreRemove),
		PostRemove:  copyHooks(in.PostRemove),
		PreUpgrade:  copyHooks(in.PreUpgrade),
		PostUpgrade: copyHooks(in.PostUpgrade),
	}
}

func copyHooks(in []Hook) []Hook {
	if in == nil {
		return nil
	}
	out := make([]Hook, len(in))
	for i, v := range in {
		out[i] = v
		out[i].Host = copyHostPredicate(v.Host)
	}
	return out
}

func copyHostPredicate(in HostPredicate) HostPredicate {
	if in == nil {
		return nil
	}
	return HostPredicate(copyStrings([]string(in)))
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
