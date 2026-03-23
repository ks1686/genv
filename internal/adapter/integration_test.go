//go:build integration

// Integration tests for the adapter layer.
//
// These tests run real package-manager binaries on the current host.
// Each test skips itself when its adapter is not available, so the same
// command works on any supported platform: only the relevant adapters run.
//
// Usage:
//
//	go test -tags integration ./internal/adapter/
//	go test -tags integration -v ./internal/adapter/
//	go test -tags integration -run TestApt ./internal/adapter/
//
// Do NOT run with -count > 1; some query commands have side effects on
// the package database cache.

package adapter_test

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/ks1686/gpm/internal/adapter"
)

// knownAbsent is a package name that should never be installed anywhere.
const knownAbsent = "gpm-integration-test-nonexistent-package-xyzzy"

// adapterSuite holds the per-adapter configuration used by runAdapterSuite.
type adapterSuite struct {
	a              adapter.Adapter
	wantBin        string            // expected first arg of PlanInstall
	explicitMap    map[string]string // managers map for NormalizeID explicit test
	explicitWant   string            // expected NormalizeID result when explicitMap is used
	knownInstalled string            // a package reliably installed on this host; empty = skip
}

// runAdapterSuite runs the standard set of sub-tests for one adapter.
// It is called by each per-adapter TestXxx function so that -run TestApt etc.
// continue to work while the test logic lives in exactly one place.
func runAdapterSuite(t *testing.T, s adapterSuite) {
	t.Helper()

	if !s.a.Available() {
		t.Skipf("%s not available on this host", s.a.Name())
	}

	t.Run("Name", func(t *testing.T) {
		if s.a.Name() == "" {
			t.Error("Name() returned empty string")
		}
	})

	t.Run("PlanInstall_structure", func(t *testing.T) {
		cmd := s.a.PlanInstall("testpkg")
		assertInstallCmd(t, cmd, s.wantBin, "testpkg")
	})

	t.Run("NormalizeID_fallback", func(t *testing.T) {
		name, explicit := s.a.NormalizeID("curl", nil)
		if name != "curl" || explicit {
			t.Errorf("NormalizeID fallback: got (%q, %v), want (\"curl\", false)", name, explicit)
		}
	})

	t.Run("NormalizeID_explicit", func(t *testing.T) {
		name, explicit := s.a.NormalizeID("pkg", s.explicitMap)
		if name != s.explicitWant || !explicit {
			t.Errorf("NormalizeID explicit: got (%q, %v), want (%q, true)", name, explicit, s.explicitWant)
		}
	})

	if s.knownInstalled != "" {
		t.Run("Query_installed", func(t *testing.T) {
			installed, err := s.a.Query(s.knownInstalled)
			if err != nil {
				t.Fatalf("Query(%q): unexpected error: %v", s.knownInstalled, err)
			}
			if !installed {
				t.Errorf("Query(%q): expected installed=true on a stock %s system", s.knownInstalled, s.a.Name())
			}
		})
	}

	t.Run("Query_absent", func(t *testing.T) {
		installed, err := s.a.Query(knownAbsent)
		if err != nil {
			t.Fatalf("Query(%q): unexpected error: %v", knownAbsent, err)
		}
		if installed {
			t.Errorf("Query(%q): expected installed=false", knownAbsent)
		}
	})

	t.Run("ListInstalled_returns_slice", func(t *testing.T) {
		pkgs, err := s.a.ListInstalled()
		if err != nil {
			t.Fatalf("ListInstalled(): unexpected error: %v", err)
		}
		// A stock system always has at least one package managed by its package manager.
		if len(pkgs) == 0 {
			t.Errorf("ListInstalled(): expected at least one package on a stock system")
		}
	})

	if s.knownInstalled != "" {
		t.Run("QueryVersion_installed_package", func(t *testing.T) {
			ver, err := s.a.QueryVersion(s.knownInstalled)
			if err != nil {
				t.Fatalf("QueryVersion(%q): unexpected error: %v", s.knownInstalled, err)
			}
			if ver == "" {
				t.Errorf("QueryVersion(%q): expected non-empty version string", s.knownInstalled)
			}
		})
	}

	t.Run("QueryVersion_absent_package_no_error", func(t *testing.T) {
		// QueryVersion on an absent package must return ("", nil) — not an error.
		_, err := s.a.QueryVersion(knownAbsent)
		if err != nil {
			t.Errorf("QueryVersion(%q): expected nil error for absent package, got: %v", knownAbsent, err)
		}
	})
}

// ---- per-adapter entry points (keep named so -run TestApt etc. work) --------

func TestApt(t *testing.T) {
	runAdapterSuite(t, adapterSuite{
		a:              adapter.Apt{},
		wantBin:        "sudo",
		explicitMap:    map[string]string{"apt": "vim-nox"},
		explicitWant:   "vim-nox",
		knownInstalled: "bash",
	})
}

func TestDnf(t *testing.T) {
	runAdapterSuite(t, adapterSuite{
		a:              adapter.Dnf{},
		wantBin:        "sudo",
		explicitMap:    map[string]string{"dnf": "vim-enhanced"},
		explicitWant:   "vim-enhanced",
		knownInstalled: "bash",
	})
}

func TestPacman(t *testing.T) {
	runAdapterSuite(t, adapterSuite{
		a:              adapter.Pacman{},
		wantBin:        "sudo",
		explicitMap:    map[string]string{"pacman": "vim"},
		explicitWant:   "vim",
		knownInstalled: "bash",
	})
}

func TestParu(t *testing.T) {
	runAdapterSuite(t, adapterSuite{
		a:              adapter.Paru{},
		wantBin:        "paru", // must NOT be "sudo" — paru handles escalation itself
		explicitMap:    map[string]string{"paru": "vim-aur"},
		explicitWant:   "vim-aur",
		knownInstalled: "bash",
	})
}

func TestYay(t *testing.T) {
	runAdapterSuite(t, adapterSuite{
		a:              adapter.Yay{},
		wantBin:        "yay", // must NOT be "sudo" — yay handles escalation itself
		explicitMap:    map[string]string{"yay": "vim-aur"},
		explicitWant:   "vim-aur",
		knownInstalled: "bash",
	})
}

func TestFlatpak(t *testing.T) {
	runAdapterSuite(t, adapterSuite{
		a:            adapter.Flatpak{},
		wantBin:      "flatpak",
		explicitMap:  map[string]string{"flatpak": "org.mozilla.firefox"},
		explicitWant: "org.mozilla.firefox",
		// No knownInstalled: no package is universally pre-installed via flatpak.
	})
}

// TestFlatpak_Query_WithRemote checks Query when Flathub is configured.
// Requires: flatpak remote-add --if-not-exists flathub https://flathub.org/repo/flathub.flatpakrepo
func TestFlatpak_Query_WithRemote(t *testing.T) {
	a := adapter.Flatpak{}
	if !a.Available() {
		t.Skip("flatpak not available on this host")
	}
	installed, err := a.Query(knownAbsent)
	if err != nil {
		t.Fatalf("Query(%q): unexpected error: %v", knownAbsent, err)
	}
	if installed {
		t.Errorf("Query(%q): expected installed=false", knownAbsent)
	}
}

func TestSnap(t *testing.T) {
	runAdapterSuite(t, adapterSuite{
		a:            adapter.Snap{},
		wantBin:      "sudo",
		explicitMap:  map[string]string{"snap": "hello"},
		explicitWant: "hello",
		// No knownInstalled: no snap is universally pre-installed.
	})
}

func TestBrew(t *testing.T) {
	runAdapterSuite(t, adapterSuite{
		a:            adapter.Brew{},
		wantBin:      "brew",
		explicitMap:  map[string]string{"brew": "neovim"},
		explicitWant: "neovim",
		// No knownInstalled: no Homebrew formula is universally pre-installed.
	})
}

func TestBrew_Query_Cask(t *testing.T) {
	a := adapter.Brew{}
	if !a.Available() {
		t.Skip("brew not available on this host")
	}
	// Pick the first installed cask dynamically so the test works on any machine.
	out, err := exec.Command("brew", "list", "--cask").Output()
	if err != nil || len(strings.TrimSpace(string(out))) == 0 {
		t.Skip("no casks installed on this host — skipping cask query test")
	}
	cask := strings.Fields(string(out))[0]
	installed, err := a.Query(cask)
	if err != nil {
		t.Fatalf("Query(%q): unexpected error: %v", cask, err)
	}
	if !installed {
		t.Errorf("Query(%q): expected installed=true for a known installed cask", cask)
	}
}

func TestLinuxbrew(t *testing.T) {
	runAdapterSuite(t, adapterSuite{
		a:            adapter.Linuxbrew{},
		wantBin:      "brew",
		explicitMap:  map[string]string{"linuxbrew": "neovim"},
		explicitWant: "neovim",
	})
}

func TestMacPorts(t *testing.T) {
	runAdapterSuite(t, adapterSuite{
		a:            adapter.MacPorts{},
		wantBin:      "sudo",
		explicitMap:  map[string]string{"macports": "neovim"},
		explicitWant: "neovim",
		// No knownInstalled: MacPorts is not pre-installed in CI; tested on real macOS host only.
	})
}

// ---- shared assertion helpers -----------------------------------------------

// assertInstallCmd checks that cmd starts with wantBin and ends with wantPkg.
func assertInstallCmd(t *testing.T, cmd []string, wantBin, wantPkg string) {
	t.Helper()
	if len(cmd) == 0 {
		t.Fatal("PlanInstall returned empty slice")
	}
	if cmd[0] != wantBin {
		t.Errorf("cmd[0] = %q, want %q", cmd[0], wantBin)
	}
	if cmd[len(cmd)-1] != wantPkg {
		t.Errorf("cmd[last] = %q, want %q", cmd[len(cmd)-1], wantPkg)
	}
}

// assertContains checks that at least one element of cmd equals s.
func assertContains(t *testing.T, cmd []string, s string) {
	t.Helper()
	for _, arg := range cmd {
		if arg == s {
			return
		}
	}
	t.Errorf("expected %q in command %v", s, strings.Join(cmd, " "))
}
