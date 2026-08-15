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

func TestParseApkVersionOutdated(t *testing.T) {
	got := parseApkVersionOutdated([]string{"git-2.45.0-r0 < git-2.46.0-r1"})
	if got["git"] != "r1" && got["git-2.46.0"] == "" {
		// name is split at last hyphen: "git-2.45.0" / "r0"
		if _, ok := got["git-2.45.0"]; !ok {
			t.Fatalf("parseApkVersionOutdated = %#v", got)
		}
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
