package adapter

import "testing"

// fallbackEligibleManagers is the exhaustive set of managers that may be used
// as a blind default fallback (resolver step 3). Only OS/system package
// managers belong here.
var fallbackEligibleManagers = map[string]bool{
	"brew":      true,
	"mas":       true,
	"pacman":    true,
	"paru":      true,
	"yay":       true,
	"snap":      true,
	"linuxbrew": true,
	"winget":    true,
	"scoop":     true,
	"choco":     true,
}

func TestDefaultFallbackEligible_matchesClassification(t *testing.T) {
	for _, a := range All {
		want := fallbackEligibleManagers[a.Name()]
		got := IsDefaultFallbackEligible(a)
		if got != want {
			t.Errorf("IsDefaultFallbackEligible(%q) = %v, want %v", a.Name(), got, want)
		}
	}
}

func TestDefaultFallbackEligible_everyEligibleManagerIsRegistered(t *testing.T) {
	registered := make(map[string]bool, len(All))
	for _, a := range All {
		registered[a.Name()] = true
	}
	for name := range fallbackEligibleManagers {
		if !registered[name] {
			t.Errorf("fallback-eligible manager %q is not registered in adapter.All", name)
		}
	}
}

func TestDefaultFallbackEligible_ecosystemManagersAreExplicitOnly(t *testing.T) {
	for _, name := range []string{
		"bun", "npm", "pnpm", "yarn", "deno", "volta",
		"uv", "pipx", "pip-user", "poetry", "conda", "mamba", "pixi",
		"cargo", "go", "rustup", "gem", "composer", "dotnet-tool",
		"ghcup", "stack", "opam", "juliaup", "sdkman", "asdf", "mise",
		"krew", "helm", "vscode",
	} {
		a := ByName(name)
		if a == nil {
			t.Fatalf("ByName(%q): no adapter", name)
		}
		if IsDefaultFallbackEligible(a) {
			t.Errorf("ecosystem manager %q must NOT be default-fallback-eligible", name)
		}
	}
}
