package adapter

import (
	"os"
	"slices"
	"strings"
	"testing"
)

func isFailingCloudCommand(args []string) bool {
	return len(args) >= 4 && args[0] == "sh" && args[1] == "-c" && strings.Contains(args[2], "exit 1")
}

func TestKrew_Available_requiresKrewPlugin(t *testing.T) {
	orig := lookPath
	origProbe := krewProbe
	t.Cleanup(func() {
		lookPath = orig
		krewProbe = origProbe
	})
	lookPath = func(file string) (string, error) {
		if file == "kubectl" {
			return "/usr/bin/kubectl", nil
		}
		return "", os.ErrNotExist
	}
	krewProbe = func() error { return os.ErrNotExist }
	if (Krew{}).Available() {
		t.Fatal("Available() = true with kubectl but no krew plugin")
	}
	krewProbe = func() error { return nil }
	if !(Krew{}).Available() {
		t.Fatal("Available() = false when krew plugin probe succeeds")
	}
}

func TestKrew_PlanCommands_whenPluginTracked(t *testing.T) {
	a := Krew{}

	if got, want := a.PlanInstall("ctx"), []string{"kubectl", "krew", "install", "ctx"}; !slices.Equal(got, want) {
		t.Errorf("PlanInstall = %v, want %v", got, want)
	}
	if got, want := a.PlanUninstall("krew/ctx"), []string{"kubectl", "krew", "uninstall", "ctx"}; !slices.Equal(got, want) {
		t.Errorf("PlanUninstall = %v, want %v", got, want)
	}
	// PlanUpgrade must target a single plugin, never the broad `kubectl krew upgrade`.
	if got, want := a.PlanUpgrade("ctx"), []string{"kubectl", "krew", "upgrade", "ctx"}; !slices.Equal(got, want) {
		t.Errorf("PlanUpgrade = %v, want %v", got, want)
	}
	if len(a.PlanUpgrade("ctx")) != 4 {
		t.Errorf("PlanUpgrade must name the plugin, got %v", a.PlanUpgrade("ctx"))
	}
}

func TestKrew_ParseList_whenPluginsInstalled(t *testing.T) {
	installFakeBinary(t, "kubectl", `if [ "$1" = "krew" ] && [ "$2" = "list" ]; then
  echo 'PLUGIN   VERSION'
  echo 'ctx      v0.9.5'
  echo 'ns       v0.9.5'
fi`)

	plugins, err := Krew{}.ListInstalled()
	if err != nil {
		t.Fatalf("Krew.ListInstalled: %v", err)
	}
	if want := []string{"ctx", "ns"}; !slices.Equal(plugins, want) {
		t.Errorf("ListInstalled = %v, want %v", plugins, want)
	}

	ok, err := Krew{}.Query("ctx")
	if err != nil || !ok {
		t.Errorf("Query(ctx) = (%v, %v), want (true, nil)", ok, err)
	}
}

func TestHelm_PlanCommands_whenNameAndURL(t *testing.T) {
	a := Helm{}
	spec := "diff=https://github.com/databus23/helm-diff"

	if got, want := a.PlanInstall(spec), []string{"helm", "plugin", "install", "https://github.com/databus23/helm-diff"}; !slices.Equal(got, want) {
		t.Errorf("PlanInstall = %v, want %v", got, want)
	}
	if got, want := a.PlanUninstall(spec), []string{"helm", "plugin", "uninstall", "diff"}; !slices.Equal(got, want) {
		t.Errorf("PlanUninstall = %v, want %v", got, want)
	}
	// PlanUpgrade updates only the named plugin.
	if got, want := a.PlanUpgrade("diff"), []string{"helm", "plugin", "update", "diff"}; !slices.Equal(got, want) {
		t.Errorf("PlanUpgrade = %v, want %v", got, want)
	}
}

func TestHelm_NormalizeID_whenManagerOverrideIsURL(t *testing.T) {
	name, explicit := Helm{}.NormalizeID("diff", map[string]string{"helm": "https://github.com/databus23/helm-diff"})
	if !explicit || name != "diff=https://github.com/databus23/helm-diff" {
		t.Errorf("NormalizeID = (%q, %v), want name=url true", name, explicit)
	}
}

func TestHelm_PlanInstall_whenURLMissingFailsActionably(t *testing.T) {
	a := Helm{}
	if got := a.PlanInstall("diff"); !isFailingCloudCommand(got) {
		t.Errorf("PlanInstall missing url = %v, want failing command", got)
	}
}

func TestHelm_ParseList_whenPluginsInstalled(t *testing.T) {
	installFakeBinary(t, "helm", `if [ "$1" = "plugin" ] && [ "$2" = "list" ]; then
  echo 'NAME  VERSION  DESCRIPTION'
  echo 'diff  3.9.0    Preview helm upgrade changes'
fi`)

	ok, err := Helm{}.Query("diff")
	if err != nil || !ok {
		t.Errorf("Helm.Query(diff) = (%v, %v), want (true, nil)", ok, err)
	}
	version, err := Helm{}.QueryVersion("diff")
	if err != nil {
		t.Fatalf("Helm.QueryVersion: %v", err)
	}
	if version != "3.9.0" {
		t.Errorf("QueryVersion = %q, want 3.9.0", version)
	}
}
