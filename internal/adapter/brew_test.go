package adapter

import (
	"os"
	"testing"
)

func TestBrew_Name(t *testing.T) {
	if got := (Brew{}).Name(); got != "brew" {
		t.Errorf("Name() = %q, want %q", got, "brew")
	}
}

func TestLinuxbrew_Name(t *testing.T) {
	if got := (Linuxbrew{}).Name(); got != "linuxbrew" {
		t.Errorf("Name() = %q, want %q", got, "linuxbrew")
	}
}

func TestBrew_PlanInstall(t *testing.T) {
	args := Brew{}.PlanInstall("git")
	want := []string{"brew", "install", "git"}
	if len(args) != len(want) {
		t.Fatalf("PlanInstall: got %v, want %v", args, want)
	}
	for i, w := range want {
		if args[i] != w {
			t.Errorf("PlanInstall[%d] = %q, want %q", i, args[i], w)
		}
	}
}

func TestBrew_PlanUninstall(t *testing.T) {
	args := Brew{}.PlanUninstall("git")
	want := []string{"brew", "uninstall", "git"}
	if len(args) != len(want) {
		t.Fatalf("PlanUninstall: got %v, want %v", args, want)
	}
	for i, w := range want {
		if args[i] != w {
			t.Errorf("PlanUninstall[%d] = %q, want %q", i, args[i], w)
		}
	}
}

func TestBrew_PlanClean(t *testing.T) {
	cmds := Brew{}.PlanClean()
	if len(cmds) != 1 {
		t.Fatalf("PlanClean: expected 1 command, got %v", cmds)
	}
	want := []string{"brew", "cleanup"}
	if len(cmds[0]) != len(want) {
		t.Fatalf("PlanClean[0]: got %v, want %v", cmds[0], want)
	}
	for i, w := range want {
		if cmds[0][i] != w {
			t.Errorf("PlanClean[0][%d] = %q, want %q", i, cmds[0][i], w)
		}
	}
}

// TestBrew_ListInstalled_UsesSingleDashOne is a regression test for a bug
// where the adapter passed "--1" (an unrecognized flag that makes brew print
// its usage banner and exit 0) instead of "-1" (one-per-line output). With
// the wrong flag, ListInstalled silently returned zero packages on every
// real machine, which broke "genv scan" for every brew/linuxbrew user.
func TestBrew_ListInstalled_UsesSingleDashOne(t *testing.T) {
	installFakeBinary(t, "brew",
		`if [ "$1" = "list" ] && [ "$2" = "--formula" ] && [ "$3" = "-1" ]; then
	echo "formula-a"
	echo "formula-b"
	exit 0
fi
if [ "$1" = "list" ] && [ "$2" = "--cask" ] && [ "$3" = "-1" ]; then
	echo "cask-a"
	exit 0
fi
echo "unexpected args: $*" >&2
exit 1`)

	pkgs, err := Brew{}.ListInstalled()
	if err != nil {
		t.Fatalf("ListInstalled: unexpected error: %v", err)
	}
	want := []string{"formula-a", "formula-b", "cask-a"}
	if len(pkgs) != len(want) {
		t.Fatalf("ListInstalled: got %v, want %v", pkgs, want)
	}
	for i, w := range want {
		if pkgs[i] != w {
			t.Errorf("ListInstalled[%d] = %q, want %q", i, pkgs[i], w)
		}
	}
}

func TestLinuxbrew_ListInstalled_UsesSingleDashOne(t *testing.T) {
	installFakeBinary(t, "brew",
		`if [ "$1" = "list" ] && [ "$2" = "--formula" ] && [ "$3" = "-1" ]; then
	echo "formula-a"
	exit 0
fi
echo "unexpected args: $*" >&2
exit 1`)

	pkgs, err := Linuxbrew{}.ListInstalled()
	if err != nil {
		t.Fatalf("ListInstalled: unexpected error: %v", err)
	}
	want := []string{"formula-a"}
	if len(pkgs) != len(want) {
		t.Fatalf("ListInstalled: got %v, want %v", pkgs, want)
	}
	if pkgs[0] != want[0] {
		t.Errorf("ListInstalled[0] = %q, want %q", pkgs[0], want[0])
	}
}

func TestBrew_Query_NotInstalled(t *testing.T) {
	installFakeBinary(t, "brew",
		`if [ "$1" = "list" ] && [ "$2" = "--formula" ] && [ "$3" = "__genv_missing__" ]; then
	exit 1
fi
if [ "$1" = "list" ] && [ "$2" = "--cask" ] && [ "$3" = "__genv_missing__" ]; then
	exit 1
fi`)
	ok, err := Brew{}.Query("__genv_missing__")
	if err != nil {
		t.Fatalf("Query: unexpected error: %v", err)
	}
	if ok {
		t.Error("Query: expected false for missing package")
	}
}

func TestBrew_Query_InstalledFormula(t *testing.T) {
	installFakeBinary(t, "brew",
		`if [ "$1" = "list" ] && [ "$2" = "--formula" ] && [ "$3" = "jq" ]; then
	exit 0
fi
exit 1`)
	ok, err := Brew{}.Query("jq")
	if err != nil {
		t.Fatalf("Query: unexpected error: %v", err)
	}
	if !ok {
		t.Error("Query: expected true for installed formula")
	}
}

func TestBrew_Query_InstalledCask(t *testing.T) {
	installFakeBinary(t, "brew",
		`if [ "$1" = "list" ] && [ "$2" = "--formula" ]; then
	exit 1
fi
if [ "$1" = "list" ] && [ "$2" = "--cask" ] && [ "$3" = "docker" ]; then
	exit 0
fi
exit 1`)
	ok, err := Brew{}.Query("docker")
	if err != nil {
		t.Fatalf("Query: unexpected error: %v", err)
	}
	if !ok {
		t.Error("Query: expected true for installed cask")
	}
}

func TestBrew_Available(t *testing.T) {
	orig := lookPath
	t.Cleanup(func() { lookPath = orig })

	lookPath = func(name string) (string, error) {
		if name == "brew" {
			return "/opt/homebrew/bin/brew", nil
		}
		return "", &os.PathError{Op: "lookpath", Err: os.ErrNotExist}
	}
	if !(Brew{}).Available() {
		t.Error("Available() = false when lookPath finds brew")
	}

	lookPath = func(string) (string, error) {
		return "", &os.PathError{Op: "lookpath", Err: os.ErrNotExist}
	}
	if (Brew{}).Available() {
		t.Error("Available() = true when lookPath fails")
	}
}
