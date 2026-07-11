package host

import "github.com/ks1686/genv/internal/schema"

// FilterForHost returns a copy of f containing only records whose Host
// predicate matches host. Records that do not carry a Host predicate are
// copied unchanged. The input spec is not mutated.
//
// Filtering applies to packages, services, file entries, and hooks. Env vars
// and shell entries (aliases, functions, source paths) do not currently carry
// a Host field, so they are copied through unchanged.
func FilterForHost(f *schema.GenvFile, host string) *schema.GenvFile {
	if f == nil {
		return nil
	}

	out := &schema.GenvFile{
		SchemaVersion: f.SchemaVersion,
		Repo:          f.Repo,
	}

	out.Packages = filterPackages(f.Packages, host)
	out.Env = copyEnv(f.Env)
	out.Shell = copyShell(f.Shell)
	out.Services = filterServices(f.Services, host)
	out.Files = filterFiles(f.Files, host)
	out.Hooks = filterHooks(f.Hooks, host)

	return out
}

func filterPackages(in []schema.Package, host string) []schema.Package {
	out := make([]schema.Package, 0, len(in))
	for _, p := range in {
		if Match(p.Host, host) {
			out = append(out, p)
		}
	}
	return out
}

func copyEnv(in map[string]schema.EnvVar) map[string]schema.EnvVar {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]schema.EnvVar, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func copyShell(in *schema.ShellConfig) *schema.ShellConfig {
	if in == nil {
		return nil
	}
	out := &schema.ShellConfig{
		Source: append([]string(nil), in.Source...),
	}
	if len(in.Aliases) > 0 {
		out.Aliases = make(map[string]schema.ShellAlias, len(in.Aliases))
		for k, v := range in.Aliases {
			out.Aliases[k] = v
		}
	}
	if len(in.Functions) > 0 {
		out.Functions = make(map[string]schema.ShellFunction, len(in.Functions))
		for k, v := range in.Functions {
			out.Functions[k] = v
		}
	}
	return out
}

func filterServices(in map[string]schema.Service, host string) map[string]schema.Service {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]schema.Service, len(in))
	for k, v := range in {
		if Match(v.Host, host) {
			out[k] = v
		}
	}
	return out
}

func filterFiles(in *schema.FilesConfig, host string) *schema.FilesConfig {
	if in == nil {
		return nil
	}
	out := &schema.FilesConfig{}
	for _, l := range in.Links {
		if Match(l.Host, host) {
			out.Links = append(out.Links, l)
		}
	}
	for _, t := range in.Templates {
		if Match(t.Host, host) {
			out.Templates = append(out.Templates, t)
		}
	}
	for _, d := range in.Dirs {
		if Match(d.Host, host) {
			out.Dirs = append(out.Dirs, d)
		}
	}
	return out
}

func filterHooks(in *schema.HooksConfig, host string) *schema.HooksConfig {
	if in == nil {
		return nil
	}
	out := &schema.HooksConfig{}
	for _, h := range in.PreApply {
		if Match(h.Host, host) {
			out.PreApply = append(out.PreApply, h)
		}
	}
	for _, h := range in.PostApply {
		if Match(h.Host, host) {
			out.PostApply = append(out.PostApply, h)
		}
	}
	for _, h := range in.PreAdd {
		if Match(h.Host, host) {
			out.PreAdd = append(out.PreAdd, h)
		}
	}
	for _, h := range in.PostAdd {
		if Match(h.Host, host) {
			out.PostAdd = append(out.PostAdd, h)
		}
	}
	for _, h := range in.PreRemove {
		if Match(h.Host, host) {
			out.PreRemove = append(out.PreRemove, h)
		}
	}
	for _, h := range in.PostRemove {
		if Match(h.Host, host) {
			out.PostRemove = append(out.PostRemove, h)
		}
	}
	for _, h := range in.PreUpgrade {
		if Match(h.Host, host) {
			out.PreUpgrade = append(out.PreUpgrade, h)
		}
	}
	for _, h := range in.PostUpgrade {
		if Match(h.Host, host) {
			out.PostUpgrade = append(out.PostUpgrade, h)
		}
	}
	return out
}
