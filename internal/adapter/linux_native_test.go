package adapter

import "testing"

func TestApt_PlanInstall(t *testing.T) {
	got := Apt{}.PlanInstall("git")
	want := []string{"sudo", "apt-get", "install", "-y", "git"}
	assertArgv(t, got, want)
}

func TestDnf_PlanInstall(t *testing.T) {
	got := Dnf{}.PlanInstall("git")
	want := []string{"sudo", "dnf", "install", "-y", "git"}
	assertArgv(t, got, want)
}

func TestApk_PlanInstall(t *testing.T) {
	got := Apk{}.PlanInstall("git")
	want := []string{"sudo", "apk", "add", "git"}
	assertArgv(t, got, want)
}

func TestParseAptSimulatedUpgrade(t *testing.T) {
	lines := []string{
		"Inst git [1:2.43.0-1] (1:2.45.2-1ubuntu1 Ubuntu:24.04/noble-updates [amd64])",
		"Conf git (1:2.45.2-1ubuntu1 Ubuntu:24.04/noble-updates [amd64])",
	}
	got := parseAptSimulatedUpgrade(lines)
	if got["git"] != "1:2.45.2-1ubuntu1" {
		t.Fatalf("parseAptSimulatedUpgrade git = %q", got["git"])
	}
}

func TestParseDnfCheckUpdate(t *testing.T) {
	lines := []string{
		"Last metadata expiration check: 0:01:00 ago",
		"git.x86_64                    2.45.1-1.fc40          updates",
	}
	got := parseDnfCheckUpdate(lines)
	if got["git"] != "2.45.1-1.fc40" {
		t.Fatalf("parseDnfCheckUpdate git = %q want 2.45.1-1.fc40", got["git"])
	}
}

func TestSplitApkNameVersion(t *testing.T) {
	name, ver := splitApkNameVersion("git-2.45.0-r0")
	if name != "git" || ver != "2.45.0-r0" {
		t.Fatalf("splitApkNameVersion(git-2.45.0-r0) = %q, %q", name, ver)
	}
	name, ver = splitApkNameVersion("py3-setuptools-70.0.0-r1")
	if name != "py3-setuptools" || ver != "70.0.0-r1" {
		t.Fatalf("splitApkNameVersion(py3-setuptools) = %q, %q", name, ver)
	}
}

func TestParseApkVersionOutdated(t *testing.T) {
	got := parseApkVersionOutdated([]string{"git-2.45.0-r0 < git-2.46.0-r1"})
	if got["git"] != "2.46.0-r1" {
		t.Fatalf("parseApkVersionOutdated = %#v, want git=2.46.0-r1", got)
	}
}

func TestApk_Search_stripsVersions(t *testing.T) {
	installFakeBinary(t, "apk", `if [ "$1" = "search" ]; then echo git-2.45.0-r0; echo git-doc-2.45.0-r0; exit 0; fi; exit 1`)
	got, err := Apk{}.Search("git")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(got) == 0 || got[0] != "git" {
		t.Fatalf("Search = %#v, want git first", got)
	}
}

func TestApk_ListOutdated_parse(t *testing.T) {
	installFakeBinary(t, "apk", `if [ "$1" = "version" ]; then echo 'git-2.45.0-r0 < git-2.46.0-r1'; exit 0; fi; exit 1`)
	got, err := Apk{}.ListOutdated([]string{"git"})
	if err != nil {
		t.Fatalf("ListOutdated: %v", err)
	}
	if got["git"] != "2.46.0-r1" {
		t.Fatalf("ListOutdated = %#v", got)
	}
}

func TestApt_ListOutdated_parse(t *testing.T) {
	installFakeBinary(t, "apt-get", `if [ "$1" = "-s" ]; then echo 'Inst git [1:2.43.0-1] (1:2.45.2-1ubuntu1 Ubuntu:24.04/noble-updates [amd64])'; exit 0; fi; exit 1`)
	got, err := Apt{}.ListOutdated([]string{"git"})
	if err != nil {
		t.Fatalf("ListOutdated: %v", err)
	}
	if got["git"] != "1:2.45.2-1ubuntu1" {
		t.Fatalf("ListOutdated = %#v", got)
	}
}

func TestDnf_ListOutdated_parse(t *testing.T) {
	installFakeBinary(t, "dnf", `if [ "$1" = "check-update" ]; then echo 'git.x86_64 2.45.1-1.fc40 updates'; exit 100; fi; exit 1`)
	got, err := Dnf{}.ListOutdated([]string{"git"})
	if err != nil {
		t.Fatalf("ListOutdated: %v", err)
	}
	if got["git"] != "2.45.1-1.fc40" {
		t.Fatalf("ListOutdated = %#v", got)
	}
}

func TestApk_ListNames_stripsVersions(t *testing.T) {
	installFakeBinary(t, "apk", `if [ "$1" = "search" ] && [ "$2" = "-q" ]; then echo git-2.45.0-r0; echo curl-8.0.0-r1; exit 0; fi; exit 1`)
	got, err := Apk{}.ListNames()
	if err != nil {
		t.Fatalf("ListNames: %v", err)
	}
	if len(got) != 2 || got[0] != "git" || got[1] != "curl" {
		t.Fatalf("ListNames = %#v, want git curl", got)
	}
}

func TestApt_Query_Installed(t *testing.T) {
	installFakeBinary(t, "dpkg-query",
		`if [ "$1" = "-W" ] && [ "$2" = "git" ]; then exit 0; fi; exit 1`)
	ok, err := Apt{}.Query("git")
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if !ok {
		t.Fatal("expected git installed")
	}
}

func assertArgv(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("argv len = %d want %d (%v vs %v)", len(got), len(want), got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("argv[%d] = %q want %q", i, got[i], want[i])
		}
	}
}
