package resolver

import (
	"bytes"
	"strings"
	"testing"

	"github.com/ks1686/gpm/internal/schema"
)

func TestPlan_PreferredManagerAvailable(t *testing.T) {
	f := &schema.GpmFile{
		Packages: []schema.Package{
			{ID: "neovim", Prefer: "brew"},
		},
	}
	actions := Plan(f, map[string]bool{"brew": true})
	if len(actions) != 1 {
		t.Fatalf("expected 1 action, got %d", len(actions))
	}
	a := actions[0]
	if !a.Resolved() {
		t.Fatal("expected resolved")
	}
	if a.Manager != "brew" {
		t.Errorf("manager: got %q, want %q", a.Manager, "brew")
	}
	if a.PkgName != "neovim" {
		t.Errorf("pkgName: got %q, want %q", a.PkgName, "neovim")
	}
}

func TestPlan_PreferredManagerUnavailable_FallsBackToAvailable(t *testing.T) {
	f := &schema.GpmFile{
		Packages: []schema.Package{
			{ID: "neovim", Prefer: "brew"},
		},
	}
	// brew not available; apt is
	actions := Plan(f, map[string]bool{"apt": true})
	a := actions[0]
	if !a.Resolved() {
		t.Fatal("expected resolved via fallback")
	}
	if a.Manager != "apt" {
		t.Errorf("manager: got %q, want %q", a.Manager, "apt")
	}
}

func TestPlan_ManagersMapPicksCorrectName(t *testing.T) {
	f := &schema.GpmFile{
		Packages: []schema.Package{
			{
				ID: "firefox",
				Managers: map[string]string{
					"flatpak": "org.mozilla.firefox",
					"brew":    "firefox",
				},
			},
		},
	}
	actions := Plan(f, map[string]bool{"flatpak": true})
	a := actions[0]
	if !a.Resolved() {
		t.Fatal("expected resolved")
	}
	if a.Manager != "flatpak" {
		t.Errorf("manager: got %q, want %q", a.Manager, "flatpak")
	}
	if a.PkgName != "org.mozilla.firefox" {
		t.Errorf("pkgName: got %q, want %q", a.PkgName, "org.mozilla.firefox")
	}
}

func TestPlan_ManagersMap_FallbackOrder(t *testing.T) {
	// Both brew and flatpak are in managers map; brew is first in fallbackOrder.
	f := &schema.GpmFile{
		Packages: []schema.Package{
			{
				ID: "firefox",
				Managers: map[string]string{
					"flatpak": "org.mozilla.firefox",
					"brew":    "firefox",
				},
			},
		},
	}
	actions := Plan(f, map[string]bool{"brew": true, "flatpak": true})
	a := actions[0]
	if a.Manager != "brew" {
		t.Errorf("expected brew (higher priority), got %q", a.Manager)
	}
}

func TestPlan_Unresolved_NoManagersAvailable(t *testing.T) {
	f := &schema.GpmFile{
		Packages: []schema.Package{
			{ID: "git"},
		},
	}
	actions := Plan(f, map[string]bool{})
	a := actions[0]
	if a.Resolved() {
		t.Fatal("expected unresolved")
	}
	if a.Cmd != nil {
		t.Error("unresolved action should have nil Cmd")
	}
	if a.Manager != "" {
		t.Errorf("unresolved Manager should be empty, got %q", a.Manager)
	}
}

func TestPlan_FallsBackToIDWhenNoManagersMap(t *testing.T) {
	f := &schema.GpmFile{
		Packages: []schema.Package{
			{ID: "git"}, // no managers map, no prefer
		},
	}
	actions := Plan(f, map[string]bool{"apt": true})
	a := actions[0]
	if !a.Resolved() {
		t.Fatal("expected resolved via generic fallback")
	}
	if a.PkgName != "git" {
		t.Errorf("pkgName: got %q, want %q", a.PkgName, "git")
	}
}

func TestPlan_PreferWithManagersMap_UsesMapName(t *testing.T) {
	f := &schema.GpmFile{
		Packages: []schema.Package{
			{
				ID:     "neovim",
				Prefer: "brew",
				Managers: map[string]string{
					"brew": "neovim",
					"apt":  "neovim",
				},
			},
		},
	}
	actions := Plan(f, map[string]bool{"brew": true})
	a := actions[0]
	if a.Manager != "brew" {
		t.Errorf("manager: got %q, want %q", a.Manager, "brew")
	}
	if a.PkgName != "neovim" {
		t.Errorf("pkgName: got %q, want %q", a.PkgName, "neovim")
	}
}

func TestPrintPlan_NoCrash_AllUnresolved(t *testing.T) {
	f := &schema.GpmFile{
		Packages: []schema.Package{
			{ID: "git"},
			{ID: "neovim", Prefer: "brew"},
		},
	}
	actions := Plan(f, map[string]bool{}) // no managers
	var sb strings.Builder
	PrintPlan(actions, &sb) // must not panic
	out := sb.String()
	if !strings.Contains(out, "git") {
		t.Error("expected git in plan output")
	}
	if !strings.Contains(out, "unresolved") {
		t.Error("expected 'unresolved' in plan output")
	}
}

func TestPrintPlan_ShowsInstallCommand(t *testing.T) {
	f := &schema.GpmFile{
		Packages: []schema.Package{
			{ID: "git"},
		},
	}
	actions := Plan(f, map[string]bool{"brew": true})
	var sb strings.Builder
	PrintPlan(actions, &sb)
	out := sb.String()
	if !strings.Contains(out, "brew install git") {
		t.Errorf("expected 'brew install git' in output, got:\n%s", out)
	}
}

func TestPrintPlan_MixedResolved(t *testing.T) {
	f := &schema.GpmFile{
		Packages: []schema.Package{
			{ID: "git"},
			{ID: "mystery-pkg"},
		},
	}
	// Only git gets resolved (brew available, mystery-pkg falls back to brew too)
	// Actually both would resolve via brew... Let's test with an empty available set
	// so both are unresolved, and separately test with brew so both resolve.
	available := map[string]bool{"brew": true}
	actions := Plan(f, available)

	var sb strings.Builder
	PrintPlan(actions, &sb)
	out := sb.String()
	if !strings.Contains(out, "git") || !strings.Contains(out, "mystery-pkg") {
		t.Errorf("expected both packages in output:\n%s", out)
	}
}

func TestInstallArgs_AllManagers(t *testing.T) {
	tests := []struct {
		mgr     string
		pkg     string
		wantBin string
	}{
		{"apt", "git", "sudo"},
		{"dnf", "git", "sudo"},
		{"pacman", "git", "sudo"},
		{"flatpak", "app.id", "flatpak"},
		{"snap", "git", "sudo"},
		{"brew", "git", "brew"},
		{"linuxbrew", "git", "brew"},
	}
	for _, tc := range tests {
		args := installArgs(tc.mgr, tc.pkg)
		if len(args) == 0 {
			t.Errorf("installArgs(%q, %q): got empty slice", tc.mgr, tc.pkg)
			continue
		}
		if args[0] != tc.wantBin {
			t.Errorf("installArgs(%q, %q): binary = %q, want %q", tc.mgr, tc.pkg, args[0], tc.wantBin)
		}
		if args[len(args)-1] != tc.pkg {
			t.Errorf("installArgs(%q, %q): last arg = %q, want pkg name", tc.mgr, tc.pkg, args[len(args)-1])
		}
	}
}

func TestInstallArgs_UnknownManager(t *testing.T) {
	args := installArgs("yum", "git")
	if args != nil {
		t.Errorf("installArgs for unknown manager should return nil, got %v", args)
	}
}

func TestDetect_ReturnsMap(t *testing.T) {
	m := Detect()
	if m == nil {
		t.Error("Detect() should return a non-nil map")
	}
	// All values in the returned map must be true (only available managers are listed).
	for mgr, ok := range m {
		if !ok {
			t.Errorf("Detect(): map[%q] = false; only true entries should be present", mgr)
		}
	}
}

func TestExecute_SkipsUnresolved(t *testing.T) {
	// An unresolved action has an empty Cmd; Execute must skip it without error.
	actions := []Action{
		{Pkg: schema.Package{ID: "mystery"}, Manager: "", Cmd: nil},
	}
	var out, errOut bytes.Buffer
	errs := Execute(actions, nil, &out, &errOut)
	if len(errs) != 0 {
		t.Errorf("expected no errors for all-unresolved actions, got: %v", errs)
	}
	if out.Len() != 0 {
		t.Errorf("expected no stdout output for all-unresolved actions, got: %q", out.String())
	}
}

func TestExecute_RunsCommand(t *testing.T) {
	// Execute a real "echo" command and verify it produces output and no errors.
	actions := []Action{
		{
			Pkg:     schema.Package{ID: "echo-test"},
			Manager: "apt",
			PkgName: "echo-test",
			Cmd:     []string{"echo", "hello-from-execute"},
		},
	}
	var out, errOut bytes.Buffer
	errs := Execute(actions, nil, &out, &errOut)
	if len(errs) != 0 {
		t.Fatalf("Execute with 'echo': unexpected errors: %v", errs)
	}
	if !strings.Contains(out.String(), "hello-from-execute") {
		t.Errorf("expected 'hello-from-execute' in stdout, got: %q", out.String())
	}
}

func TestExecute_FailedCommand(t *testing.T) {
	// A command that exits non-zero should produce one error entry.
	actions := []Action{
		{
			Pkg:     schema.Package{ID: "failing-pkg"},
			Manager: "apt",
			PkgName: "failing-pkg",
			Cmd:     []string{"false"},
		},
	}
	var out, errOut bytes.Buffer
	errs := Execute(actions, nil, &out, &errOut)
	if len(errs) == 0 {
		t.Error("expected error for failing command, got none")
	}
}

func TestPrintPlan_SinglePackage(t *testing.T) {
	// Singular "package" (not "packages") in the header.
	f := &schema.GpmFile{
		Packages: []schema.Package{{ID: "git"}},
	}
	actions := Plan(f, map[string]bool{"brew": true})
	var sb strings.Builder
	PrintPlan(actions, &sb)
	out := sb.String()
	if !strings.Contains(out, "1 package") {
		t.Errorf("expected '1 package' (singular) in output, got:\n%s", out)
	}
	if strings.Contains(out, "1 packages") {
		t.Errorf("unexpected '1 packages' (plural) in output; should be singular:\n%s", out)
	}
}

func TestPrintPlan_UnresolvedHint(t *testing.T) {
	// When unresolved packages exist the output must mention the hint lines.
	f := &schema.GpmFile{
		Packages: []schema.Package{{ID: "mystery"}},
	}
	actions := Plan(f, map[string]bool{})
	var sb strings.Builder
	PrintPlan(actions, &sb)
	out := sb.String()
	if !strings.Contains(out, "Hint:") {
		t.Errorf("expected 'Hint:' in output for unresolved packages, got:\n%s", out)
	}
	if !strings.Contains(out, "--strict") {
		t.Errorf("expected '--strict' mention in output, got:\n%s", out)
	}
}
