package adapter

import (
	"os"
	"slices"
	"testing"
)

// fakeMasList is the shared "mas list" body used across tests. It mimics the
// real "<id>  <name>  (<version>)" layout, including a multi-word app name.
const fakeMasList = `if [ "$1" = "list" ]; then
  echo " 937984704  Amphetamine         (5.3.2)"
  echo " 497799835  Xcode               (26.6)"
  echo "1631624924  Final Cut Pro       (12.3)"
  exit 0
fi
echo "unexpected args: $*" >&2
exit 1`

func TestMas_Name(t *testing.T) {
	if got := (Mas{}).Name(); got != "mas" {
		t.Errorf("Name() = %q, want %q", got, "mas")
	}
}

func TestMas_PlanInstall(t *testing.T) {
	args := Mas{}.PlanInstall("497799835")
	want := []string{"mas", "install", "497799835"}
	if len(args) != len(want) {
		t.Fatalf("PlanInstall: got %v, want %v", args, want)
	}
	for i, w := range want {
		if args[i] != w {
			t.Errorf("PlanInstall[%d] = %q, want %q", i, args[i], w)
		}
	}
}

func TestMas_PlanUninstall_UsesSudo(t *testing.T) {
	args := Mas{}.PlanUninstall("497799835")
	want := []string{"sudo", "mas", "uninstall", "497799835"}
	if len(args) != len(want) {
		t.Fatalf("PlanUninstall: got %v, want %v", args, want)
	}
	for i, w := range want {
		if args[i] != w {
			t.Errorf("PlanUninstall[%d] = %q, want %q", i, args[i], w)
		}
	}
}

func TestMas_PlanUpgrade(t *testing.T) {
	args := Mas{}.PlanUpgrade("497799835")
	assertContainsArg(t, args, "upgrade")
	assertContainsArg(t, args, "497799835")
}

func TestMas_PlanUpgradeBatch(t *testing.T) {
	args := Mas{}.PlanUpgradeBatch([]string{"497799835", "409201541"})
	want := []string{"mas", "upgrade", "497799835", "409201541"}
	if !slices.Equal(args, want) {
		t.Fatalf("PlanUpgradeBatch = %v, want %v", args, want)
	}
}

func TestMas_PlanClean_IsNoOp(t *testing.T) {
	cmds := Mas{}.PlanClean()
	if cmds != nil {
		t.Errorf("PlanClean() = %v, want nil", cmds)
	}
}

func TestMas_NormalizeID_ExplicitMapping(t *testing.T) {
	name, explicit := Mas{}.NormalizeID("xcode", map[string]string{"mas": "497799835"})
	if !explicit {
		t.Error("NormalizeID: expected explicit=true when managers has a mas entry")
	}
	if name != "497799835" {
		t.Errorf("NormalizeID = %q, want %q", name, "497799835")
	}
}

func TestMas_ListInstalled_ReturnsProductIDs(t *testing.T) {
	installFakeBinary(t, "mas", fakeMasList)
	pkgs, err := Mas{}.ListInstalled()
	if err != nil {
		t.Fatalf("ListInstalled: unexpected error: %v", err)
	}
	want := []string{"937984704", "497799835", "1631624924"}
	if len(pkgs) != len(want) {
		t.Fatalf("ListInstalled: got %v, want %v", pkgs, want)
	}
	for i, w := range want {
		if pkgs[i] != w {
			t.Errorf("ListInstalled[%d] = %q, want %q", i, pkgs[i], w)
		}
	}
}

func TestMas_QueryVersion_ParsesParenthesizedVersion(t *testing.T) {
	installFakeBinary(t, "mas", fakeMasList)
	// Final Cut Pro exercises the multi-word-name path: the version is the last
	// whitespace-separated field, not simply the third.
	got, err := Mas{}.QueryVersion("1631624924")
	if err != nil {
		t.Fatalf("QueryVersion: unexpected error: %v", err)
	}
	if got != "12.3" {
		t.Errorf("QueryVersion = %q, want %q", got, "12.3")
	}
}

func TestMas_QueryVersion_NotInstalled(t *testing.T) {
	installFakeBinary(t, "mas", fakeMasList)
	got, err := Mas{}.QueryVersion("111111111")
	if err != nil {
		t.Fatalf("QueryVersion: unexpected error: %v", err)
	}
	if got != "" {
		t.Errorf("QueryVersion = %q, want empty for missing app", got)
	}
}

func TestMas_Query(t *testing.T) {
	installFakeBinary(t, "mas", fakeMasList)
	ok, err := Mas{}.Query("497799835")
	if err != nil {
		t.Fatalf("Query: unexpected error: %v", err)
	}
	if !ok {
		t.Error("Query: expected true for installed app")
	}
	ok, err = Mas{}.Query("111111111")
	if err != nil {
		t.Fatalf("Query: unexpected error: %v", err)
	}
	if ok {
		t.Error("Query: expected false for missing app")
	}
}

func TestMas_Available(t *testing.T) {
	orig := lookPath
	t.Cleanup(func() { lookPath = orig })

	lookPath = func(name string) (string, error) {
		if name == "mas" {
			return "/opt/homebrew/bin/mas", nil
		}
		return "", &os.PathError{Op: "lookpath", Err: os.ErrNotExist}
	}
	if !(Mas{}).Available() {
		t.Error("Available() = false when lookPath finds mas")
	}

	lookPath = func(string) (string, error) {
		return "", &os.PathError{Op: "lookpath", Err: os.ErrNotExist}
	}
	if (Mas{}).Available() {
		t.Error("Available() = true when lookPath fails")
	}
}
