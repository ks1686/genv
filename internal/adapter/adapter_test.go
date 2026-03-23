package adapter

import (
	"testing"

	"github.com/ks1686/genv/internal/schema"
)

// TestAllAdapterNames verifies that every adapter in the registry has a
// non-empty, unique name and is reachable via ByName.
func TestAllAdapterNames(t *testing.T) {
	seen := make(map[string]bool)
	for _, a := range All {
		name := a.Name()
		if name == "" {
			t.Errorf("adapter %T: Name() returned empty string", a)
		}
		if seen[name] {
			t.Errorf("duplicate adapter name %q in registry", name)
		}
		seen[name] = true
	}
}

// TestByName_KnownManagers verifies that every adapter in All is reachable by name.
func TestByName_KnownManagers(t *testing.T) {
	for _, a := range All {
		got := ByName(a.Name())
		if got == nil {
			t.Errorf("ByName(%q): returned nil, want non-nil", a.Name())
		}
		if got != nil && got.Name() != a.Name() {
			t.Errorf("ByName(%q): returned adapter with name %q", a.Name(), got.Name())
		}
	}
}

// TestByName_UnknownManager verifies ByName returns nil for unregistered names.
func TestByName_UnknownManager(t *testing.T) {
	if got := ByName("yum"); got != nil {
		t.Errorf("ByName(\"yum\"): expected nil, got %v", got)
	}
	if got := ByName(""); got != nil {
		t.Errorf("ByName(\"\"): expected nil, got %v", got)
	}
}

// TestNormalizeID_ExplicitMapping verifies that a manager-specific name in the
// managers map takes precedence over the canonical ID.
func TestNormalizeID_ExplicitMapping(t *testing.T) {
	tests := []struct {
		mgrName  string
		id       string
		managers map[string]string
		wantName string
		wantExp  bool
	}{
		{"apt", "vim", map[string]string{"apt": "vim-nox"}, "vim-nox", true},
		{"dnf", "vim", map[string]string{"dnf": "vim-enhanced"}, "vim-enhanced", true},
		{"pacman", "vim", map[string]string{"pacman": "vim"}, "vim", true},
		{"paru", "vim", map[string]string{"paru": "vim-aur"}, "vim-aur", true},
		{"yay", "vim", map[string]string{"yay": "vim-aur"}, "vim-aur", true},
		{"flatpak", "firefox", map[string]string{"flatpak": "org.mozilla.firefox"}, "org.mozilla.firefox", true},
		{"snap", "code", map[string]string{"snap": "code"}, "code", true},
		{"brew", "neovim", map[string]string{"brew": "neovim"}, "neovim", true},
		{"macports", "neovim", map[string]string{"macports": "neovim"}, "neovim", true},
		{"linuxbrew", "neovim", map[string]string{"linuxbrew": "neovim"}, "neovim", true},
	}
	for _, tc := range tests {
		t.Run(tc.mgrName+"/explicit", func(t *testing.T) {
			a := ByName(tc.mgrName)
			if a == nil {
				t.Fatalf("ByName(%q): no adapter", tc.mgrName)
			}
			name, explicit := a.NormalizeID(tc.id, tc.managers)
			if name != tc.wantName {
				t.Errorf("name: got %q, want %q", name, tc.wantName)
			}
			if explicit != tc.wantExp {
				t.Errorf("explicit: got %v, want %v", explicit, tc.wantExp)
			}
		})
	}
}

// TestNormalizeID_FallbackToID verifies that each adapter falls back to the
// canonical ID when no manager-specific entry exists in the managers map.
func TestNormalizeID_FallbackToID(t *testing.T) {
	for _, a := range All {
		t.Run(a.Name()+"/fallback", func(t *testing.T) {
			name, explicit := a.NormalizeID("git", nil)
			if name != "git" {
				t.Errorf("%s NormalizeID fallback: got %q, want \"git\"", a.Name(), name)
			}
			if explicit {
				t.Errorf("%s NormalizeID fallback: explicit should be false", a.Name())
			}
		})
	}
}

// TestPlanInstall_NonEmpty verifies that every registered adapter returns a
// non-empty command slice from PlanInstall and that the package name is the
// last argument.
func TestPlanInstall_NonEmpty(t *testing.T) {
	for _, a := range All {
		t.Run(a.Name(), func(t *testing.T) {
			args := a.PlanInstall("git")
			if len(args) == 0 {
				t.Errorf("%s PlanInstall: returned empty slice", a.Name())
				return
			}
			if args[len(args)-1] != "git" {
				t.Errorf("%s PlanInstall: last arg = %q, want \"git\"", a.Name(), args[len(args)-1])
			}
		})
	}
}

// TestPlanInstall_ExpectedBinaries verifies that each adapter uses the expected
// leading binary (sudo or the manager binary itself).
func TestPlanInstall_ExpectedBinaries(t *testing.T) {
	tests := []struct {
		mgr     string
		wantBin string
	}{
		{"apt", "sudo"},
		{"dnf", "sudo"},
		{"pacman", "sudo"},
		{"paru", "paru"},
		{"yay", "yay"},
		{"flatpak", "flatpak"},
		{"snap", "sudo"},
		{"brew", "brew"},
		{"macports", "sudo"},
		{"linuxbrew", "brew"},
	}
	for _, tc := range tests {
		t.Run(tc.mgr, func(t *testing.T) {
			a := ByName(tc.mgr)
			if a == nil {
				t.Fatalf("ByName(%q): no adapter", tc.mgr)
			}
			args := a.PlanInstall("pkg")
			if args[0] != tc.wantBin {
				t.Errorf("%s PlanInstall: binary = %q, want %q", tc.mgr, args[0], tc.wantBin)
			}
		})
	}
}

// TestPlanUninstall_NonEmpty verifies that every adapter returns a non-empty
// command slice from PlanUninstall and that the package name is the last argument.
func TestPlanUninstall_NonEmpty(t *testing.T) {
	for _, a := range All {
		t.Run(a.Name(), func(t *testing.T) {
			args := a.PlanUninstall("git")
			if len(args) == 0 {
				t.Errorf("%s PlanUninstall: returned empty slice", a.Name())
				return
			}
			if args[len(args)-1] != "git" {
				t.Errorf("%s PlanUninstall: last arg = %q, want \"git\"", a.Name(), args[len(args)-1])
			}
		})
	}
}

// TestPlanUninstall_ExpectedBinaries verifies each adapter uses the expected
// leading binary for uninstall.
func TestPlanUninstall_ExpectedBinaries(t *testing.T) {
	tests := []struct {
		mgr     string
		wantBin string
	}{
		{"apt", "sudo"},
		{"dnf", "sudo"},
		{"pacman", "sudo"},
		{"paru", "paru"},
		{"yay", "yay"},
		{"flatpak", "flatpak"},
		{"snap", "sudo"},
		{"brew", "brew"},
		{"macports", "sudo"},
		{"linuxbrew", "brew"},
	}
	for _, tc := range tests {
		t.Run(tc.mgr, func(t *testing.T) {
			a := ByName(tc.mgr)
			if a == nil {
				t.Fatalf("ByName(%q): no adapter", tc.mgr)
			}
			args := a.PlanUninstall("pkg")
			if args[0] != tc.wantBin {
				t.Errorf("%s PlanUninstall: binary = %q, want %q", tc.mgr, args[0], tc.wantBin)
			}
		})
	}
}

// TestPlanClean_ValidCommands verifies that every adapter's PlanClean returns
// either nil or a slice of non-empty command argv slices.
func TestPlanClean_ValidCommands(t *testing.T) {
	for _, a := range All {
		t.Run(a.Name(), func(t *testing.T) {
			cmds := a.PlanClean()
			for i, cmd := range cmds {
				if len(cmd) == 0 {
					t.Errorf("%s PlanClean: command[%d] is empty", a.Name(), i)
				}
			}
		})
	}
}

// TestKnownManagersMatchesRegistry verifies that schema.KnownManagers and
// adapter.All are in sync: every adapter name is a known manager and every
// known manager has a registered adapter. Adding one without the other will
// cause this test to fail, preventing silent drift between the two lists.
func TestKnownManagersMatchesRegistry(t *testing.T) {
	adapterNames := make(map[string]bool, len(All))
	for _, a := range All {
		adapterNames[a.Name()] = true
	}
	for mgr := range schema.KnownManagers {
		if !adapterNames[mgr] {
			t.Errorf("schema.KnownManagers[%q] has no corresponding adapter in adapter.All", mgr)
		}
	}
	for name := range adapterNames {
		if !schema.KnownManagers[name] {
			t.Errorf("adapter %q is in adapter.All but missing from schema.KnownManagers", name)
		}
	}
}
