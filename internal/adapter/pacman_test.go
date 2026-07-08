package adapter

import (
	"os"
	"strings"
	"testing"
)

func TestPacman_Name(t *testing.T) {
	if got := (Pacman{}).Name(); got != "pacman" {
		t.Errorf("Name() = %q, want %q", got, "pacman")
	}
}

func TestPacman_PlanInstall(t *testing.T) {
	args := Pacman{}.PlanInstall("git")
	want := []string{"sudo", "pacman", "-S", "--needed", "--noconfirm", "git"}
	if len(args) != len(want) {
		t.Fatalf("PlanInstall: got %v, want %v", args, want)
	}
	for i, w := range want {
		if args[i] != w {
			t.Errorf("PlanInstall[%d] = %q, want %q", i, args[i], w)
		}
	}
}

func TestPacman_PlanUninstall(t *testing.T) {
	args := Pacman{}.PlanUninstall("git")
	want := []string{"sudo", "pacman", "-Rcs", "--noconfirm", "git"}
	if len(args) != len(want) {
		t.Fatalf("PlanUninstall: got %v, want %v", args, want)
	}
	for i, w := range want {
		if args[i] != w {
			t.Errorf("PlanUninstall[%d] = %q, want %q", i, args[i], w)
		}
	}
}

func TestPacman_PlanUpgrade(t *testing.T) {
	args := Pacman{}.PlanUpgrade("git")
	want := []string{"sudo", "pacman", "-S", "--needed", "--noconfirm", "git"}
	if len(args) != len(want) {
		t.Fatalf("PlanUpgrade: got %v, want %v", args, want)
	}
	for i, w := range want {
		if args[i] != w {
			t.Errorf("PlanUpgrade[%d] = %q, want %q", i, args[i], w)
		}
	}
}

func TestPacman_PlanUpgradeBatch(t *testing.T) {
	args := Pacman{}.PlanUpgradeBatch([]string{"git", "neovim"})
	want := []string{"sudo", "pacman", "-S", "--needed", "--noconfirm", "git", "neovim"}
	if len(args) != len(want) {
		t.Fatalf("PlanUpgradeBatch: got %v, want %v", args, want)
	}
	for i, w := range want {
		if args[i] != w {
			t.Errorf("PlanUpgradeBatch[%d] = %q, want %q", i, args[i], w)
		}
	}
}

func TestPacman_ListInstalledVersions(t *testing.T) {
	installFakeBinary(t, "pacman",
		`if [ "$1" = "-Q" ]; then
	echo "bash 5.2.37-1"
	echo "git 2.45.0-1"
fi`)
	versions, err := Pacman{}.ListInstalledVersions()
	if err != nil {
		t.Fatalf("ListInstalledVersions: %v", err)
	}
	want := map[string]string{"bash": "5.2.37-1", "git": "2.45.0-1"}
	if len(versions) != len(want) {
		t.Fatalf("ListInstalledVersions: got %v, want %v", versions, want)
	}
	for pkg, ver := range want {
		if versions[pkg] != ver {
			t.Errorf("ListInstalledVersions[%q] = %q, want %q", pkg, versions[pkg], ver)
		}
	}
}

func TestPacman_PlanClean(t *testing.T) {
	cmds := Pacman{}.PlanClean()
	if len(cmds) != 1 {
		t.Fatalf("PlanClean: expected 1 command, got %v", cmds)
	}
	want := []string{"sudo", "pacman", "-Sc", "--noconfirm"}
	if len(cmds[0]) != len(want) {
		t.Fatalf("PlanClean[0]: got %v, want %v", cmds[0], want)
	}
	for i, w := range want {
		if cmds[0][i] != w {
			t.Errorf("PlanClean[0][%d] = %q, want %q", i, cmds[0][i], w)
		}
	}
}

func TestPacman_Query_NotInstalled(t *testing.T) {
	installFakeBinary(t, "pacman",
		`if [ "$1" = "-Qi" ] && [ "$2" = "__genv_missing__" ]; then
	exit 1
fi`)
	ok, err := Pacman{}.Query("__genv_missing__")
	if err != nil {
		t.Fatalf("Query: unexpected error: %v", err)
	}
	if ok {
		t.Error("Query: expected false for missing package")
	}
}

func TestPacman_Query_Installed(t *testing.T) {
	installFakeBinary(t, "pacman",
		`if [ "$1" = "-Qi" ] && [ "$2" = "bash" ]; then
	exit 0
fi`)
	ok, err := Pacman{}.Query("bash")
	if err != nil {
		t.Fatalf("Query: unexpected error: %v", err)
	}
	if !ok {
		t.Error("Query: expected true for installed package")
	}
}

func TestPacman_ListInstalled(t *testing.T) {
	installFakeBinary(t, "pacman",
		`if [ "$1" = "-Qq" ]; then
	echo "bash"
	echo "git"
	echo "pacman"
fi`)
	pkgs, err := Pacman{}.ListInstalled()
	if err != nil {
		t.Fatalf("ListInstalled: %v", err)
	}
	want := []string{"bash", "git", "pacman"}
	if len(pkgs) != len(want) {
		t.Fatalf("ListInstalled: got %v, want %v", pkgs, want)
	}
	for i, w := range want {
		if pkgs[i] != w {
			t.Errorf("ListInstalled[%d] = %q, want %q", i, pkgs[i], w)
		}
	}
}

func TestPacman_QueryVersion(t *testing.T) {
	installFakeBinary(t, "pacman",
		`if [ "$1" = "-Q" ] && [ "$2" = "bash" ]; then
	echo "bash 5.2.37-1"
fi`)
	ver, err := Pacman{}.QueryVersion("bash")
	if err != nil {
		t.Fatalf("QueryVersion: %v", err)
	}
	if ver != "5.2.37-1" {
		t.Errorf("QueryVersion: got %q, want %q", ver, "5.2.37-1")
	}
}

func TestPacman_QueryVersion_NotInstalled(t *testing.T) {
	installFakeBinary(t, "pacman",
		`if [ "$1" = "-Q" ] && [ "$2" = "__genv_missing__" ]; then
	exit 1
fi`)
	ver, err := Pacman{}.QueryVersion("__genv_missing__")
	if err != nil {
		t.Fatalf("QueryVersion: unexpected error: %v", err)
	}
	if ver != "" {
		t.Errorf("QueryVersion: expected empty string, got %q", ver)
	}
}

func TestPacman_Search(t *testing.T) {
	installFakeBinary(t, "pacman",
		`if [ "$1" = "-Ss" ] && [ "$2" = "vim" ]; then
	echo "extra/vim 9.0-1"
	echo "    Vi IMproved text editor"
	echo "extra/vim-minimal 9.0-1"
	echo "    Minimal vim installation"
fi`)
	names, err := Pacman{}.Search("vim")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	want := []string{"vim", "vim-minimal"}
	if len(names) != len(want) {
		t.Fatalf("Search: got %v, want %v", names, want)
	}
	for i, w := range want {
		if names[i] != w {
			t.Errorf("Search[%d] = %q, want %q", i, names[i], w)
		}
	}
}

func TestPacman_Available(t *testing.T) {
	orig := lookPath
	t.Cleanup(func() { lookPath = orig })

	lookPath = func(name string) (string, error) {
		if name == "pacman" {
			return "/usr/bin/pacman", nil
		}
		return "", &os.PathError{Op: "lookpath", Err: os.ErrNotExist}
	}
	if !(Pacman{}).Available() {
		t.Error("Available() = false when lookPath finds pacman")
	}

	lookPath = func(string) (string, error) {
		return "", &os.PathError{Op: "lookpath", Err: os.ErrNotExist}
	}
	if (Pacman{}).Available() {
		t.Error("Available() = true when lookPath fails")
	}
}

// ---------------------------------------------------------------------------
// Integration tests — run only when pacman is present on the host.
// ---------------------------------------------------------------------------

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
	if !strings.Contains(ver, ".") {
		t.Errorf("Pacman.QueryVersion(bash): expected dotted version, got %q", ver)
	}
}
