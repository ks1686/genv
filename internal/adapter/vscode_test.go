package adapter

import (
	"maps"
	"slices"
	"testing"
)

func TestVscode_PlanCommands_whenExtensionTracked(t *testing.T) {
	a := Vscode{}

	if got, want := a.PlanInstall("golang.go"), []string{"code", "--install-extension", "golang.go"}; !slices.Equal(got, want) {
		t.Errorf("PlanInstall = %v, want %v", got, want)
	}
	if got, want := a.PlanUninstall("golang.go@0.42.0"), []string{"code", "--uninstall-extension", "golang.go"}; !slices.Equal(got, want) {
		t.Errorf("PlanUninstall = %v, want %v", got, want)
	}
	// Upgrade must reinstall a single id with --force, never a broad update.
	if got, want := a.PlanUpgrade("golang.go"), []string{"code", "--install-extension", "golang.go", "--force"}; !slices.Equal(got, want) {
		t.Errorf("PlanUpgrade = %v, want %v", got, want)
	}
}

func TestVscode_ParseListWithVersions_whenExtensionsInstalled(t *testing.T) {
	installFakeBinary(t, "code", `if [ "$1" = "--list-extensions" ] && [ "$2" = "--show-versions" ]; then
  echo 'golang.Go@0.42.0'
  echo 'ms-python.python@2024.4.0'
fi`)

	versions, err := Vscode{}.ListInstalledVersions()
	if err != nil {
		t.Fatalf("Vscode.ListInstalledVersions: %v", err)
	}
	want := map[string]string{"golang.go": "0.42.0", "ms-python.python": "2024.4.0"}
	if !maps.Equal(versions, want) {
		t.Errorf("ListInstalledVersions = %v, want %v", versions, want)
	}
}

func TestVscode_Query_whenExtensionIDIsCaseInsensitive(t *testing.T) {
	installFakeBinary(t, "code", `if [ "$1" = "--list-extensions" ]; then
  echo 'golang.Go@0.42.0'
fi`)

	// Tracked id uses different casing than the installed listing.
	ok, err := Vscode{}.Query("Golang.go")
	if err != nil {
		t.Fatalf("Vscode.Query: %v", err)
	}
	if !ok {
		t.Error("Vscode.Query = false, want true (ids are case-insensitive)")
	}
	version, err := Vscode{}.QueryVersion("golang.GO")
	if err != nil {
		t.Fatalf("Vscode.QueryVersion: %v", err)
	}
	if version != "0.42.0" {
		t.Errorf("QueryVersion = %q, want 0.42.0", version)
	}
}
