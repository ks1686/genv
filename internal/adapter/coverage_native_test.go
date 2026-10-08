package adapter

import (
	"context"
	"errors"
	"os/exec"
	"reflect"
	"runtime"
	"testing"
)

func skipWithoutShell(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake shell binaries are POSIX-only")
	}
}

func TestApt_SearchListNamesAndInstalledVersions(t *testing.T) {
	skipWithoutShell(t)
	installFakeBinary(t, "apt-cache", `case "$1" in
search) printf 'git - fast vcs\ngit-doc - docs\nlibgit2 - lib\nripgrep - grep\n' ;;
pkgnames) printf 'git\nripgrep\n' ;;
esac`)
	installFakeBinary(t, "dpkg-query", `printf 'git 1:2.45.2-1\nripgrep 14.1.0-1\nbroken\n'`)

	got, err := Apt{}.Search("git")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	// "ripgrep" does not contain "git"; libgit2 and git-doc do.
	if want := []string{"git", "git-doc", "libgit2"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Search = %v, want %v", got, want)
	}
	names, err := Apt{}.ListNames()
	if err != nil || !reflect.DeepEqual(names, []string{"git", "ripgrep"}) {
		t.Errorf("ListNames = %v, %v", names, err)
	}
	versions, err := Apt{}.ListInstalledVersions()
	if err != nil {
		t.Fatalf("ListInstalledVersions: %v", err)
	}
	if versions["git"] != "1:2.45.2-1" || versions["ripgrep"] != "14.1.0-1" {
		t.Errorf("versions = %v", versions)
	}
	if _, ok := versions["broken"]; ok {
		t.Error("a line with no version must be ignored")
	}
}

func TestApt_SearchNoMatchesAndFailure(t *testing.T) {
	skipWithoutShell(t)
	installFakeBinary(t, "apt-cache", `exit 0`)
	if got, err := (Apt{}).SearchContext(context.Background(), "zzz"); err != nil || len(got) != 0 {
		t.Errorf("empty search = %v, %v", got, err)
	}
	// A non-zero exit is the documented "empty inventory" convention of the
	// list helpers; only a failure to run the binary at all is an error.
	installFakeBinary(t, "dpkg-query", `exit 3`)
	if got, err := (Apt{}).ListInstalledVersions(); err != nil || len(got) != 0 {
		t.Errorf("exited dpkg-query = %v, %v; want an empty inventory", got, err)
	}
}

func TestDnf_SearchListNamesAndInstalledVersions(t *testing.T) {
	skipWithoutShell(t)
	installFakeBinary(t, "dnf", `case "$1" in
search) printf 'Last metadata expiration check: 0:00:01 ago\n====== Name Matched: git ======\ngit.x86_64 : Fast VCS\ngit-core.x86_64 : core\n' ;;
repoquery) printf 'git\ngit-core\n' ;;
esac`)
	installFakeBinary(t, "rpm", `printf 'git 2.45.2\ncurl 8.0.0\n'`)

	if _, err := (Dnf{}).Search("git"); err != nil {
		t.Fatalf("Search: %v", err)
	}
	if _, err := (Dnf{}).SearchContext(context.Background(), "git"); err != nil {
		t.Fatalf("SearchContext: %v", err)
	}
	if _, err := (Dnf{}).ListNames(); err != nil {
		t.Fatalf("ListNames: %v", err)
	}
	if _, err := (Dnf{}).ListNamesContext(context.Background()); err != nil {
		t.Fatalf("ListNamesContext: %v", err)
	}
	if _, err := (Dnf{}).ListInstalledVersions(); err != nil {
		t.Fatalf("ListInstalledVersions: %v", err)
	}
}

func TestApk_ListInstalledVersions(t *testing.T) {
	skipWithoutShell(t)
	installFakeBinary(t, "apk", `printf 'git-2.45.0-r0\npy3-setuptools-70.0.0-r1\n\n'`)
	versions, err := Apk{}.ListInstalledVersions()
	if err != nil {
		t.Fatalf("ListInstalledVersions: %v", err)
	}
	if versions["git"] != "2.45.0-r0" || versions["py3-setuptools"] != "70.0.0-r1" {
		t.Errorf("versions = %v", versions)
	}
	if got := parseApkInfoVersions([]string{"noversion"}); len(got) != 0 {
		t.Errorf("a name without a version must be dropped, got %v", got)
	}
	installFakeBinary(t, "apk", `exit 2`)
	if _, err := (Apk{}).ListInstalledVersions(); err == nil {
		t.Error("a failing apk must surface")
	}
}

func TestExternalAdapterIsTrackOnly(t *testing.T) {
	var e External
	e.TrackOnly()
	if e.PlanUninstall("x") != nil || e.PlanUpgrade("x") != nil || e.PlanInstall("x") != nil || e.PlanClean() != nil {
		t.Error("external packages are never planned for install/uninstall/upgrade/clean")
	}
}

func TestDefaultFallbackEligibleIsOptIn(t *testing.T) {
	eligible := []Adapter{Brew{}, Mas{}, Pacman{}, Paru{}, Yay{}, Snap{}, Apt{}, Dnf{}, Apk{}, Linuxbrew{}, Winget{}, Scoop{}, Choco{}}
	for _, a := range eligible {
		if !IsDefaultFallbackEligible(a) {
			t.Errorf("%s should be a default-fallback manager", a.Name())
		}
		a.(DefaultFallbackEligible).DefaultFallbackEligible()
	}
	for _, a := range []Adapter{Npm{}, Cargo{}, PipUser{}, External{}} {
		if IsDefaultFallbackEligible(a) {
			t.Errorf("%s must never be a blind default fallback", a.Name())
		}
	}
}

func TestCommandAdapterNormalizeAndClean(t *testing.T) {
	c := NewCommand("plug", CommandDef{List: "plugctl list", Install: "plugctl add {{id}}", Remove: "plugctl rm {{id}}"})
	if name, explicit := c.NormalizeID("slack", map[string]string{"plug": "slack-desktop"}); name != "slack-desktop" || !explicit {
		t.Errorf("NormalizeID = %q, %v; want the mapped name, explicit", name, explicit)
	}
	if name, explicit := c.NormalizeID("slack", nil); name != "slack" || explicit {
		t.Errorf("NormalizeID without mapping = %q, %v", name, explicit)
	}
	if c.PlanClean() != nil {
		t.Error("a spec adapter has nothing to clean")
	}
}

func TestCommandOutputErrorIgnoresExitStatusOnly(t *testing.T) {
	skipWithoutShell(t)
	exitErr := exec.Command("sh", "-c", "exit 1").Run()
	if commandOutputError(exitErr) != nil {
		t.Error("a non-zero exit is an empty inventory, not an error")
	}
	other := errors.New("boom")
	if got := commandOutputError(other); got != other {
		t.Errorf("other errors must pass through, got %v", got)
	}
}

func TestFindComposerEntryAndCondaVersions(t *testing.T) {
	entries := []composerEntry{{name: "laravel/installer", version: "5.0"}}
	if got, ok := findComposerEntry(entries, "laravel/installer:^5"); !ok || got.version != "5.0" {
		t.Errorf("findComposerEntry = %+v, %v", got, ok)
	}
	if _, ok := findComposerEntry(entries, "other/pkg"); ok {
		t.Error("an unknown package must not match")
	}

	skipWithoutShell(t)
	installFakeBinary(t, "conda", `printf '[{"name":"numpy","version":"1.26"}]'`)
	got, err := listCondaVersions("conda", "base")
	if err != nil || len(got) != 1 || got[0].name != "numpy" {
		t.Errorf("listCondaVersions = %v, %v", got, err)
	}
	installFakeBinary(t, "conda", `exit 1`)
	if got, err := listCondaVersions("conda", "base"); err != nil || got != nil {
		t.Errorf("an exited conda is an empty env, got %v, %v", got, err)
	}
}

func TestGhcupOpamAndPipUserInventories(t *testing.T) {
	skipWithoutShell(t)
	installFakeBinary(t, "ghcup", `printf 'ghc 9.4.8 installed\nghc 9.6.2 installed\ncabal 3.10 installed\nshort\n'`)
	versions, err := ghcupInstalledVersions("ghc")
	if err != nil || !reflect.DeepEqual(versions, []string{"9.4.8", "9.6.2"}) {
		t.Errorf("ghcupInstalledVersions = %v, %v", versions, err)
	}

	installFakeBinary(t, "opam", `printf 'dune 3.14\nocaml\n\n'`)
	packages, err := opamInstalledPackages("default")
	if err != nil {
		t.Fatalf("opamInstalledPackages: %v", err)
	}
	if packages["dune"] != "3.14" {
		t.Errorf("dune = %q", packages["dune"])
	}
	if v, ok := packages["ocaml"]; !ok || v != "" {
		t.Errorf("a package without a version must be present and empty, got %q, %v", v, ok)
	}

	installFakeBinary(t, "python3", `printf '[{"name":"httpie","version":"3.2"}]'`)
	pipVersions, err := PipUser{}.ListInstalledVersions()
	if err != nil || pipVersions["httpie"] != "3.2" {
		t.Errorf("PipUser.ListInstalledVersions = %v, %v", pipVersions, err)
	}
}

func TestMasCompletionNamesDelegateToSearch(t *testing.T) {
	skipWithoutShell(t)
	installFakeBinary(t, "mas", `printf '497799835  Xcode  (15.0)\n'`)
	a, errA := Mas{}.CompletionNames("Xcode")
	b, errB := Mas{}.CompletionNamesContext(context.Background(), "Xcode")
	if (errA == nil) != (errB == nil) || !reflect.DeepEqual(a, b) {
		t.Errorf("CompletionNames %v/%v and CompletionNamesContext %v/%v must agree", a, errA, b, errB)
	}
}
