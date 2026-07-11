package adapter

import (
	"slices"
	"strings"
	"testing"
)

func isFailingUniversalCommand(args []string) bool {
	return len(args) >= 4 && args[0] == "sh" && args[1] == "-c" && strings.Contains(args[2], "exit 1")
}

func TestSdkman_PlanCommands_whenIDIsCandidateVersion(t *testing.T) {
	a := Sdkman{}

	if got, want := a.PlanInstall("java:21.0.2-tem"), []string{"sdk", "install", "java", "21.0.2-tem"}; !slices.Equal(got, want) {
		t.Errorf("PlanInstall = %v, want %v", got, want)
	}
	if got, want := a.PlanUninstall("java:21.0.2-tem"), []string{"sdk", "uninstall", "java", "21.0.2-tem"}; !slices.Equal(got, want) {
		t.Errorf("PlanUninstall = %v, want %v", got, want)
	}
	if got, want := a.PlanUpgrade("java:21.0.2-tem"), []string{"sdk", "install", "java", "21.0.2-tem"}; !slices.Equal(got, want) {
		t.Errorf("PlanUpgrade = %v, want %v", got, want)
	}
}

func TestSdkman_PlanInstall_whenIDInvalidFailsActionably(t *testing.T) {
	a := Sdkman{}
	for _, id := range []string{"java", ":21", "java:", ""} {
		if got := a.PlanInstall(id); !isFailingUniversalCommand(got) {
			t.Errorf("PlanInstall(%q) = %v, want failing command", id, got)
		}
	}
}

func TestAsdf_PlanCommands_whenPluginOrToolID(t *testing.T) {
	a := Asdf{}

	if got, want := a.PlanInstall("plugin:nodejs"), []string{"asdf", "plugin", "add", "nodejs"}; !slices.Equal(got, want) {
		t.Errorf("plugin PlanInstall = %v, want %v", got, want)
	}
	if got, want := a.PlanUninstall("plugin:nodejs"), []string{"asdf", "plugin", "remove", "nodejs"}; !slices.Equal(got, want) {
		t.Errorf("plugin PlanUninstall = %v, want %v", got, want)
	}
	if got, want := a.PlanInstall("tool:nodejs@22.11.0"), []string{"asdf", "install", "nodejs", "22.11.0"}; !slices.Equal(got, want) {
		t.Errorf("tool PlanInstall = %v, want %v", got, want)
	}
	if got, want := a.PlanUninstall("tool:nodejs@22.11.0"), []string{"asdf", "uninstall", "nodejs", "22.11.0"}; !slices.Equal(got, want) {
		t.Errorf("tool PlanUninstall = %v, want %v", got, want)
	}
	if got, want := a.PlanUpgrade("tool:nodejs@22.11.0"), []string{"asdf", "install", "nodejs", "22.11.0"}; !slices.Equal(got, want) {
		t.Errorf("tool PlanUpgrade = %v, want %v", got, want)
	}
}

func TestAsdf_PlanInstall_whenIDInvalidFailsActionably(t *testing.T) {
	a := Asdf{}
	for _, id := range []string{"nodejs", "plugin:", "tool:nodejs", "tool:@22", "tool:nodejs@", ""} {
		if got := a.PlanInstall(id); !isFailingUniversalCommand(got) {
			t.Errorf("PlanInstall(%q) = %v, want failing command", id, got)
		}
	}
}

func TestMise_PlanCommands_whenIDIsToolVersion(t *testing.T) {
	a := Mise{}

	if got, want := a.PlanInstall("node@22.11.0"), []string{"mise", "use", "-g", "node@22.11.0"}; !slices.Equal(got, want) {
		t.Errorf("PlanInstall = %v, want %v", got, want)
	}
	if got, want := a.PlanUninstall("node@22.11.0"), []string{"mise", "uninstall", "node@22.11.0"}; !slices.Equal(got, want) {
		t.Errorf("PlanUninstall = %v, want %v", got, want)
	}
	if got, want := a.PlanUpgrade("node@22.11.0"), []string{"mise", "use", "-g", "node@22.11.0"}; !slices.Equal(got, want) {
		t.Errorf("PlanUpgrade = %v, want %v", got, want)
	}
}

func TestMise_PlanInstall_whenIDInvalidFailsActionably(t *testing.T) {
	a := Mise{}
	for _, id := range []string{"node", "@22", "node@", ""} {
		if got := a.PlanInstall(id); !isFailingUniversalCommand(got) {
			t.Errorf("PlanInstall(%q) = %v, want failing command", id, got)
		}
	}
}

func TestMise_Query_whenToolVersionInstalled(t *testing.T) {
	installFakeBinary(t, "mise", `if [ "$1" = "ls" ] && [ "$2" = "node" ] && [ "$3" = "--installed" ]; then
  echo 'node  22.11.0  ~/.config/mise/config.toml'
  echo 'node  20.18.0'
fi`)

	ok, err := Mise{}.Query("node@22.11.0")
	if err != nil {
		t.Fatalf("Mise.Query: %v", err)
	}
	if !ok {
		t.Error("Mise.Query = false, want true")
	}

	absent, err := Mise{}.Query("node@18.0.0")
	if err != nil {
		t.Fatalf("Mise.Query absent: %v", err)
	}
	if absent {
		t.Error("Mise.Query absent = true, want false")
	}
}
