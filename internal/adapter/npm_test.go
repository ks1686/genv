package adapter

import (
	"maps"
	"slices"
	"testing"
)

func TestNpm_PlanCommands_whenPackageIsScopedAndVersioned(t *testing.T) {
	a := Npm{}

	if got, want := a.PlanInstall("@scope/pkg@1.0.0"), []string{"npm", "install", "--global", "@scope/pkg@1.0.0"}; !slices.Equal(got, want) {
		t.Errorf("PlanInstall = %v, want %v", got, want)
	}
	if got, want := a.PlanUninstall("@scope/pkg@1.0.0"), []string{"npm", "uninstall", "--global", "@scope/pkg"}; !slices.Equal(got, want) {
		t.Errorf("PlanUninstall = %v, want %v", got, want)
	}
	if got, want := a.PlanUpgrade("@scope/pkg@1.0.0"), []string{"npm", "install", "--global", "@scope/pkg@1.0.0"}; !slices.Equal(got, want) {
		t.Errorf("PlanUpgrade = %v, want %v", got, want)
	}
	assertNoBroadJSUpdate(t, a.PlanUpgrade("@scope/pkg@1.0.0"))
}

func TestNpm_ListInstalledVersions_whenNpmOutputsJSON(t *testing.T) {
	installFakeBinary(t, "npm", `if [ "$1" = "list" ] && [ "$2" = "--global" ] && [ "$3" = "--depth=0" ] && [ "$4" = "--json" ]; then
  cat <<'JSON'
{"dependencies":{"@scope/pkg":{"version":"1.0.0"},"typescript":{"version":"5.9.2"}}}
JSON
fi`)

	versions, err := Npm{}.ListInstalledVersions()

	if err != nil {
		t.Fatalf("Npm.ListInstalledVersions: %v", err)
	}
	want := map[string]string{"@scope/pkg": "1.0.0", "typescript": "5.9.2"}
	if !maps.Equal(versions, want) {
		t.Errorf("ListInstalledVersions = %v, want %v", versions, want)
	}
}

func TestNpm_QueryVersion_whenScopedPackageHasVersionSuffix(t *testing.T) {
	installFakeBinary(t, "npm", `if [ "$1" = "list" ]; then
  printf '%s\n' '{"dependencies":{"@scope/pkg":{"version":"1.0.0"}}}'
fi`)

	version, err := Npm{}.QueryVersion("@scope/pkg@latest")

	if err != nil {
		t.Fatalf("Npm.QueryVersion: %v", err)
	}
	if version != "1.0.0" {
		t.Errorf("QueryVersion = %q, want %q", version, "1.0.0")
	}
}

func TestNpm_ListInstalled_whenNpmOutputsJSON(t *testing.T) {
	installFakeBinary(t, "npm", `if [ "$1" = "list" ]; then
  printf '%s\n' '{"dependencies":{"@scope/pkg":{"version":"1.0.0"},"typescript":{"version":"5.9.2"}}}'
fi`)

	names, err := Npm{}.ListInstalled()

	if err != nil {
		t.Fatalf("Npm.ListInstalled: %v", err)
	}
	if want := []string{"@scope/pkg", "typescript"}; !slices.Equal(names, want) {
		t.Errorf("ListInstalled = %v, want %v", names, want)
	}
}

func TestNpm_ListInstalled_SkipsNpmSelfKeepsCorepack(t *testing.T) {
	installFakeBinary(t, "npm", `if [ "$1" = "list" ]; then
  printf '%s\n' '{"dependencies":{"corepack":{"version":"0.31.0"},"npm":{"version":"10.9.2"},"typescript":{"version":"5.9.2"}}}'
fi`)

	names, err := Npm{}.ListInstalled()
	if err != nil {
		t.Fatalf("Npm.ListInstalled: %v", err)
	}
	if slices.Contains(names, "npm") {
		t.Errorf("ListInstalled = %v, must not include npm itself", names)
	}
	if want := []string{"corepack", "typescript"}; !slices.Equal(names, want) {
		t.Errorf("ListInstalled = %v, want %v", names, want)
	}

	ok, err := Npm{}.Query("npm")
	if err != nil {
		t.Fatalf("Npm.Query(npm): %v", err)
	}
	if ok {
		t.Error("Npm.Query(npm) = true, want false")
	}

	versions, err := Npm{}.ListInstalledVersions()
	if err != nil {
		t.Fatalf("Npm.ListInstalledVersions: %v", err)
	}
	if _, found := versions["npm"]; found {
		t.Errorf("ListInstalledVersions includes npm: %v", versions)
	}
	if _, found := versions["corepack"]; !found {
		t.Errorf("ListInstalledVersions missing corepack: %v", versions)
	}
}
