package adapter

import (
	"os"
	"testing"
)

func TestSnap_Name(t *testing.T) {
	if got := (Snap{}).Name(); got != "snap" {
		t.Errorf("Name() = %q, want %q", got, "snap")
	}
}

func TestSnap_PlanUpgradeBatch(t *testing.T) {
	args := Snap{}.PlanUpgradeBatch([]string{"firefox", "code"})
	want := []string{"sudo", "snap", "refresh", "firefox", "code"}
	if len(args) != len(want) {
		t.Fatalf("PlanUpgradeBatch: got %v, want %v", args, want)
	}
	for i, w := range want {
		if args[i] != w {
			t.Errorf("PlanUpgradeBatch[%d] = %q, want %q", i, args[i], w)
		}
	}
}

func TestSnap_ListInstalledVersions(t *testing.T) {
	installFakeBinary(t, "snap",
		`if [ "$1" = "list" ]; then
	echo "Name  Version  Rev  Tracking  Publisher  Notes"
	echo "core  16-2.61  16928  latest/stable  canonical  core"
	echo "firefox  138.0-1  1234  latest/stable  mozilla  -"
fi`)
	versions, err := Snap{}.ListInstalledVersions()
	if err != nil {
		t.Fatalf("ListInstalledVersions: %v", err)
	}
	want := map[string]string{"core": "16-2.61", "firefox": "138.0-1"}
	if len(versions) != len(want) {
		t.Fatalf("ListInstalledVersions: got %v, want %v", versions, want)
	}
	for pkg, ver := range want {
		if versions[pkg] != ver {
			t.Errorf("ListInstalledVersions[%q] = %q, want %q", pkg, versions[pkg], ver)
		}
	}
}

func TestSnap_Available(t *testing.T) {
	orig := lookPath
	t.Cleanup(func() { lookPath = orig })

	lookPath = func(name string) (string, error) {
		if name == "snap" {
			return "/usr/bin/snap", nil
		}
		return "", &os.PathError{Op: "lookpath", Err: os.ErrNotExist}
	}
	if !(Snap{}).Available() {
		t.Error("Available() = false when lookPath finds snap")
	}

	lookPath = func(string) (string, error) {
		return "", &os.PathError{Op: "lookpath", Err: os.ErrNotExist}
	}
	if (Snap{}).Available() {
		t.Error("Available() = true when lookPath fails")
	}
}
