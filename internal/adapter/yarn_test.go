package adapter

import (
	"maps"
	"slices"
	"testing"
)

func TestYarn_PlanCommands_whenPackageIsScopedAndVersioned(t *testing.T) {
	a := Yarn{}

	if got, want := a.PlanInstall("@scope/pkg@1.0.0"), []string{"yarn", "global", "add", "@scope/pkg@1.0.0"}; !slices.Equal(got, want) {
		t.Errorf("PlanInstall = %v, want %v", got, want)
	}
	if got, want := a.PlanUninstall("@scope/pkg@1.0.0"), []string{"yarn", "global", "remove", "@scope/pkg"}; !slices.Equal(got, want) {
		t.Errorf("PlanUninstall = %v, want %v", got, want)
	}
	if got, want := a.PlanUpgrade("@scope/pkg@1.0.0"), []string{"yarn", "global", "add", "@scope/pkg@1.0.0"}; !slices.Equal(got, want) {
		t.Errorf("PlanUpgrade = %v, want %v", got, want)
	}
	assertNoBroadJSUpdate(t, a.PlanUpgrade("@scope/pkg@1.0.0"))
}

func TestYarn_ParseGlobalListEntries_whenClassicOutputHasQuotedSpecs(t *testing.T) {
	lines := []string{
		"yarn global v1.22.22",
		"info \"@scope/pkg@1.0.0\" has binaries:",
		"info \"typescript@5.9.2\" has binaries:",
		"Done in 0.12s.",
	}

	got := parseYarnGlobalListEntries(lines)

	want := map[string]string{"@scope/pkg": "1.0.0", "typescript": "5.9.2"}
	if !maps.Equal(entriesVersions(got), want) {
		t.Errorf("parseYarnGlobalListEntries versions = %v, want %v", entriesVersions(got), want)
	}
}

func TestYarn_ListInstalledVersions_whenClassicOutputHasQuotedSpecs(t *testing.T) {
	installFakeBinary(t, "yarn", `if [ "$1" = "global" ] && [ "$2" = "list" ]; then
  echo 'yarn global v1.22.22'
  echo 'info "@scope/pkg@1.0.0" has binaries:'
  echo 'info "typescript@5.9.2" has binaries:'
fi`)

	versions, err := Yarn{}.ListInstalledVersions()

	if err != nil {
		t.Fatalf("Yarn.ListInstalledVersions: %v", err)
	}
	want := map[string]string{"@scope/pkg": "1.0.0", "typescript": "5.9.2"}
	if !maps.Equal(versions, want) {
		t.Errorf("ListInstalledVersions = %v, want %v", versions, want)
	}
}
