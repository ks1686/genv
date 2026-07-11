package adapter

import (
	"maps"
	"slices"
	"strings"
	"testing"
)

func TestVolta_PlanCommands_whenPackageIsScopedAndVersioned(t *testing.T) {
	a := Volta{}

	if got, want := a.PlanInstall("@scope/pkg@1.0.0"), []string{"volta", "install", "@scope/pkg@1.0.0"}; !slices.Equal(got, want) {
		t.Errorf("PlanInstall = %v, want %v", got, want)
	}
	if got, want := a.PlanUninstall("@scope/pkg@1.0.0"), []string{"volta", "uninstall", "@scope/pkg"}; !slices.Equal(got, want) {
		t.Errorf("PlanUninstall = %v, want %v", got, want)
	}
	if got, want := a.PlanUpgrade("@scope/pkg@1.0.0"), []string{"volta", "install", "@scope/pkg@1.0.0"}; !slices.Equal(got, want) {
		t.Errorf("PlanUpgrade = %v, want %v", got, want)
	}
	assertNoBroadJSUpdate(t, a.PlanUpgrade("@scope/pkg@1.0.0"))
}

func TestVolta_ParseListAllEntries_whenOutputHasToolsSection(t *testing.T) {
	lines := []string{
		"⚡️ User toolchain:",
		"    node@22.11.0",
		"    npm@10.9.0",
		"⚡️ Tools:",
		"    typescript@5.9.2 (default)",
		"    @scope/pkg@1.0.0",
	}

	got := parseVoltaListAllEntries(lines)

	want := map[string]string{"typescript": "5.9.2", "@scope/pkg": "1.0.0"}
	if !maps.Equal(entriesVersions(got), want) {
		t.Errorf("parseVoltaListAllEntries versions = %v, want %v", entriesVersions(got), want)
	}
}

func TestVolta_ListInstalledVersions_whenVoltaOutputsTools(t *testing.T) {
	installFakeBinary(t, "volta", `if [ "$1" = "list" ] && [ "$2" = "all" ]; then
  echo '⚡️ User toolchain:'
  echo '    node@22.11.0'
  echo '    npm@10.9.0'
  echo '⚡️ Tools:'
  echo '    typescript@5.9.2 (default)'
  echo '    @scope/pkg@1.0.0'
fi`)

	versions, err := Volta{}.ListInstalledVersions()

	if err != nil {
		t.Fatalf("Volta.ListInstalledVersions: %v", err)
	}
	want := map[string]string{"typescript": "5.9.2", "@scope/pkg": "1.0.0"}
	if !maps.Equal(versions, want) {
		t.Errorf("ListInstalledVersions = %v, want %v", versions, want)
	}
}

func assertNoBroadJSUpdate(t *testing.T, args []string) {
	t.Helper()
	joined := strings.Join(args, " ")
	for _, broad := range []string{"npm update -g", "npm update --global", "pnpm update -g", "pnpm update --global", "yarn global upgrade", "volta update"} {
		if strings.Contains(joined, broad) {
			t.Fatalf("command %q contains broad update %q", joined, broad)
		}
	}
}
