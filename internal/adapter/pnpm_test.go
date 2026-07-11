package adapter

import (
	"maps"
	"slices"
	"testing"
)

func TestPnpm_PlanCommands_whenPackageIsScopedAndVersioned(t *testing.T) {
	a := Pnpm{}

	if got, want := a.PlanInstall("@scope/pkg@1.0.0"), []string{"pnpm", "add", "--global", "@scope/pkg@1.0.0"}; !slices.Equal(got, want) {
		t.Errorf("PlanInstall = %v, want %v", got, want)
	}
	if got, want := a.PlanUninstall("@scope/pkg@1.0.0"), []string{"pnpm", "remove", "--global", "@scope/pkg"}; !slices.Equal(got, want) {
		t.Errorf("PlanUninstall = %v, want %v", got, want)
	}
	if got, want := a.PlanUpgrade("@scope/pkg@1.0.0"), []string{"pnpm", "add", "--global", "@scope/pkg@1.0.0"}; !slices.Equal(got, want) {
		t.Errorf("PlanUpgrade = %v, want %v", got, want)
	}
	assertNoBroadJSUpdate(t, a.PlanUpgrade("@scope/pkg@1.0.0"))
}

func TestPnpm_ListInstalledVersions_whenPnpmOutputsJSONArray(t *testing.T) {
	installFakeBinary(t, "pnpm", `if [ "$1" = "list" ] && [ "$2" = "--global" ] && [ "$3" = "--depth=0" ] && [ "$4" = "--json" ]; then
  cat <<'JSON'
[{"dependencies":{"@scope/pkg":{"version":"1.0.0"},"typescript":{"version":"5.9.2"}}}]
JSON
fi`)

	versions, err := Pnpm{}.ListInstalledVersions()

	if err != nil {
		t.Fatalf("Pnpm.ListInstalledVersions: %v", err)
	}
	want := map[string]string{"@scope/pkg": "1.0.0", "typescript": "5.9.2"}
	if !maps.Equal(versions, want) {
		t.Errorf("ListInstalledVersions = %v, want %v", versions, want)
	}
}

func TestPnpm_Query_whenScopedPackageHasVersionSuffix(t *testing.T) {
	installFakeBinary(t, "pnpm", `if [ "$1" = "list" ]; then
  printf '%s\n' '[{"dependencies":{"@scope/pkg":{"version":"1.0.0"}}}]'
fi`)

	ok, err := Pnpm{}.Query("@scope/pkg@latest")

	if err != nil {
		t.Fatalf("Pnpm.Query: %v", err)
	}
	if !ok {
		t.Error("Pnpm.Query = false, want true")
	}
}
