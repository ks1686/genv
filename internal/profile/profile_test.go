package profile_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ks1686/genv/internal/profile"
	"github.com/ks1686/genv/internal/schema"
)

func TestMerge(t *testing.T) {
	base := &schema.GenvFile{
		SchemaVersion: schema.Version6,
		Packages: []schema.Package{
			{ID: "pkg1", Prefer: "brew"},
			{ID: "pkg2", Prefer: "brew"},
		},
		Env: map[string]schema.EnvVar{
			"VAR1": {Value: "base1"},
			"VAR2": {Value: "base2"},
		},
		Shell: &schema.ShellConfig{
			Aliases: map[string]schema.ShellAlias{
				"alias1": {Value: "base1"},
			},
			Source: []string{"source1"},
		},
	}

	ext := &schema.GenvFile{
		SchemaVersion: schema.Version6,
		Packages: []schema.Package{
			{ID: "pkg2", Prefer: "apt"},  // override
			{ID: "pkg3", Prefer: "brew"}, // add
		},
		Env: map[string]schema.EnvVar{
			"VAR2": {Value: "ext2"}, // override
			"VAR3": {Value: "ext3"}, // add
		},
		Shell: &schema.ShellConfig{
			Aliases: map[string]schema.ShellAlias{
				"alias1": {Value: "ext1"}, // override
				"alias2": {Value: "ext2"}, // add
			},
			Source: []string{"source1", "source2"}, // deduplicate
		},
	}

	merged := profile.Merge(base, ext)

	if len(merged.Packages) != 3 {
		t.Errorf("expected 3 packages, got %d", len(merged.Packages))
	}
	pkgMap := make(map[string]schema.Package)
	for _, p := range merged.Packages {
		pkgMap[p.ID] = p
	}
	if pkgMap["pkg2"].Prefer != "apt" {
		t.Errorf("expected pkg2 manager to be apt, got %s", pkgMap["pkg2"].Prefer)
	}

	if len(merged.Env) != 3 {
		t.Errorf("expected 3 env vars, got %d", len(merged.Env))
	}
	if merged.Env["VAR2"].Value != "ext2" {
		t.Errorf("expected VAR2 to be ext2, got %s", merged.Env["VAR2"].Value)
	}

	if len(merged.Shell.Aliases) != 2 {
		t.Errorf("expected 2 aliases, got %d", len(merged.Shell.Aliases))
	}
	if merged.Shell.Aliases["alias1"].Value != "ext1" {
		t.Errorf("expected alias1 to be ext1, got %s", merged.Shell.Aliases["alias1"].Value)
	}

	if len(merged.Shell.Source) != 2 {
		t.Errorf("expected 2 sources, got %d", len(merged.Shell.Source))
	}
}

func TestLoadAndCreate(t *testing.T) {
	dir := t.TempDir()
	specPath := filepath.Join(dir, "genv.json")

	// Create base
	base := `{"schemaVersion": "6", "packages": []}`
	if err := os.WriteFile(specPath, []byte(base), 0644); err != nil {
		t.Fatal(err)
	}

	// List should be empty
	profiles, err := profile.List(specPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(profiles) != 0 {
		t.Errorf("expected 0 profiles, got %d", len(profiles))
	}

	// Create profile
	if err := profile.Create(specPath, "work"); err != nil {
		t.Fatal(err)
	}

	// List should have "work"
	profiles, err = profile.List(specPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(profiles) != 1 || profiles[0] != "work" {
		t.Errorf("expected [work], got %v", profiles)
	}

	// Create existing should fail
	if err := profile.Create(specPath, "work"); err == nil {
		t.Error("expected error creating existing profile")
	}

	// LoadMerged
	merged, err := profile.LoadMerged(specPath, "work")
	if err != nil {
		t.Fatal(err)
	}
	if merged.SchemaVersion != schema.Version6 {
		t.Errorf("expected schema version 6, got %s", merged.SchemaVersion)
	}
}

func TestMerge_Full(t *testing.T) {
	base := &schema.GenvFile{
		SchemaVersion: schema.Version6,
		Shell: &schema.ShellConfig{
			Functions: map[string]schema.ShellFunction{
				"func1": {Body: "base1"},
			},
		},
		Services: map[string]schema.Service{
			"svc1": {Start: []string{"base1"}},
		},
		Files: &schema.FilesConfig{
			Links: []schema.FileLink{
				{Target: "link1", Source: "base1"},
			},
			Templates: []schema.FileTemplate{
				{Target: "tmpl1", Source: "base1"},
			},
			Dirs: []schema.FileDir{
				{Target: "dir1"},
			},
		},
		Hooks: &schema.HooksConfig{
			PreUpgrade: []schema.Hook{{Command: "base1"}},
		},
		Updates: &schema.UpdatesConfig{
			AutoApply: true,
		},
		Repo: &schema.Repo{
			URL: "base1",
		},
	}

	ext := &schema.GenvFile{
		SchemaVersion: schema.Version6,
		Shell: &schema.ShellConfig{
			Functions: map[string]schema.ShellFunction{
				"func1": {Body: "ext1"}, // override
				"func2": {Body: "ext2"}, // add
			},
		},
		Services: map[string]schema.Service{
			"svc1": {Start: []string{"ext1"}}, // override
			"svc2": {Start: []string{"ext2"}}, // add
		},
		Files: &schema.FilesConfig{
			Links: []schema.FileLink{
				{Target: "link1", Source: "ext1"}, // override
				{Target: "link2", Source: "ext2"}, // add
			},
			Templates: []schema.FileTemplate{
				{Target: "tmpl1", Source: "ext1"}, // override
				{Target: "tmpl2", Source: "ext2"}, // add
			},
			Dirs: []schema.FileDir{
				{Target: "dir1"}, // override
				{Target: "dir2"}, // add
			},
		},
		Hooks: &schema.HooksConfig{
			PreUpgrade: []schema.Hook{{Command: "ext1"}}, // append
		},
		Updates: &schema.UpdatesConfig{
			AutoApply: false, // override
		},
		Repo: &schema.Repo{
			URL: "ext1", // override
		},
	}

	merged := profile.Merge(base, ext)

	if len(merged.Shell.Functions) != 2 {
		t.Errorf("expected 2 functions, got %d", len(merged.Shell.Functions))
	}
	if merged.Shell.Functions["func1"].Body != "ext1" {
		t.Errorf("expected func1 to be ext1, got %s", merged.Shell.Functions["func1"].Body)
	}

	if len(merged.Services) != 2 {
		t.Errorf("expected 2 services, got %d", len(merged.Services))
	}
	if merged.Services["svc1"].Start[0] != "ext1" {
		t.Errorf("expected svc1 to be ext1, got %s", merged.Services["svc1"].Start[0])
	}

	if len(merged.Files.Links) != 2 {
		t.Errorf("expected 2 links, got %d", len(merged.Files.Links))
	}
	linkMap := make(map[string]schema.FileLink)
	for _, l := range merged.Files.Links {
		linkMap[l.Target] = l
	}
	if linkMap["link1"].Source != "ext1" {
		t.Errorf("expected link1 to be ext1, got %s", linkMap["link1"].Source)
	}

	if len(merged.Files.Templates) != 2 {
		t.Errorf("expected 2 templates, got %d", len(merged.Files.Templates))
	}
	tmplMap := make(map[string]schema.FileTemplate)
	for _, t := range merged.Files.Templates {
		tmplMap[t.Target] = t
	}
	if tmplMap["tmpl1"].Source != "ext1" {
		t.Errorf("expected tmpl1 to be ext1, got %s", tmplMap["tmpl1"].Source)
	}

	if len(merged.Files.Dirs) != 2 {
		t.Errorf("expected 2 dirs, got %d", len(merged.Files.Dirs))
	}
	dirMap := make(map[string]schema.FileDir)
	for _, d := range merged.Files.Dirs {
		dirMap[d.Target] = d
	}
	if dirMap["dir1"].Target != "dir1" {
		t.Errorf("expected dir1 to be dir1, got %s", dirMap["dir1"].Target)
	}

	if len(merged.Hooks.PreUpgrade) != 2 {
		t.Errorf("expected 2 pre-upgrade hooks, got %d", len(merged.Hooks.PreUpgrade))
	}
	if merged.Hooks.PreUpgrade[0].Command != "base1" || merged.Hooks.PreUpgrade[1].Command != "ext1" {
		t.Errorf("expected hooks to be appended, got %v", merged.Hooks.PreUpgrade)
	}

	if merged.Updates.AutoApply != false {
		t.Errorf("expected updates to be overridden")
	}

	if merged.Repo.URL != "ext1" {
		t.Errorf("expected repo to be overridden")
	}
}
