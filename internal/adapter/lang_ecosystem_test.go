package adapter

import (
	"slices"
	"strings"
	"testing"
)

func isFailingLangCommand(args []string) bool {
	return len(args) >= 4 && args[0] == "sh" && args[1] == "-c" && strings.Contains(args[2], "exit 1")
}

func TestGhcup_PlanCommands_whenIDIsToolVersion(t *testing.T) {
	a := Ghcup{}

	if got, want := a.PlanInstall("ghc:9.4.8"), []string{"ghcup", "install", "ghc", "9.4.8"}; !slices.Equal(got, want) {
		t.Errorf("PlanInstall = %v, want %v", got, want)
	}
	if got, want := a.PlanUninstall("cabal:3.10.2.0"), []string{"ghcup", "rm", "cabal", "3.10.2.0"}; !slices.Equal(got, want) {
		t.Errorf("PlanUninstall = %v, want %v", got, want)
	}
	if got, want := a.PlanUpgrade("hls:2.6.0.0"), []string{"ghcup", "install", "hls", "2.6.0.0"}; !slices.Equal(got, want) {
		t.Errorf("PlanUpgrade = %v, want %v", got, want)
	}
}

func TestGhcup_PlanCommands_whenIDIsInvalidFailsActionably(t *testing.T) {
	a := Ghcup{}
	for _, id := range []string{"9.4.8", "ghc:", "python:3.12", ""} {
		if got := a.PlanInstall(id); !isFailingLangCommand(got) {
			t.Errorf("PlanInstall(%q) = %v, want failing command", id, got)
		}
	}
	ok, err := a.Query("not-a-tool")
	if err != nil || ok {
		t.Errorf("Query(invalid) = (%v, %v), want (false, nil)", ok, err)
	}
}

func TestOpam_PlanCommands_whenIDIsSwitchScoped(t *testing.T) {
	a := Opam{}

	if got, want := a.PlanInstall("4.14:dune"), []string{"opam", "install", "--switch", "4.14", "-y", "dune"}; !slices.Equal(got, want) {
		t.Errorf("PlanInstall = %v, want %v", got, want)
	}
	if got, want := a.PlanUninstall("4.14:dune.3.14.0"), []string{"opam", "remove", "--switch", "4.14", "-y", "dune"}; !slices.Equal(got, want) {
		t.Errorf("PlanUninstall = %v, want %v", got, want)
	}
	if got, want := a.PlanUpgrade("4.14:dune"), []string{"opam", "upgrade", "--switch", "4.14", "-y", "dune"}; !slices.Equal(got, want) {
		t.Errorf("PlanUpgrade = %v, want %v", got, want)
	}
}

func TestOpam_PlanCommands_whenIDMissingSwitchFailsActionably(t *testing.T) {
	a := Opam{}
	for _, id := range []string{"dune", ":dune", "4.14:", ""} {
		if got := a.PlanInstall(id); !isFailingLangCommand(got) {
			t.Errorf("PlanInstall(%q) = %v, want failing command", id, got)
		}
	}
}

func TestJuliaup_PlanCommands_whenIDIsChannel(t *testing.T) {
	a := Juliaup{}

	if got, want := a.PlanInstall("release"), []string{"juliaup", "add", "release"}; !slices.Equal(got, want) {
		t.Errorf("PlanInstall = %v, want %v", got, want)
	}
	if got, want := a.PlanUninstall("1.10"), []string{"juliaup", "remove", "1.10"}; !slices.Equal(got, want) {
		t.Errorf("PlanUninstall = %v, want %v", got, want)
	}
	if got, want := a.PlanUpgrade("lts"), []string{"juliaup", "update", "lts"}; !slices.Equal(got, want) {
		t.Errorf("PlanUpgrade = %v, want %v", got, want)
	}
}

func TestJuliaup_PlanCommands_whenIDIsInvalidFailsActionably(t *testing.T) {
	a := Juliaup{}
	for _, id := range []string{"", "1.10 beta", "release channel"} {
		if got := a.PlanInstall(id); !isFailingLangCommand(got) {
			t.Errorf("PlanInstall(%q) = %v, want failing command", id, got)
		}
	}
}

func TestJuliaup_ParseInstalledChannels_whenStatusOutput(t *testing.T) {
	installFakeBinary(t, "juliaup", `if [ "$1" = "status" ]; then
  echo ' Default  Channel  Version               Update'
  echo '        *  release  1.10.4+0.aarch64.apple.darwin14'
  echo '           lts      1.6.7+0.aarch64.apple.darwin14'
fi`)

	channels, err := Juliaup{}.ListInstalled()
	if err != nil {
		t.Fatalf("Juliaup.ListInstalled: %v", err)
	}
	if want := []string{"release", "lts"}; !slices.Equal(channels, want) {
		t.Errorf("ListInstalled = %v, want %v", channels, want)
	}
}

func TestStack_PlanCommands_whenTrackedAndUninstallUnsupported(t *testing.T) {
	a := Stack{}

	if got, want := a.PlanInstall("hlint"), []string{"stack", "install", "hlint"}; !slices.Equal(got, want) {
		t.Errorf("PlanInstall = %v, want %v", got, want)
	}
	if got, want := a.PlanUpgrade("hlint"), []string{"stack", "install", "hlint"}; !slices.Equal(got, want) {
		t.Errorf("PlanUpgrade = %v, want %v", got, want)
	}
	if got := a.PlanUninstall("hlint"); !isFailingLangCommand(got) {
		t.Errorf("PlanUninstall = %v, want failing unsupported command", got)
	}
}
