package adapter

import (
	"maps"
	"slices"
	"testing"
)

// isolatePATH drops inherited PATH so a host `cursor` or `code` cannot
// shadow the fake binaries these tests install.
func isolatePATH(t *testing.T) {
	t.Helper()
	t.Setenv("PATH", t.TempDir())
}

func TestVscode_Available_whenOnlyCursorOnPATH(t *testing.T) {
	isolatePATH(t)
	installFakeBinary(t, "cursor", "")
	a := Vscode{}
	if !a.Available() {
		t.Fatal("Available() = false when only cursor is on PATH")
	}
}

func TestVscode_Available_whenOnlyCodeOnPATH(t *testing.T) {
	isolatePATH(t)
	installFakeBinary(t, "code", "")
	a := Vscode{}
	if !a.Available() {
		t.Fatal("Available() = false when only code is on PATH")
	}
}

func TestVscode_Available_whenNeitherCLIOnPATH(t *testing.T) {
	isolatePATH(t)
	a := Vscode{}
	if a.Available() {
		t.Fatal("Available() = true when neither cursor nor code is on PATH")
	}
}

func TestVscode_PlanCommands_whenExtensionTracked(t *testing.T) {
	isolatePATH(t)
	installFakeBinary(t, "code", "")
	a := Vscode{}

	if got, want := a.PlanInstall("golang.go"), []string{"code", "--install-extension", "golang.go"}; !slices.Equal(got, want) {
		t.Errorf("PlanInstall = %v, want %v", got, want)
	}
	if got, want := a.PlanUninstall("golang.go@0.42.0"), []string{"code", "--uninstall-extension", "golang.go"}; !slices.Equal(got, want) {
		t.Errorf("PlanUninstall = %v, want %v", got, want)
	}
	// Upgrade reinstalls a single id with --force and never opts into
	// pre-release. `code --install-extension --force` installs latest stable;
	// pre-release requires an explicit `--pre-release` that genv does not pass.
	if got, want := a.PlanUpgrade("golang.go"), []string{"code", "--install-extension", "golang.go", "--force"}; !slices.Equal(got, want) {
		t.Errorf("PlanUpgrade = %v, want %v", got, want)
	}
	if slices.Contains(a.PlanUpgrade("golang.go"), "--pre-release") {
		t.Error("PlanUpgrade must not pass --pre-release; that channel is not the default")
	}
}

func TestVscode_PlanCommands_whenOnlyCursorOnPATH(t *testing.T) {
	isolatePATH(t)
	installFakeBinary(t, "cursor", "")
	a := Vscode{}

	if got, want := a.PlanInstall("golang.go"), []string{"cursor", "--install-extension", "golang.go"}; !slices.Equal(got, want) {
		t.Errorf("PlanInstall = %v, want %v", got, want)
	}
	if got, want := a.PlanUninstall("golang.go@0.42.0"), []string{"cursor", "--uninstall-extension", "golang.go"}; !slices.Equal(got, want) {
		t.Errorf("PlanUninstall = %v, want %v", got, want)
	}
	if got, want := a.PlanUpgrade("golang.go"), []string{"cursor", "--install-extension", "golang.go", "--force"}; !slices.Equal(got, want) {
		t.Errorf("PlanUpgrade = %v, want %v", got, want)
	}
}

func TestVscode_PlanCommands_prefersCursorWhenBothOnPATH(t *testing.T) {
	isolatePATH(t)
	installFakeBinary(t, "code", "")
	installFakeBinary(t, "cursor", "")
	a := Vscode{}

	if got, want := a.PlanInstall("golang.go"), []string{"cursor", "--install-extension", "golang.go"}; !slices.Equal(got, want) {
		t.Errorf("PlanInstall = %v, want %v", got, want)
	}
}

func TestVscode_ParseListWithVersions_whenExtensionsInstalled(t *testing.T) {
	isolatePATH(t)
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

func TestVscode_ListInstalled_whenOnlyCursorOnPATH(t *testing.T) {
	isolatePATH(t)
	installFakeBinary(t, "cursor", `if [ "$1" = "--list-extensions" ] && [ "$2" = "--show-versions" ]; then
  echo 'golang.Go@0.42.0'
  echo 'anysphere.remote-ssh@1.1.14'
fi`)

	got, err := Vscode{}.ListInstalled()
	if err != nil {
		t.Fatalf("Vscode.ListInstalled: %v", err)
	}
	slices.Sort(got)
	want := []string{"anysphere.remote-ssh", "golang.go"}
	if !slices.Equal(got, want) {
		t.Errorf("ListInstalled = %v, want %v", got, want)
	}
}

func TestVscode_Query_whenExtensionIDIsCaseInsensitive(t *testing.T) {
	isolatePATH(t)
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
