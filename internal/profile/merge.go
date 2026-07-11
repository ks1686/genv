package profile

import (
	"github.com/ks1686/genv/internal/schema"
)

// Merge combines a base GenvFile with a profile GenvFile.
// Base entries are always included. Profile entries add or override.
// - Packages: merged by ID. Profile overrides base if ID matches.
// - Env: merged by Name. Profile overrides base.
// - Shell: Aliases/Functions merged by Name. Source appended.
// - Services: merged by Name. Profile overrides base.
// - Files: merged by Target. Profile overrides base.
// - Hooks: merged by phase. Profile appends to base.
// - Updates: Profile overrides base if set.
// - Repo: Profile overrides base if set.
func Merge(base, prof *schema.GenvFile) *schema.GenvFile {
	merged := &schema.GenvFile{
		SchemaVersion: base.SchemaVersion,
	}
	if prof.SchemaVersion > merged.SchemaVersion {
		merged.SchemaVersion = prof.SchemaVersion
	}

	// Merge Packages
	pkgMap := make(map[string]schema.Package)
	var pkgOrder []string
	for _, p := range base.Packages {
		pkgMap[p.ID] = p
		pkgOrder = append(pkgOrder, p.ID)
	}
	for _, p := range prof.Packages {
		if _, exists := pkgMap[p.ID]; !exists {
			pkgOrder = append(pkgOrder, p.ID)
		}
		pkgMap[p.ID] = p
	}
	for _, id := range pkgOrder {
		merged.Packages = append(merged.Packages, pkgMap[id])
	}

	// Merge Env
	if len(base.Env) > 0 || len(prof.Env) > 0 {
		merged.Env = make(map[string]schema.EnvVar)
		for k, v := range base.Env {
			merged.Env[k] = v
		}
		for k, v := range prof.Env {
			merged.Env[k] = v
		}
	}

	// Merge Shell
	if base.Shell != nil || prof.Shell != nil {
		merged.Shell = &schema.ShellConfig{}
		if base.Shell != nil {
			if len(base.Shell.Aliases) > 0 {
				merged.Shell.Aliases = make(map[string]schema.ShellAlias)
				for k, v := range base.Shell.Aliases {
					merged.Shell.Aliases[k] = v
				}
			}
			if len(base.Shell.Functions) > 0 {
				merged.Shell.Functions = make(map[string]schema.ShellFunction)
				for k, v := range base.Shell.Functions {
					merged.Shell.Functions[k] = v
				}
			}
			merged.Shell.Source = append([]string(nil), base.Shell.Source...)
		}
		if prof.Shell != nil {
			if len(prof.Shell.Aliases) > 0 {
				if merged.Shell.Aliases == nil {
					merged.Shell.Aliases = make(map[string]schema.ShellAlias)
				}
				for k, v := range prof.Shell.Aliases {
					merged.Shell.Aliases[k] = v
				}
			}
			if len(prof.Shell.Functions) > 0 {
				if merged.Shell.Functions == nil {
					merged.Shell.Functions = make(map[string]schema.ShellFunction)
				}
				for k, v := range prof.Shell.Functions {
					merged.Shell.Functions[k] = v
				}
			}
			sourceMap := make(map[string]bool)
			for _, s := range merged.Shell.Source {
				sourceMap[s] = true
			}
			for _, s := range prof.Shell.Source {
				if !sourceMap[s] {
					merged.Shell.Source = append(merged.Shell.Source, s)
					sourceMap[s] = true
				}
			}
		}
	}

	// Merge Services
	if len(base.Services) > 0 || len(prof.Services) > 0 {
		merged.Services = make(map[string]schema.Service)
		for k, v := range base.Services {
			merged.Services[k] = v
		}
		for k, v := range prof.Services {
			merged.Services[k] = v
		}
	}

	// Merge Files
	if base.Files != nil || prof.Files != nil {
		merged.Files = &schema.FilesConfig{}
		if base.Files != nil {
			merged.Files.Links = append([]schema.FileLink(nil), base.Files.Links...)
			merged.Files.Templates = append([]schema.FileTemplate(nil), base.Files.Templates...)
			merged.Files.Dirs = append([]schema.FileDir(nil), base.Files.Dirs...)
		}
		if prof.Files != nil {
			linkMap := make(map[string]int)
			for i, l := range merged.Files.Links {
				linkMap[l.Target] = i
			}
			for _, l := range prof.Files.Links {
				if idx, exists := linkMap[l.Target]; exists {
					merged.Files.Links[idx] = l
				} else {
					merged.Files.Links = append(merged.Files.Links, l)
					linkMap[l.Target] = len(merged.Files.Links) - 1
				}
			}

			tmplMap := make(map[string]int)
			for i, t := range merged.Files.Templates {
				tmplMap[t.Target] = i
			}
			for _, t := range prof.Files.Templates {
				if idx, exists := tmplMap[t.Target]; exists {
					merged.Files.Templates[idx] = t
				} else {
					merged.Files.Templates = append(merged.Files.Templates, t)
					tmplMap[t.Target] = len(merged.Files.Templates) - 1
				}
			}

			dirMap := make(map[string]int)
			for i, d := range merged.Files.Dirs {
				dirMap[d.Target] = i
			}
			for _, d := range prof.Files.Dirs {
				if idx, exists := dirMap[d.Target]; exists {
					merged.Files.Dirs[idx] = d
				} else {
					merged.Files.Dirs = append(merged.Files.Dirs, d)
					dirMap[d.Target] = len(merged.Files.Dirs) - 1
				}
			}
		}
	}

	// Merge Hooks
	if base.Hooks != nil || prof.Hooks != nil {
		merged.Hooks = &schema.HooksConfig{}
		if base.Hooks != nil {
			merged.Hooks.PreApply = append([]schema.Hook(nil), base.Hooks.PreApply...)
			merged.Hooks.PostApply = append([]schema.Hook(nil), base.Hooks.PostApply...)
			merged.Hooks.PreAdd = append([]schema.Hook(nil), base.Hooks.PreAdd...)
			merged.Hooks.PostAdd = append([]schema.Hook(nil), base.Hooks.PostAdd...)
			merged.Hooks.PreRemove = append([]schema.Hook(nil), base.Hooks.PreRemove...)
			merged.Hooks.PostRemove = append([]schema.Hook(nil), base.Hooks.PostRemove...)
			merged.Hooks.PreUpgrade = append([]schema.Hook(nil), base.Hooks.PreUpgrade...)
			merged.Hooks.PostUpgrade = append([]schema.Hook(nil), base.Hooks.PostUpgrade...)
		}
		if prof.Hooks != nil {
			merged.Hooks.PreApply = append(merged.Hooks.PreApply, prof.Hooks.PreApply...)
			merged.Hooks.PostApply = append(merged.Hooks.PostApply, prof.Hooks.PostApply...)
			merged.Hooks.PreAdd = append(merged.Hooks.PreAdd, prof.Hooks.PreAdd...)
			merged.Hooks.PostAdd = append(merged.Hooks.PostAdd, prof.Hooks.PostAdd...)
			merged.Hooks.PreRemove = append(merged.Hooks.PreRemove, prof.Hooks.PreRemove...)
			merged.Hooks.PostRemove = append(merged.Hooks.PostRemove, prof.Hooks.PostRemove...)
			merged.Hooks.PreUpgrade = append(merged.Hooks.PreUpgrade, prof.Hooks.PreUpgrade...)
			merged.Hooks.PostUpgrade = append(merged.Hooks.PostUpgrade, prof.Hooks.PostUpgrade...)
		}
	}

	// Merge Updates
	if prof.Updates != nil {
		merged.Updates = prof.Updates
	} else if base.Updates != nil {
		merged.Updates = base.Updates
	}

	// Merge Repo
	if prof.Repo != nil {
		merged.Repo = prof.Repo
	} else if base.Repo != nil {
		merged.Repo = base.Repo
	}

	return merged
}
