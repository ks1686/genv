package adapter

import (
	"errors"
	"os"
	"strings"
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

// TestByName verifies that ByName correctly resolves valid adapter names
// and returns nil for unregistered or invalid names.
func TestByName(t *testing.T) {
	// Test valid names from the All registry
	for _, a := range All {
		t.Run("valid_"+a.Name(), func(t *testing.T) {
			got := ByName(a.Name())
			if got == nil {
				t.Fatalf("ByName(%q) returned nil, want non-nil", a.Name())
			}
			if got.Name() != a.Name() {
				t.Errorf("ByName(%q) returned adapter with name %q", a.Name(), got.Name())
			}
		})
	}

	// Test invalid names
	invalidNames := []string{"yum", "chocolatey", "npm", "pip", ""}
	for _, name := range invalidNames {
		t.Run("invalid_"+name, func(t *testing.T) {
			got := ByName(name)
			if got != nil {
				t.Errorf("ByName(%q) expected nil, got %v", name, got)
			}
		})
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
		{"paru", "vim", map[string]string{"paru": "vim-aur"}, "vim-aur", true},
		{"yay", "vim", map[string]string{"yay": "vim-aur"}, "vim-aur", true},
		{"snap", "code", map[string]string{"snap": "code"}, "code", true},
		{"brew", "neovim", map[string]string{"brew": "neovim"}, "neovim", true},
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
// non-empty command slice from PlanInstall and that the package name appears
// as (or is a suffix of) the last argument.
func TestPlanInstall_NonEmpty(t *testing.T) {
	for _, a := range All {
		t.Run(a.Name(), func(t *testing.T) {
			args := a.PlanInstall("git")
			if len(args) == 0 {
				t.Errorf("%s PlanInstall: returned empty slice", a.Name())
				return
			}
			if !strings.HasSuffix(args[len(args)-1], "git") {
				t.Errorf("%s PlanInstall: last arg = %q, want suffix \"git\"", a.Name(), args[len(args)-1])
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
		{"paru", "paru"},
		{"yay", "yay"},
		{"snap", "sudo"},
		{"brew", "brew"},
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
// command slice from PlanUninstall and that the package name appears in the command.
func TestPlanUninstall_NonEmpty(t *testing.T) {
	for _, a := range All {
		t.Run(a.Name(), func(t *testing.T) {
			args := a.PlanUninstall("git")
			if len(args) == 0 {
				t.Errorf("%s PlanUninstall: returned empty slice", a.Name())
				return
			}
			assertContainsArg(t, args, "git")
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
		{"paru", "paru"},
		{"yay", "yay"},
		{"snap", "sudo"},
		{"brew", "brew"},
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

// assertContainsArg fails t if want is not present in args.
func assertContainsArg(t *testing.T, args []string, want string) {
	t.Helper()
	for _, arg := range args {
		if arg == want {
			return
		}
	}
	t.Errorf("expected %q in %v", want, args)
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

// ---------------------------------------------------------------------------
// PlanUpgrade — no tests existed before; every adapter must have valid upgrade
// ---------------------------------------------------------------------------

// TestPlanUpgrade_ExpectedBinaries verifies that each adapter uses the
// expected leading binary for its upgrade command.
func TestPlanUpgrade_ExpectedBinaries(t *testing.T) {
	tests := []struct {
		mgr     string
		wantBin string
	}{
		{"paru", "paru"},
		{"yay", "yay"},
		{"snap", "sudo"},
		{"brew", "brew"},
		{"linuxbrew", "brew"},
	}
	for _, tc := range tests {
		t.Run(tc.mgr, func(t *testing.T) {
			a := ByName(tc.mgr)
			if a == nil {
				t.Fatalf("ByName(%q): no adapter", tc.mgr)
			}
			args := a.PlanUpgrade("pkg")
			if args[0] != tc.wantBin {
				t.Errorf("%s PlanUpgrade: binary = %q, want %q", tc.mgr, args[0], tc.wantBin)
			}
		})
	}
}

// TestPlanUpgrade_PkgNamePresent verifies that the package name appears
// somewhere in every adapter's PlanUpgrade command.
func TestPlanUpgrade_PkgNamePresent(t *testing.T) {
	const pkg = "neovim"
	for _, a := range All {
		t.Run(a.Name(), func(t *testing.T) {
			assertContainsArg(t, a.PlanUpgrade(pkg), pkg)
		})
	}
}

// TestPlanUpgrade_ContainsUpgradeVerb verifies that each adapter uses the
// correct upgrade-action token in its PlanUpgrade command.
func TestPlanUpgrade_ContainsUpgradeVerb(t *testing.T) {
	tests := []struct {
		mgr  string
		verb string
	}{
		{"paru", "-S"},
		{"yay", "-S"},
		{"snap", "refresh"},
		{"brew", "upgrade"},
		{"linuxbrew", "upgrade"},
	}
	for _, tc := range tests {
		t.Run(tc.mgr, func(t *testing.T) {
			a := ByName(tc.mgr)
			if a == nil {
				t.Fatalf("ByName(%q): no adapter", tc.mgr)
			}
			assertContainsArg(t, a.PlanUpgrade("testpkg"), tc.verb)
		})
	}
}

// ---------------------------------------------------------------------------
// PlanClean — content and argument validation (previously only non-empty)
// ---------------------------------------------------------------------------

// TestPlanClean_Snap_ReturnsNil verifies that Snap.PlanClean returns nil
// (snap has no standard cache-clean command).
func TestPlanClean_Snap_ReturnsNil(t *testing.T) {
	cmds := Snap{}.PlanClean()
	if cmds != nil {
		t.Errorf("Snap PlanClean: expected nil, got %v", cmds)
	}
}

// TestPlanClean_CommandCount verifies the exact number of commands each
// adapter returns from PlanClean.
func TestPlanClean_CommandCount(t *testing.T) {
	tests := []struct {
		mgr       string
		wantCount int
	}{
		{"paru", 1},
		{"yay", 1},
		{"snap", 0},
		{"brew", 1},
		{"linuxbrew", 1},
	}
	for _, tc := range tests {
		t.Run(tc.mgr, func(t *testing.T) {
			a := ByName(tc.mgr)
			if a == nil {
				t.Fatalf("ByName(%q): no adapter", tc.mgr)
			}
			cmds := a.PlanClean()
			if got := len(cmds); got != tc.wantCount {
				t.Errorf("%s PlanClean: %d commands, want %d (cmds: %v)", tc.mgr, got, tc.wantCount, cmds)
			}
		})
	}
}

// TestPlanClean_PerAdapterBinary verifies the leading binary of the last
// (main) clean command for each adapter that returns commands.
func TestPlanClean_PerAdapterBinary(t *testing.T) {
	tests := []struct {
		mgr     string
		wantBin string
	}{
		{"paru", "paru"},
		{"yay", "yay"},
		{"brew", "brew"},
		{"linuxbrew", "brew"},
	}
	for _, tc := range tests {
		t.Run(tc.mgr, func(t *testing.T) {
			a := ByName(tc.mgr)
			if a == nil {
				t.Fatalf("ByName(%q): no adapter", tc.mgr)
			}
			cmds := a.PlanClean()
			if len(cmds) == 0 {
				t.Fatalf("%s PlanClean: no commands returned", tc.mgr)
			}
			last := cmds[len(cmds)-1]
			if last[0] != tc.wantBin {
				t.Errorf("%s PlanClean last cmd[0] = %q, want %q", tc.mgr, last[0], tc.wantBin)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// PlanInstall — verb and noninteractive flag validation
// ---------------------------------------------------------------------------

// TestPlanInstall_ContainsInstallVerb verifies that each adapter's PlanInstall
// contains the expected install-action token.
func TestPlanInstall_ContainsInstallVerb(t *testing.T) {
	tests := []struct {
		mgr  string
		verb string
	}{
		{"paru", "-S"},
		{"yay", "-S"},
		{"snap", "install"},
		{"brew", "install"},
		{"linuxbrew", "install"},
	}
	for _, tc := range tests {
		t.Run(tc.mgr, func(t *testing.T) {
			a := ByName(tc.mgr)
			if a == nil {
				t.Fatalf("ByName(%q): no adapter", tc.mgr)
			}
			assertContainsArg(t, a.PlanInstall("testpkg"), tc.verb)
		})
	}
}

// TestPlanInstall_ContainsNoninteractiveFlag verifies that adapters which
// require a non-interactive flag include it in PlanInstall.
func TestPlanInstall_ContainsNoninteractiveFlag(t *testing.T) {
	tests := []struct {
		mgr      string
		wantFlag string
	}{
		{"paru", "--noconfirm"},
		{"yay", "--noconfirm"},
	}
	for _, tc := range tests {
		t.Run(tc.mgr, func(t *testing.T) {
			a := ByName(tc.mgr)
			if a == nil {
				t.Fatalf("ByName(%q): no adapter", tc.mgr)
			}
			assertContainsArg(t, a.PlanInstall("testpkg"), tc.wantFlag)
		})
	}
}

// ---------------------------------------------------------------------------
// PlanUninstall — verb and noninteractive flag validation
// ---------------------------------------------------------------------------

// TestPlanUninstall_ContainsRemoveVerb verifies that each adapter's
// PlanUninstall contains the expected remove-action token.
func TestPlanUninstall_ContainsRemoveVerb(t *testing.T) {
	tests := []struct {
		mgr  string
		verb string
	}{
		{"paru", "-Rns"},
		{"yay", "-Rns"},
		{"snap", "remove"},
		{"brew", "uninstall"},
		{"linuxbrew", "uninstall"},
	}
	for _, tc := range tests {
		t.Run(tc.mgr, func(t *testing.T) {
			a := ByName(tc.mgr)
			if a == nil {
				t.Fatalf("ByName(%q): no adapter", tc.mgr)
			}
			assertContainsArg(t, a.PlanUninstall("testpkg"), tc.verb)
		})
	}
}

// TestPlanUninstall_ContainsNoninteractiveFlag verifies that adapters which
// require a non-interactive flag include it in PlanUninstall.
func TestPlanUninstall_ContainsNoninteractiveFlag(t *testing.T) {
	tests := []struct {
		mgr      string
		wantFlag string
	}{
		{"paru", "--noconfirm"},
		{"yay", "--noconfirm"},
	}
	for _, tc := range tests {
		t.Run(tc.mgr, func(t *testing.T) {
			a := ByName(tc.mgr)
			if a == nil {
				t.Fatalf("ByName(%q): no adapter", tc.mgr)
			}
			assertContainsArg(t, a.PlanUninstall("testpkg"), tc.wantFlag)
		})
	}
}

// ---------------------------------------------------------------------------
// parsePacmanSearch — pure function, previously untested
// ---------------------------------------------------------------------------

func TestParsePacmanSearch_BasicMatch(t *testing.T) {
	lines := []string{
		"extra/vim 9.0-1 [installed]",
		"    Vi IMproved text editor",
		"extra/vim-minimal 9.0-1",
		"    Minimal vim installation",
	}
	got := parsePacmanSearch(lines, "vim")
	want := []string{"vim", "vim-minimal"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("[%d]: got %q, want %q", i, got[i], w)
		}
	}
}

func TestParsePacmanSearch_CaseInsensitive(t *testing.T) {
	lines := []string{
		"extra/VIM 9.0-1",
		"    Vi IMproved",
	}
	got := parsePacmanSearch(lines, "vim")
	if len(got) != 1 || got[0] != "VIM" {
		t.Errorf("case insensitive: got %v, want [VIM]", got)
	}
}

func TestParsePacmanSearch_SkipsDescriptionLines(t *testing.T) {
	// Indented lines (descriptions) must never be returned even if they
	// contain the query string.
	lines := []string{
		"    vim is a great editor with vim-like bindings",
		"\tvim-mode description line",
	}
	got := parsePacmanSearch(lines, "vim")
	if len(got) != 0 {
		t.Errorf("description lines must be skipped, got %v", got)
	}
}

func TestParsePacmanSearch_NoMatch(t *testing.T) {
	lines := []string{
		"extra/htop 3.2.0-1",
		"    Process viewer",
	}
	got := parsePacmanSearch(lines, "vim")
	if len(got) != 0 {
		t.Errorf("expected 0 matches, got %v", got)
	}
}

func TestParsePacmanSearch_NoSlashInPackageLine(t *testing.T) {
	// Package lines without "repo/name" format must be skipped.
	lines := []string{
		"vim 9.0-1",
	}
	got := parsePacmanSearch(lines, "vim")
	if len(got) != 0 {
		t.Errorf("line without repo/ prefix must be skipped, got %v", got)
	}
}

func TestParsePacmanSearch_EmptyInput(t *testing.T) {
	got := parsePacmanSearch(nil, "vim")
	if len(got) != 0 {
		t.Errorf("nil input: expected empty result, got %v", got)
	}
}

// ---------------------------------------------------------------------------
// Search output parsers — fake binaries via PATH injection
// ---------------------------------------------------------------------------

// TestBrewSearch_FiltersArrowHeaders verifies that brew's "==> Formulae" and
// "==> Casks" section headers are never returned in results.
func TestBrewSearch_FiltersArrowHeaders(t *testing.T) {
	installFakeBinary(t, "brew",
		`if [ "$1" = "search" ]; then
  echo "==> Formulae"
  echo "neovim"
  echo "vim"
  echo "==> Casks"
  echo "macvim"
fi`)
	names, err := Brew{}.Search("vim")
	if err != nil {
		t.Fatalf("Brew.Search: %v", err)
	}
	for _, n := range names {
		if n == "==> Formulae" || n == "==> Casks" {
			t.Errorf("section header %q must not appear in results", n)
		}
	}
	wantSet := map[string]bool{"neovim": true, "vim": true, "macvim": true}
	for _, n := range names {
		if !wantSet[n] {
			t.Errorf("unexpected name %q in results %v", n, names)
		}
	}
	if len(names) != 3 {
		t.Errorf("expected 3 results, got %d: %v", len(names), names)
	}
}

// TestSnapSearch_SkipsHeaderLine verifies that Snap.Search skips the first
// (header) line of "snap find" output and returns only package names.
func TestSnapSearch_SkipsHeaderLine(t *testing.T) {
	installFakeBinary(t, "snap",
		`if [ "$1" = "find" ]; then
  echo "Name  Version  Publisher  Notes  Summary"
  echo "vim  9.0  canonical  -  Vi IMproved editor"
  echo "vim-enhanced  9.0  canonical  -  Enhanced vim"
fi`)
	names, err := Snap{}.Search("vim")
	if err != nil {
		t.Fatalf("Snap.Search: %v", err)
	}
	for _, n := range names {
		if n == "Name" {
			t.Error("Snap.Search: header 'Name' column must not appear in results")
		}
	}
	if len(names) != 2 {
		t.Fatalf("expected 2 results (header skipped), got %d: %v", len(names), names)
	}
	if names[0] != "vim" || names[1] != "vim-enhanced" {
		t.Errorf("expected [vim vim-enhanced], got %v", names)
	}
}

// ---------------------------------------------------------------------------
// runListOutput / runVersionOutput — missing-binary and whitespace edge cases
// ---------------------------------------------------------------------------

// TestRunListOutput_MissingBinary verifies that runListOutput returns an error
// (not nil, nil) when the binary does not exist.
func TestRunListOutput_MissingBinary(t *testing.T) {
	lines, err := runListOutput("__genv_nonexistent_binary__")
	if err == nil {
		t.Error("runListOutput with missing binary: expected error, got nil")
	}
	if lines != nil {
		t.Errorf("runListOutput with missing binary: expected nil lines, got %v", lines)
	}
}

// TestRunListOutput_WhitespaceOnlyLinesSkipped verifies that lines containing
// only whitespace are excluded from the returned slice.
func TestRunListOutput_WhitespaceOnlyLinesSkipped(t *testing.T) {
	lines, err := runListOutput("printf", "foo\n   \nbar\n\n")
	if err != nil {
		t.Fatalf("runListOutput: %v", err)
	}
	for _, line := range lines {
		if line == "" {
			t.Errorf("empty line appeared in results")
		}
	}
	if len(lines) != 2 {
		t.Errorf("expected 2 non-blank lines, got %d: %v", len(lines), lines)
	}
}

// TestRunVersionOutput_MissingBinary verifies that runVersionOutput returns
// ("", error) when the binary does not exist, not ("", nil).
func TestRunVersionOutput_MissingBinary(t *testing.T) {
	v, err := runVersionOutput("__genv_nonexistent_binary__")
	if err == nil {
		t.Error("runVersionOutput with missing binary: expected error, got nil")
	}
	if v != "" {
		t.Errorf("runVersionOutput with missing binary: expected empty string, got %q", v)
	}
}
