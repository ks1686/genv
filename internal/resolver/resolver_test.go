package resolver

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/ks1686/genv/internal/adapter"
	"github.com/ks1686/genv/internal/genvfile"
	"github.com/ks1686/genv/internal/schema"
)

func TestPlan_PreferredManagerAvailable(t *testing.T) {
	f := &schema.GenvFile{
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
	f := &schema.GenvFile{
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
	f := &schema.GenvFile{
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
	f := &schema.GenvFile{
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
	f := &schema.GenvFile{
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
	f := &schema.GenvFile{
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
	f := &schema.GenvFile{
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
	f := &schema.GenvFile{
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
	f := &schema.GenvFile{
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
	f := &schema.GenvFile{
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

func TestPlanInstall_AllManagers(t *testing.T) {
	tests := []struct {
		mgr     string
		pkg     string
		wantBin string
	}{
		{"apt", "git", "sudo"},
		{"dnf", "git", "sudo"},
		{"pacman", "git", "sudo"},
		{"paru", "git", "paru"},
		{"yay", "git", "yay"},
		{"flatpak", "app.id", "flatpak"},
		{"snap", "git", "sudo"},
		{"brew", "git", "brew"},
		{"macports", "git", "sudo"},
		{"linuxbrew", "git", "brew"},
	}
	for _, tc := range tests {
		a := adapter.ByName(tc.mgr)
		if a == nil {
			t.Errorf("ByName(%q): no adapter found", tc.mgr)
			continue
		}
		args := a.PlanInstall(tc.pkg)
		if len(args) == 0 {
			t.Errorf("PlanInstall(%q, %q): got empty slice", tc.mgr, tc.pkg)
			continue
		}
		if args[0] != tc.wantBin {
			t.Errorf("PlanInstall(%q, %q): binary = %q, want %q", tc.mgr, tc.pkg, args[0], tc.wantBin)
		}
		if args[len(args)-1] != tc.pkg {
			t.Errorf("PlanInstall(%q, %q): last arg = %q, want pkg name", tc.mgr, tc.pkg, args[len(args)-1])
		}
	}
}

func TestByName_UnknownManager(t *testing.T) {
	a := adapter.ByName("yum")
	if a != nil {
		t.Errorf("ByName for unknown manager should return nil, got %v", a)
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
	errs := Execute(context.Background(), actions, nil, &out, &errOut)
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
	errs := Execute(context.Background(), actions, nil, &out, &errOut)
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
	errs := Execute(context.Background(), actions, nil, &out, &errOut)
	if len(errs) == 0 {
		t.Error("expected error for failing command, got none")
	}
}

func TestPrintPlan_SinglePackage(t *testing.T) {
	// Singular "package" (not "packages") in the header.
	f := &schema.GenvFile{
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
	f := &schema.GenvFile{
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

// TestPrintPlan_ReturnsCorrectCounts verifies the resolved/unresolved return
// values for all combinations.
func TestPrintPlan_ReturnsCorrectCounts(t *testing.T) {
	tests := []struct {
		name           string
		pkgs           []schema.Package
		available      map[string]bool
		wantResolved   int
		wantUnresolved int
	}{
		{
			name:           "all resolved",
			pkgs:           []schema.Package{{ID: "git"}, {ID: "neovim"}},
			available:      map[string]bool{"brew": true},
			wantResolved:   2,
			wantUnresolved: 0,
		},
		{
			name:           "all unresolved",
			pkgs:           []schema.Package{{ID: "git"}, {ID: "neovim"}},
			available:      map[string]bool{},
			wantResolved:   0,
			wantUnresolved: 2,
		},
		{
			name: "mixed",
			pkgs: []schema.Package{
				{ID: "git"},
				{
					ID:       "only-flatpak",
					Managers: map[string]string{"flatpak": "io.pkg"},
					Prefer:   "flatpak",
				},
			},
			// brew available → git resolves; prefer=flatpak but flatpak absent → falls
			// back to brew for only-flatpak too (step 3 fallback), so both resolve.
			available:      map[string]bool{"brew": true},
			wantResolved:   2,
			wantUnresolved: 0,
		},
		{
			name:           "empty packages",
			pkgs:           nil,
			available:      map[string]bool{"brew": true},
			wantResolved:   0,
			wantUnresolved: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := &schema.GenvFile{Packages: tc.pkgs}
			actions := Plan(f, tc.available)
			var sb strings.Builder
			resolved, unresolved := PrintPlan(actions, &sb)
			if resolved != tc.wantResolved {
				t.Errorf("resolved: got %d, want %d", resolved, tc.wantResolved)
			}
			if unresolved != tc.wantUnresolved {
				t.Errorf("unresolved: got %d, want %d", unresolved, tc.wantUnresolved)
			}
		})
	}
}

// TestPlan_EmptyPackages verifies that Plan with no packages returns an empty
// slice (not nil) and does not panic.
func TestPlan_EmptyPackages(t *testing.T) {
	f := &schema.GenvFile{Packages: []schema.Package{}}
	actions := Plan(f, map[string]bool{"brew": true})
	if len(actions) != 0 {
		t.Errorf("expected 0 actions, got %d", len(actions))
	}
}

// TestPlan_MultiplePackagesMixed verifies a file with several packages where
// some resolve and some don't.
func TestPlan_MultiplePackagesMixed(t *testing.T) {
	f := &schema.GenvFile{
		Packages: []schema.Package{
			{ID: "git"},
			{ID: "neovim", Prefer: "brew"},
			{ID: "secret-pkg", Prefer: "flatpak", Managers: map[string]string{"flatpak": "io.secret"}},
		},
	}
	// brew available, flatpak absent → git and neovim resolve; secret-pkg's
	// prefer is flatpak (unavailable) and its managers map has only flatpak
	// (unavailable), so it falls back to the generic fallback at step 3 (brew).
	available := map[string]bool{"brew": true}
	actions := Plan(f, available)
	if len(actions) != 3 {
		t.Fatalf("expected 3 actions, got %d", len(actions))
	}
	for _, a := range actions {
		if !a.Resolved() {
			t.Errorf("expected all packages to resolve via brew fallback; %q is unresolved", a.Pkg.ID)
		}
	}
}

// TestNormalizeID verifies that each adapter uses the managers map when present
// and falls back to the package ID otherwise.
func TestNormalizeID(t *testing.T) {
	tests := []struct {
		name         string
		mgr          string
		id           string
		managers     map[string]string
		wantName     string
		wantExplicit bool
	}{
		{
			name:         "uses managers map",
			mgr:          "flatpak",
			id:           "firefox",
			managers:     map[string]string{"flatpak": "org.mozilla.firefox"},
			wantName:     "org.mozilla.firefox",
			wantExplicit: true,
		},
		{
			name:         "falls back to id when no map entry",
			mgr:          "brew",
			id:           "firefox",
			managers:     nil,
			wantName:     "firefox",
			wantExplicit: false,
		},
		{
			name:         "falls back to id when manager not in map",
			mgr:          "brew",
			id:           "firefox",
			managers:     map[string]string{"flatpak": "org.mozilla.firefox"},
			wantName:     "firefox",
			wantExplicit: false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			a := adapter.ByName(tc.mgr)
			if a == nil {
				t.Fatalf("ByName(%q): no adapter found", tc.mgr)
			}
			gotName, gotExplicit := a.NormalizeID(tc.id, tc.managers)
			if gotName != tc.wantName {
				t.Errorf("NormalizeID name: got %q, want %q", gotName, tc.wantName)
			}
			if gotExplicit != tc.wantExplicit {
				t.Errorf("NormalizeID explicit: got %v, want %v", gotExplicit, tc.wantExplicit)
			}
		})
	}
}

// TestExecute_MultipleActions verifies that Execute runs all resolved actions
// and collects errors correctly.
func TestExecute_MultipleActions(t *testing.T) {
	actions := []Action{
		{
			Pkg:     schema.Package{ID: "pkg1"},
			Manager: "apt",
			PkgName: "pkg1",
			Cmd:     []string{"echo", "installing-pkg1"},
		},
		{
			Pkg:     schema.Package{ID: "pkg2"},
			Manager: "apt",
			PkgName: "pkg2",
			Cmd:     []string{"echo", "installing-pkg2"},
		},
	}
	var out, errOut strings.Builder
	errs := Execute(context.Background(), actions, nil, &out, &errOut)
	if len(errs) != 0 {
		t.Fatalf("Execute: unexpected errors: %v", errs)
	}
	if !strings.Contains(out.String(), "installing-pkg1") {
		t.Errorf("expected pkg1 output, got: %q", out.String())
	}
	if !strings.Contains(out.String(), "installing-pkg2") {
		t.Errorf("expected pkg2 output, got: %q", out.String())
	}
}

// TestExecute_MixedResolvedAndUnresolved verifies that Execute only runs
// resolved actions and returns errors only for resolved commands that fail.
func TestExecute_MixedResolvedAndUnresolved(t *testing.T) {
	actions := []Action{
		// Unresolved — must be skipped silently.
		{Pkg: schema.Package{ID: "mystery"}, Manager: "", Cmd: nil},
		// Resolved — runs echo successfully.
		{
			Pkg:     schema.Package{ID: "echo-pkg"},
			Manager: "brew",
			PkgName: "echo-pkg",
			Cmd:     []string{"echo", "ok"},
		},
		// Unresolved — also skipped.
		{Pkg: schema.Package{ID: "another-mystery"}, Manager: "", Cmd: nil},
	}
	var out, errOut strings.Builder
	errs := Execute(context.Background(), actions, nil, &out, &errOut)
	if len(errs) != 0 {
		t.Fatalf("Execute: unexpected errors: %v", errs)
	}
	if !strings.Contains(out.String(), "ok") {
		t.Errorf("expected 'ok' from echo command, got: %q", out.String())
	}
}

// TestPlan_PreferUnavailable_ManagersMapFallback verifies that when the
// preferred manager is unavailable but a valid entry exists in the managers map
// for a different available manager, it is used.
func TestPlan_PreferUnavailable_ManagersMapFallback(t *testing.T) {
	f := &schema.GenvFile{
		Packages: []schema.Package{
			{
				ID:     "firefox",
				Prefer: "flatpak", // flatpak not available
				Managers: map[string]string{
					"flatpak": "org.mozilla.firefox",
					"brew":    "firefox",
				},
			},
		},
	}
	actions := Plan(f, map[string]bool{"brew": true})
	a := actions[0]
	if !a.Resolved() {
		t.Fatal("expected resolved via managers map fallback")
	}
	if a.Manager != "brew" {
		t.Errorf("manager: got %q, want %q", a.Manager, "brew")
	}
	if a.PkgName != "firefox" {
		t.Errorf("pkgName: got %q, want %q", a.PkgName, "firefox")
	}
}

// ---------------------------------------------------------------------------
// Reconcile regression tests — lock replay and version-constraint behavior
// ---------------------------------------------------------------------------

// TestReconcile_NewPackage_ToInstall verifies that a package in the spec but
// absent from the lock ends up in ToInstall.
func TestReconcile_NewPackage_ToInstall(t *testing.T) {
	desired := []schema.Package{{ID: "git"}}
	managed := []genvfile.LockedPackage{} // empty lock
	result := Reconcile(desired, managed, map[string]bool{"brew": true})
	if len(result.ToInstall) != 1 {
		t.Fatalf("ToInstall: got %d, want 1", len(result.ToInstall))
	}
	if result.ToInstall[0].Pkg.ID != "git" {
		t.Errorf("ToInstall[0].Pkg.ID = %q, want \"git\"", result.ToInstall[0].Pkg.ID)
	}
	if len(result.ToRemove) != 0 || len(result.Unchanged) != 0 {
		t.Errorf("unexpected ToRemove/Unchanged entries")
	}
}

// TestReconcile_RemovedPackage_ToRemove verifies that a package in the lock
// but absent from the spec ends up in ToRemove.
func TestReconcile_RemovedPackage_ToRemove(t *testing.T) {
	desired := []schema.Package{}
	managed := []genvfile.LockedPackage{
		{ID: "htop", Manager: "brew", PkgName: "htop"},
	}
	result := Reconcile(desired, managed, map[string]bool{"brew": true})
	if len(result.ToRemove) != 1 {
		t.Fatalf("ToRemove: got %d, want 1", len(result.ToRemove))
	}
	if result.ToRemove[0].Pkg.ID != "htop" {
		t.Errorf("ToRemove[0].Pkg.ID = %q, want \"htop\"", result.ToRemove[0].Pkg.ID)
	}
}

// TestReconcile_Unchanged_NoVersion verifies that a package in both spec and
// lock with no version constraint stays Unchanged.
func TestReconcile_Unchanged_NoVersion(t *testing.T) {
	desired := []schema.Package{{ID: "git"}}
	managed := []genvfile.LockedPackage{
		{ID: "git", Manager: "brew", PkgName: "git", InstalledVersion: "2.43.0"},
	}
	result := Reconcile(desired, managed, map[string]bool{"brew": true})
	if len(result.Unchanged) != 1 {
		t.Fatalf("Unchanged: got %d, want 1", len(result.Unchanged))
	}
	if len(result.ToInstall) != 0 {
		t.Errorf("unexpected ToInstall: %v", result.ToInstall)
	}
}

// TestReconcile_VersionSatisfied_StaysUnchanged verifies that a lock entry
// whose InstalledVersion satisfies the spec constraint stays Unchanged.
func TestReconcile_VersionSatisfied_StaysUnchanged(t *testing.T) {
	desired := []schema.Package{{ID: "vim", Version: "9.*"}}
	managed := []genvfile.LockedPackage{
		{ID: "vim", Manager: "brew", PkgName: "vim", InstalledVersion: "9.1.0"},
	}
	result := Reconcile(desired, managed, map[string]bool{"brew": true})
	if len(result.Unchanged) != 1 {
		t.Fatalf("Unchanged: got %d, want 1 (version 9.1.0 satisfies 9.*)", len(result.Unchanged))
	}
	if len(result.ToInstall) != 0 {
		t.Errorf("unexpected reinstall queued for satisfying version")
	}
}

// TestReconcile_VersionDrift_MovesToInstall verifies that a lock entry whose
// InstalledVersion does not satisfy the spec constraint is queued for reinstall.
func TestReconcile_VersionDrift_MovesToInstall(t *testing.T) {
	desired := []schema.Package{{ID: "neovim", Version: "0.10.*"}}
	managed := []genvfile.LockedPackage{
		{ID: "neovim", Manager: "brew", PkgName: "neovim", InstalledVersion: "0.9.5"},
	}
	result := Reconcile(desired, managed, map[string]bool{"brew": true})
	if len(result.ToInstall) != 1 {
		t.Fatalf("ToInstall: got %d, want 1 (0.9.5 does not satisfy 0.10.*)", len(result.ToInstall))
	}
	if len(result.Unchanged) != 0 {
		t.Errorf("drifted package must not appear in Unchanged")
	}
}

// TestReconcile_NoInstalledVersion_AlwaysUnchanged verifies backward
// compatibility: old lock entries with empty InstalledVersion are never
// treated as drifted, even when the spec has a version constraint.
func TestReconcile_NoInstalledVersion_AlwaysUnchanged(t *testing.T) {
	desired := []schema.Package{{ID: "git", Version: "2.40.*"}}
	managed := []genvfile.LockedPackage{
		{ID: "git", Manager: "apt", PkgName: "git"}, // InstalledVersion == ""
	}
	result := Reconcile(desired, managed, map[string]bool{"apt": true})
	if len(result.Unchanged) != 1 {
		t.Fatalf("Unchanged: got %d, want 1 (old lock entries must not cause drift)", len(result.Unchanged))
	}
	if len(result.ToInstall) != 0 {
		t.Errorf("old lock entry with empty InstalledVersion must not be queued for reinstall")
	}
}

// TestReconcile_ExactVersionMatch_StaysUnchanged verifies an exact-version
// constraint is satisfied by an identical InstalledVersion.
func TestReconcile_ExactVersionMatch_StaysUnchanged(t *testing.T) {
	desired := []schema.Package{{ID: "ripgrep", Version: "14.1.0"}}
	managed := []genvfile.LockedPackage{
		{ID: "ripgrep", Manager: "brew", PkgName: "ripgrep", InstalledVersion: "14.1.0"},
	}
	result := Reconcile(desired, managed, map[string]bool{"brew": true})
	if len(result.Unchanged) != 1 {
		t.Fatalf("Unchanged: got %d, want 1", len(result.Unchanged))
	}
}

// TestReconcile_ExactVersionMismatch_MovesToInstall verifies that an exact
// constraint with a different installed version is treated as drift.
func TestReconcile_ExactVersionMismatch_MovesToInstall(t *testing.T) {
	desired := []schema.Package{{ID: "ripgrep", Version: "14.1.0"}}
	managed := []genvfile.LockedPackage{
		{ID: "ripgrep", Manager: "brew", PkgName: "ripgrep", InstalledVersion: "13.0.0"},
	}
	result := Reconcile(desired, managed, map[string]bool{"brew": true})
	if len(result.ToInstall) != 1 {
		t.Fatalf("ToInstall: got %d, want 1", len(result.ToInstall))
	}
}
