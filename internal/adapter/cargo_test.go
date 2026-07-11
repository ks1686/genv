package adapter

import (
	"maps"
	"slices"
	"testing"
)

func TestCargo_ParseInstallListEntries_whenOutputHasMultilineBinaries(t *testing.T) {
	// Given
	lines := []string{
		"ripgrep v14.1.1:",
		"    rg",
		"cargo-edit v0.13.0:",
		"    cargo-add",
		"    cargo-rm",
		"malformed",
		"    ignored-bin",
	}

	// When
	got := parseCargoInstallListEntries(lines)

	// Then
	want := []cargoEntry{{name: "ripgrep", version: "14.1.1"}, {name: "cargo-edit", version: "0.13.0"}}
	if !slices.Equal(got, want) {
		t.Errorf("parseCargoInstallListEntries = %v, want %v", got, want)
	}
}

func TestCargo_ParseInstallListEntries_whenOutputEmptyOrMalformed(t *testing.T) {
	// Given
	lines := []string{"", "    bin-only", "not a crate header", "also-not:"}

	// When
	got := parseCargoInstallListEntries(lines)

	// Then
	if len(got) != 0 {
		t.Errorf("parseCargoInstallListEntries malformed = %v, want empty", got)
	}
}

func TestCargo_PlanCommands_whenPackageHasVersionSuffix(t *testing.T) {
	// Given
	a := Cargo{}

	// When / Then
	if got, want := a.PlanInstall("cargo-edit@0.13.0"), []string{"cargo", "install", "cargo-edit@0.13.0"}; !slices.Equal(got, want) {
		t.Errorf("PlanInstall = %v, want %v", got, want)
	}
	if got, want := a.PlanUninstall("cargo-edit@0.13.0"), []string{"cargo", "uninstall", "cargo-edit"}; !slices.Equal(got, want) {
		t.Errorf("PlanUninstall = %v, want %v", got, want)
	}
	if got, want := a.PlanUpgrade("cargo-edit@0.13.0"), []string{"cargo", "install", "cargo-edit@0.13.0"}; !slices.Equal(got, want) {
		t.Errorf("PlanUpgrade = %v, want %v", got, want)
	}
	if cmds := a.PlanClean(); cmds != nil {
		t.Errorf("PlanClean = %v, want nil", cmds)
	}
}

func TestCargo_ListInstalledAndVersions_whenCargoOutputsInstalledCrates(t *testing.T) {
	// Given
	installFakeBinary(t, "cargo",
		`if [ "$1" = "install" ] && [ "$2" = "--list" ]; then
  echo "ripgrep v14.1.1:"
  echo "    rg"
  echo "cargo-edit v0.13.0:"
  echo "    cargo-add"
  echo "    cargo-rm"
fi`)

	// When
	pkgs, err := Cargo{}.ListInstalled()

	// Then
	if err != nil {
		t.Fatalf("Cargo.ListInstalled: %v", err)
	}
	if want := []string{"ripgrep", "cargo-edit"}; !slices.Equal(pkgs, want) {
		t.Errorf("ListInstalled = %v, want %v", pkgs, want)
	}
	version, err := Cargo{}.QueryVersion("cargo-edit@0.13.0")
	if err != nil {
		t.Fatalf("Cargo.QueryVersion: %v", err)
	}
	if version != "0.13.0" {
		t.Errorf("QueryVersion = %q, want %q", version, "0.13.0")
	}
	versions, err := Cargo{}.ListInstalledVersions()
	if err != nil {
		t.Fatalf("Cargo.ListInstalledVersions: %v", err)
	}
	if want := map[string]string{"ripgrep": "14.1.1", "cargo-edit": "0.13.0"}; !maps.Equal(versions, want) {
		t.Errorf("ListInstalledVersions = %v, want %v", versions, want)
	}
}

func TestCargo_Query_whenPackageHasVersionSuffix(t *testing.T) {
	// Given
	installFakeBinary(t, "cargo",
		`if [ "$1" = "install" ] && [ "$2" = "--list" ]; then
  echo "cargo-edit v0.13.0:"
  echo "    cargo-add"
fi`)

	// When
	ok, err := Cargo{}.Query("cargo-edit@0.13.0")

	// Then
	if err != nil {
		t.Fatalf("Cargo.Query: %v", err)
	}
	if !ok {
		t.Error("Cargo.Query = false, want true")
	}
}
