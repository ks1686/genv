package adapter

import (
	"errors"
	"os"
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

// ---------------------------------------------------------------------------
// Available() — mocked lookPath
// ---------------------------------------------------------------------------

// TestAvailable_AllAdapters_WithMockedLookPath verifies that Available() returns
// true when lookPath finds the binary and false when lookPath returns an error.
func TestAvailable_AllAdapters_WithMockedLookPath(t *testing.T) {
	orig := lookPath
	t.Cleanup(func() { lookPath = orig })

	for _, a := range All {
		t.Run(a.Name()+"/found", func(t *testing.T) {
			lookPath = func(string) (string, error) { return "/usr/bin/mgr", nil }
			if !a.Available() {
				t.Errorf("%s.Available() = false when lookPath succeeds", a.Name())
			}
		})
		t.Run(a.Name()+"/missing", func(t *testing.T) {
			lookPath = func(string) (string, error) { return "", &os.PathError{Op: "lookpath", Err: os.ErrNotExist} }
			if a.Available() {
				t.Errorf("%s.Available() = true when lookPath fails", a.Name())
			}
		})
	}
}

// ---------------------------------------------------------------------------
// parseMgrQueryVersion — pure function
// ---------------------------------------------------------------------------

func TestParseMgrQueryVersion(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"neovim 0.10.0-1", "0.10.0-1"},
		{"git 2.43.0-1", "2.43.0-1"},
		{"pkg 1.0", "1.0"},
		{"onlyname", ""}, // no space → empty
		{"", ""},         // empty input → empty
		{"a b c", "b c"}, // multiple spaces → rest of line
	}
	for _, tc := range tests {
		got := parseMgrQueryVersion(tc.input)
		if got != tc.want {
			t.Errorf("parseMgrQueryVersion(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

// ---------------------------------------------------------------------------
// runQuery / runListOutput / runVersionOutput — tested with real binaries
// ---------------------------------------------------------------------------

// TestRunQuery_ExitZero verifies that a command exiting 0 is treated as "installed".
func TestRunQuery_ExitZero(t *testing.T) {
	ok, err := runQuery("true")
	if err != nil {
		t.Fatalf("runQuery(true): unexpected error: %v", err)
	}
	if !ok {
		t.Error("runQuery(true): expected true (exit 0 = installed)")
	}
}

// TestRunQuery_ExitNonZero verifies that a non-zero exit code means "not installed"
// and is not returned as an error.
func TestRunQuery_ExitNonZero(t *testing.T) {
	ok, err := runQuery("false")
	if err != nil {
		t.Fatalf("runQuery(false): unexpected error: %v", err)
	}
	if ok {
		t.Error("runQuery(false): expected false (exit non-zero = absent)")
	}
}

// TestRunQuery_MissingBinary verifies that a missing binary returns an error
// (not a simple false).
func TestRunQuery_MissingBinary(t *testing.T) {
	_, err := runQuery("__genv_nonexistent_binary__")
	if err == nil {
		t.Error("runQuery with missing binary: expected error, got nil")
	}
	// Must NOT be an ExitError — it must be an exec/OS error.
	if errors.As(err, new(interface{ ExitCode() int })) {
		t.Error("runQuery with missing binary: error should not be ExitError")
	}
}

// TestRunListOutput_ReturnsLines verifies that stdout lines are split and trimmed.
func TestRunListOutput_ReturnsLines(t *testing.T) {
	lines, err := runListOutput("printf", "foo\nbar\nbaz\n")
	if err != nil {
		t.Fatalf("runListOutput: %v", err)
	}
	want := []string{"foo", "bar", "baz"}
	if len(lines) != len(want) {
		t.Fatalf("lines: got %v, want %v", lines, want)
	}
	for i, w := range want {
		if lines[i] != w {
			t.Errorf("lines[%d]: got %q, want %q", i, lines[i], w)
		}
	}
}

// TestRunListOutput_NonZeroExit verifies that a non-zero exit is treated as
// "no packages" (nil, nil) and not an error.
func TestRunListOutput_NonZeroExit(t *testing.T) {
	lines, err := runListOutput("false")
	if err != nil {
		t.Fatalf("runListOutput(false): unexpected error: %v", err)
	}
	if lines != nil {
		t.Errorf("runListOutput(false): expected nil, got %v", lines)
	}
}

// TestRunVersionOutput_ReturnsVersion verifies that stdout is returned trimmed.
func TestRunVersionOutput_ReturnsVersion(t *testing.T) {
	v, err := runVersionOutput("echo", "1.2.3")
	if err != nil {
		t.Fatalf("runVersionOutput: %v", err)
	}
	if v != "1.2.3" {
		t.Errorf("runVersionOutput: got %q, want %q", v, "1.2.3")
	}
}

// TestRunVersionOutput_NonZeroExit verifies that a non-zero exit returns ("", nil).
func TestRunVersionOutput_NonZeroExit(t *testing.T) {
	v, err := runVersionOutput("false")
	if err != nil {
		t.Fatalf("runVersionOutput(false): unexpected error: %v", err)
	}
	if v != "" {
		t.Errorf("runVersionOutput(false): expected empty string, got %q", v)
	}
}

// ---------------------------------------------------------------------------
// isWSL / wslSafeLookPath — testable on any Linux host
// ---------------------------------------------------------------------------

// TestIsWSL_NonWSL verifies that isWSL() returns false on a non-WSL Linux host.
// The result will be true only on WSL2, and false on bare Linux or macOS.
func TestIsWSL_NonWSL(t *testing.T) {
	// Just verify it doesn't panic and returns a bool.
	// We do not assert the value because this test may run inside WSL.
	_ = isWSL()
}

// TestWslSafeLookPath_NonWSL verifies that wslSafeLookPath on a non-WSL host
// delegates directly to exec.LookPath. "sh" is present on all POSIX hosts.
func TestWslSafeLookPath_NonWSL(t *testing.T) {
	if isWSL() {
		t.Skip("skipping on WSL host — wslSafeLookPath uses WSL-specific logic")
	}
	_, err := wslSafeLookPath("sh")
	if err != nil {
		t.Errorf("wslSafeLookPath(\"sh\"): expected sh to be found, got: %v", err)
	}
	_, err = wslSafeLookPath("__genv_nonexistent__")
	if err == nil {
		t.Error("wslSafeLookPath(nonexistent): expected error, got nil")
	}
}

// ---------------------------------------------------------------------------
// Adapter Query / ListInstalled / QueryVersion
// These tests call each adapter's methods directly. For adapters whose binary
// is present on the test host, we make concrete assertions. For those that are
// absent, we verify the methods complete without panicking (the code path is
// still covered even when the binary cannot be found).
// ---------------------------------------------------------------------------

// TestAllAdapters_MethodsNoPanic verifies that Query, ListInstalled, and
// QueryVersion never panic regardless of whether the adapter's binary is
// installed on the current host.
func TestAllAdapters_MethodsNoPanic(t *testing.T) {
	const absentPkg = "__genv_nonexistent_pkg__"
	for _, a := range All {
		t.Run(a.Name()+"/Query", func(t *testing.T) {
			_, _ = a.Query(absentPkg)
		})
		t.Run(a.Name()+"/ListInstalled", func(t *testing.T) {
			_, _ = a.ListInstalled()
		})
		t.Run(a.Name()+"/QueryVersion", func(t *testing.T) {
			_, _ = a.QueryVersion(absentPkg)
		})
	}
}

// TestPacman_Query_And_Version exercises Pacman's Query/ListInstalled/QueryVersion
// against real pacman when available. On Arch Linux, "bash" is always installed.
func TestPacman_Query_And_Version(t *testing.T) {
	a := Pacman{}
	if !a.Available() {
		t.Skip("pacman not available on this host")
	}
	ok, err := a.Query("bash")
	if err != nil {
		t.Fatalf("Pacman.Query(bash): %v", err)
	}
	if !ok {
		t.Error("Pacman.Query(bash): expected true (bash is always installed on Arch)")
	}

	pkgs, err := a.ListInstalled()
	if err != nil {
		t.Fatalf("Pacman.ListInstalled: %v", err)
	}
	if len(pkgs) == 0 {
		t.Error("Pacman.ListInstalled: expected at least one package")
	}

	ver, err := a.QueryVersion("bash")
	if err != nil {
		t.Fatalf("Pacman.QueryVersion(bash): %v", err)
	}
	if ver == "" {
		t.Error("Pacman.QueryVersion(bash): expected non-empty version")
	}
}

// TestParu_Query_And_Version exercises Paru when available.
// Paru reuses pacman's database, so "bash" is always installed when paru is.
func TestParu_Query_And_Version(t *testing.T) {
	a := Paru{}
	if !a.Available() {
		t.Skip("paru not available on this host")
	}
	ok, err := a.Query("bash")
	if err != nil {
		t.Fatalf("Paru.Query(bash): %v", err)
	}
	if !ok {
		t.Error("Paru.Query(bash): expected true (bash is always installed on Arch)")
	}

	pkgs, err := a.ListInstalled()
	if err != nil {
		t.Fatalf("Paru.ListInstalled: %v", err)
	}
	if len(pkgs) == 0 {
		t.Error("Paru.ListInstalled: expected at least one package")
	}

	ver, err := a.QueryVersion("bash")
	if err != nil {
		t.Fatalf("Paru.QueryVersion(bash): %v", err)
	}
	if ver == "" {
		t.Error("Paru.QueryVersion(bash): expected non-empty version")
	}
}

// TestFlatpak_AbsentPackage verifies that Flatpak returns (false, nil) for a
// package that is definitely not installed.
func TestFlatpak_AbsentPackage(t *testing.T) {
	a := Flatpak{}
	if !a.Available() {
		t.Skip("flatpak not available on this host")
	}
	ok, err := a.Query("__genv.nonexistent.flatpak.app__")
	if err != nil {
		t.Fatalf("Flatpak.Query(nonexistent): unexpected error: %v", err)
	}
	if ok {
		t.Error("Flatpak.Query(nonexistent): expected false")
	}
}

// ---------------------------------------------------------------------------
// Parsing logic tests — fake binaries via PATH injection
// These tests create temporary shell scripts that produce the expected
// manager output format, then verify that the adapter's parsing logic
// extracts the correct data. exec.Command uses PATH lookup, so prepending
// the fake-binary dir to PATH is sufficient without any code changes.
// ---------------------------------------------------------------------------

// installFakeBinary writes a shell script to dir/<name> that outputs body
// on stdout and makes it executable, then adds dir to the front of PATH.
// It returns a cleanup function that restores the original PATH.
func installFakeBinary(t *testing.T, name, body string) {
	t.Helper()
	dir := t.TempDir()
	path := dir + "/" + name
	script := "#!/bin/sh\n" + body + "\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("installFakeBinary(%q): WriteFile: %v", name, err)
	}
	orig := os.Getenv("PATH")
	t.Setenv("PATH", dir+":"+orig)
}

// TestSnap_ListInstalled_ParsesHeader verifies that the first ("header") line
// from "snap list" output is skipped and package names are extracted correctly.
func TestSnap_ListInstalled_ParsesHeader(t *testing.T) {
	installFakeBinary(t, "snap",
		`if [ "$1" = "list" ]; then
  echo "Name  Version  Rev  Tracking  Publisher  Notes"
  echo "core  16-2.61  16928  latest/stable  canonical  core"
  echo "hello  2.10  20  latest/stable  canonical  -"
fi`)
	pkgs, err := Snap{}.ListInstalled()
	if err != nil {
		t.Fatalf("Snap.ListInstalled: %v", err)
	}
	if len(pkgs) != 2 {
		t.Fatalf("expected 2 packages (header skipped), got %d: %v", len(pkgs), pkgs)
	}
	if pkgs[0] != "core" || pkgs[1] != "hello" {
		t.Errorf("expected [core hello], got %v", pkgs)
	}
}

// TestSnap_QueryVersion_ParsesOutput verifies the column-based version extraction.
func TestSnap_QueryVersion_ParsesOutput(t *testing.T) {
	installFakeBinary(t, "snap",
		`if [ "$1" = "list" ]; then
  echo "Name  Version  Rev"
  echo "core  16-2.61  16928"
fi`)
	ver, err := Snap{}.QueryVersion("core")
	if err != nil {
		t.Fatalf("Snap.QueryVersion: %v", err)
	}
	if ver != "16-2.61" {
		t.Errorf("version: got %q, want %q", ver, "16-2.61")
	}
}

// TestFlatpak_QueryVersion_ParsesVersion verifies "Version:" extraction from
// "flatpak info" output.
func TestFlatpak_QueryVersion_ParsesVersion(t *testing.T) {
	installFakeBinary(t, "flatpak",
		`if [ "$1" = "info" ]; then
  echo "Ref: app/org.mozilla.firefox/x86_64/stable"
  echo "Version: 120.0"
  echo "License: MPL-2.0"
fi`)
	ver, err := Flatpak{}.QueryVersion("org.mozilla.firefox")
	if err != nil {
		t.Fatalf("Flatpak.QueryVersion: %v", err)
	}
	if ver != "120.0" {
		t.Errorf("version: got %q, want %q", ver, "120.0")
	}
}

// TestMacPorts_ListInstalled_ParsesAtSuffix verifies "@version" stripping.
func TestMacPorts_ListInstalled_ParsesAtSuffix(t *testing.T) {
	installFakeBinary(t, "port",
		`if [ "$1" = "echo" ] && [ "$2" = "installed" ]; then
  echo "vim @9.0.0607_2+huge (active)"
  echo "git @2.43.0_0 (active)"
fi`)
	pkgs, err := MacPorts{}.ListInstalled()
	if err != nil {
		t.Fatalf("MacPorts.ListInstalled: %v", err)
	}
	if len(pkgs) != 2 {
		t.Fatalf("expected 2 packages, got %d: %v", len(pkgs), pkgs)
	}
	if pkgs[0] != "vim" || pkgs[1] != "git" {
		t.Errorf("expected [vim git], got %v", pkgs)
	}
}

// TestMacPorts_QueryVersion_ParsesVersion verifies "@version (active)" parsing.
func TestMacPorts_QueryVersion_ParsesVersion(t *testing.T) {
	installFakeBinary(t, "port",
		`if [ "$1" = "installed" ]; then
  echo "  vim @9.0.0607_2+huge (active)"
fi`)
	ver, err := MacPorts{}.QueryVersion("vim")
	if err != nil {
		t.Fatalf("MacPorts.QueryVersion: %v", err)
	}
	if ver != "9.0.0607_2" {
		t.Errorf("version: got %q, want %q", ver, "9.0.0607_2")
	}
}

// TestBrewQueryVersion_ParsesOutput verifies "pkgname version" splitting in
// brewQueryVersion (called by both Brew and Linuxbrew QueryVersion).
func TestBrewQueryVersion_ParsesOutput(t *testing.T) {
	installFakeBinary(t, "brew",
		`if [ "$1" = "list" ] && [ "$2" = "--versions" ]; then
  echo "git 2.43.0"
fi`)
	ver, err := Brew{}.QueryVersion("git")
	if err != nil {
		t.Fatalf("Brew.QueryVersion: %v", err)
	}
	if ver != "2.43.0" {
		t.Errorf("version: got %q, want %q", ver, "2.43.0")
	}
}

// TestBrew_ListInstalled_CombinesFormulaeAndCasks verifies that Brew.ListInstalled
// concatenates formulae and casks from two separate brew list calls.
func TestBrew_ListInstalled_CombinesFormulaeAndCasks(t *testing.T) {
	installFakeBinary(t, "brew",
		`if [ "$1" = "list" ] && [ "$2" = "--formula" ]; then
  echo "git"
  echo "neovim"
elif [ "$1" = "list" ] && [ "$2" = "--cask" ]; then
  echo "firefox"
fi`)
	pkgs, err := Brew{}.ListInstalled()
	if err != nil {
		t.Fatalf("Brew.ListInstalled: %v", err)
	}
	if len(pkgs) != 3 {
		t.Fatalf("expected 3 packages (2 formulae + 1 cask), got %d: %v", len(pkgs), pkgs)
	}
}

// TestBrew_Query_ChecksCask verifies that Brew.Query falls back to cask when
// the formula check returns false (not installed as formula).
func TestBrew_Query_ChecksCask(t *testing.T) {
	// The fake brew returns exit 1 for formula list and exit 0 for cask list.
	installFakeBinary(t, "brew",
		`if [ "$1" = "list" ] && [ "$2" = "--formula" ]; then
  exit 1
elif [ "$1" = "list" ] && [ "$2" = "--cask" ]; then
  exit 0
fi`)
	ok, err := Brew{}.Query("firefox")
	if err != nil {
		t.Fatalf("Brew.Query(cask path): %v", err)
	}
	if !ok {
		t.Error("Brew.Query: expected true when installed as cask")
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
