package migrate

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/ks1686/genv/internal/schema"
)

func TestToV8BucketsHostScopedSpec(t *testing.T) {
	input := parseSpec(t, `{
	  "schemaVersion": "7",
	  "packages": [
	    {"id": "shared", "prefer": "snap"},
	    {"id": "mac", "prefer": "brew", "host": "macos"},
	    {"id": "arch", "prefer": "pacman", "host": "arch"},
	    {"id": "wsl", "prefer": "pacman", "host": ["arch", "wsl2"]}
	  ],
	  "env": {
	    "EDITOR": {"value": "nvim"}
	  },
	  "shell": {
	    "aliases": {"ll": {"value": "ls -la"}},
	    "functions": {"hello": {"body": "echo hello"}},
	    "source": ["~/.profile"]
	  },
	  "services": {
	    "macsvc": {"brew_formula": "postgresql@16", "host": "macos"},
	    "sharedsvc": {"start": ["echo", "start"]}
	  },
	  "files": {
	    "links": [
	      {"source": "dot/zshrc", "target": "~/.zshrc", "host": "macos"},
	      {"source": "dot/profile", "target": "~/.profile"}
	    ],
	    "dirs": [
	      {"target": "~/.config/genv", "host": ["arch", "wsl2"]}
	    ]
	  },
	  "hooks": {
	    "postApply": [
	      {"command": "echo shared"},
	      {"command": "echo mac", "host": "macos"}
	    ]
	  },
	  "repo": {"url": "https://example.com/spec.git", "ref": "main"},
	  "updates": {"enabled": true, "interval": "24h"}
	}`)

	got, warnings, err := ToV8(input)
	if err != nil {
		t.Fatalf("ToV8 returned error: %v", err)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "without host predicates") {
		t.Fatalf("warnings = %v, want unscoped package copy warning", warnings)
	}
	if got.SchemaVersion != schema.Version8 {
		t.Fatalf("schemaVersion = %q, want %q", got.SchemaVersion, schema.Version8)
	}
	if got.Repo == nil || got.Repo.URL != "https://example.com/spec.git" || got.Updates == nil || got.Updates.Interval != "24h" {
		t.Fatalf("repo/updates not preserved: repo=%+v updates=%+v", got.Repo, got.Updates)
	}
	if len(got.Packages) != 0 || got.Env != nil || got.Shell != nil || got.Services != nil || got.Files != nil || got.Hooks != nil {
		t.Fatalf("legacy top-level fields not stripped: %+v", got)
	}
	if got.Defaults == nil || got.Defaults.Env["EDITOR"] == nil || got.Defaults.Env["EDITOR"].Value != "nvim" {
		t.Fatalf("shared env not migrated to defaults: %+v", got.Defaults)
	}
	if got.Defaults.Shell == nil || got.Defaults.Shell.Aliases["ll"] == nil || got.Defaults.Shell.Functions["hello"] == nil || len(got.Defaults.Shell.Source) != 1 {
		t.Fatalf("shared shell not migrated to defaults: %+v", got.Defaults.Shell)
	}

	wantTargets := []string{"macos", "arch", "wsl-arch"}
	for _, target := range wantTargets {
		if got.Targets[target] == nil {
			t.Fatalf("missing target %q in %+v", target, got.Targets)
		}
		if !hasPackage(got.Targets[target].Packages, "shared") {
			t.Fatalf("empty-host package not copied to %s: %+v", target, got.Targets[target].Packages)
		}
		if _, ok := got.Targets[target].Services["sharedsvc"]; !ok {
			t.Fatalf("empty-host service not copied to %s: %+v", target, got.Targets[target].Services)
		}
		if got.Targets[target].Files == nil || !hasLink(got.Targets[target].Files.Links, "dot/profile") {
			t.Fatalf("empty-host file link not copied to %s: %+v", target, got.Targets[target].Files)
		}
		if got.Targets[target].Hooks == nil || !hasHook(got.Targets[target].Hooks.PostApply, "echo shared") {
			t.Fatalf("empty-host hook not copied to %s: %+v", target, got.Targets[target].Hooks)
		}
	}
	if !hasPackage(got.Targets["macos"].Packages, "mac") || !hasLink(got.Targets["macos"].Files.Links, "dot/zshrc") {
		t.Fatalf("macos entries not bucketed: %+v", got.Targets["macos"])
	}
	if !hasPackage(got.Targets["arch"].Packages, "arch") || !hasPackage(got.Targets["arch"].Packages, "wsl") {
		t.Fatalf("arch entries not bucketed: %+v", got.Targets["arch"].Packages)
	}
	if !hasPackage(got.Targets["wsl-arch"].Packages, "wsl") || got.Targets["wsl-arch"].Files == nil || !hasDir(got.Targets["wsl-arch"].Files.Dirs, "~/.config/genv") {
		t.Fatalf("wsl2 entries not bucketed into wsl-arch: %+v", got.Targets["wsl-arch"])
	}

	data, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), `"host"`) {
		t.Fatalf("migrated JSON still contains host: %s", data)
	}
	if _, errs, err := schema.ParseAndValidate(data); err != nil || len(errs) > 0 {
		t.Fatalf("migrated spec failed validation: err=%v errs=%v json=%s", err, errs, data)
	}
}

func TestToV8DefaultsToLinuxWhenNoHostPredicates(t *testing.T) {
	input := parseSpec(t, `{
	  "schemaVersion": "7",
	  "packages": [{"id": "git", "prefer": "snap"}],
	  "env": {"EDITOR": {"value": "vim"}}
	}`)

	got, warnings, err := ToV8(input)
	if err != nil {
		t.Fatalf("ToV8 returned error: %v", err)
	}
	if len(warnings) == 0 {
		t.Fatal("expected warning when no concrete target can be inferred")
	}
	if got.Targets["linux"] == nil || !hasPackage(got.Targets["linux"].Packages, "git") {
		t.Fatalf("empty-host package not placed in linux fallback: %+v", got.Targets)
	}
}

func TestToV8WarnsForBareWSL2(t *testing.T) {
	input := parseSpec(t, `{
	  "schemaVersion": "7",
	  "packages": [{"id": "wsl-only", "host": "wsl2"}]
	}`)

	got, warnings, err := ToV8(input)
	if err != nil {
		t.Fatalf("ToV8 returned error: %v", err)
	}
	if got.Targets["wsl-arch"] == nil || !hasPackage(got.Targets["wsl-arch"].Packages, "wsl-only") {
		t.Fatalf("bare wsl2 package not bucketed into wsl-arch: %+v", got.Targets)
	}
	if len(warnings) == 0 || !strings.Contains(warnings[0], "Ubuntu WSL") {
		t.Fatalf("expected Ubuntu WSL warning, got %v", warnings)
	}
}

func parseSpec(t *testing.T, raw string) *schema.GenvFile {
	t.Helper()
	f, errs, err := schema.ParseAndValidate([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if len(errs) > 0 {
		t.Fatalf("validation errors: %v", errs)
	}
	return f
}

func hasPackage(packages []schema.Package, id string) bool {
	for _, pkg := range packages {
		if pkg.ID == id && len(pkg.Host) == 0 {
			return true
		}
	}
	return false
}

func hasLink(links []schema.FileLink, source string) bool {
	for _, link := range links {
		if link.Source == source && len(link.Host) == 0 {
			return true
		}
	}
	return false
}

func hasDir(dirs []schema.FileDir, target string) bool {
	for _, dir := range dirs {
		if dir.Target == target && len(dir.Host) == 0 {
			return true
		}
	}
	return false
}

func hasHook(hooks []schema.Hook, command string) bool {
	for _, hook := range hooks {
		if hook.Command == command && len(hook.Host) == 0 {
			return true
		}
	}
	return false
}
