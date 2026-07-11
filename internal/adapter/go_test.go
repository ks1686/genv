package adapter

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestGoInstallSpec_whenModuleHasNoVersion(t *testing.T) {
	// Given
	module := "github.com/rakyll/hey"

	// When
	got := goInstallSpec(module)

	// Then
	if got != "github.com/rakyll/hey@latest" {
		t.Errorf("goInstallSpec = %q, want %q", got, "github.com/rakyll/hey@latest")
	}
}

func TestGoInstallSpec_whenModuleHasVersion(t *testing.T) {
	// Given
	module := "github.com/rakyll/hey@v0.1.4"

	// When
	got := goInstallSpec(module)

	// Then
	if got != module {
		t.Errorf("goInstallSpec = %q, want %q", got, module)
	}
}

func TestGoBinaryBaseName_whenModulePathHasSemanticSuffix(t *testing.T) {
	// Given
	tests := []struct {
		name string
		in   string
		want string
		ok   bool
	}{
		{name: "full module", in: "github.com/rakyll/hey", want: "hey", ok: true},
		{name: "with version", in: "github.com/rakyll/hey@v0.1.4", want: "hey", ok: true},
		{name: "semantic import suffix", in: "example.com/acme/tool/v2", want: "tool", ok: true},
		{name: "trailing slash", in: "github.com/rakyll/hey/", want: "hey", ok: true},
		{name: "empty", in: "", ok: false},
		{name: "only slash", in: "/", ok: false},
		{name: "path traversal", in: "github.com/acme/../evil", ok: false},
		{name: "dot segment", in: "github.com/acme/./evil", ok: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// When
			got, ok := goBinaryBaseName(tt.in)

			// Then
			if got != tt.want || ok != tt.ok {
				t.Errorf("goBinaryBaseName(%q) = (%q, %v), want (%q, %v)", tt.in, got, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestParseGoEnvBinDir_whenGOBINWins(t *testing.T) {
	// Given
	goBin := filepath.Join(t.TempDir(), "go-bin")
	goPath := filepath.Join(t.TempDir(), "go-path")

	// When
	got, ok := parseGoEnvBinDir(goBin + "\n" + goPath + "\n")

	// Then
	if !ok || got != goBin {
		t.Errorf("parseGoEnvBinDir GOBIN = (%q, %v), want (%q, true)", got, ok, goBin)
	}
}

func TestParseGoEnvBinDir_whenGOPATHFallbackUsesFirstElement(t *testing.T) {
	// Given
	first := filepath.Join(t.TempDir(), "first")
	second := filepath.Join(t.TempDir(), "second")
	goPath := first + string(os.PathListSeparator) + second

	// When
	got, ok := parseGoEnvBinDir("\n" + goPath + "\n")

	// Then
	want := filepath.Join(first, "bin")
	if !ok || got != want {
		t.Errorf("parseGoEnvBinDir GOPATH = (%q, %v), want (%q, true)", got, ok, want)
	}
}

func TestParseGoEnvBinDir_whenValuesMissing(t *testing.T) {
	// When
	got, ok := parseGoEnvBinDir("\n\n")

	// Then
	if ok || got != "" {
		t.Errorf("parseGoEnvBinDir missing = (%q, %v), want empty false", got, ok)
	}
}

func TestGo_Query_whenFakeGoEnvResolvesBinDir(t *testing.T) {
	// Given
	binDir := t.TempDir()
	installFakeBinary(t, "go", `if [ "$1" = "env" ] && [ "$2" = "GOBIN" ] && [ "$3" = "GOPATH" ]; then
  printf '%s\n' "$GENV_TEST_GOBIN"
  printf '%s\n' "$GENV_TEST_GOPATH"
fi`)
	t.Setenv("GENV_TEST_GOBIN", binDir)
	t.Setenv("GENV_TEST_GOPATH", "")
	writeTestFile(t, filepath.Join(binDir, "hey"))

	// When
	ok, err := Go{}.Query("github.com/rakyll/hey@latest")

	// Then
	if err != nil {
		t.Fatalf("Go.Query: %v", err)
	}
	if !ok {
		t.Error("Go.Query = false, want true")
	}
}

func TestGo_Query_whenGoEnvFails(t *testing.T) {
	// Given
	installFakeBinary(t, "go", `exit 1`)

	// When
	ok, err := Go{}.Query("github.com/rakyll/hey")

	// Then
	if err != nil {
		t.Fatalf("Go.Query failed env: %v", err)
	}
	if ok {
		t.Error("Go.Query failed env = true, want false")
	}
}

func TestGoUninstallCommand_whenPathIsInsideGoBin(t *testing.T) {
	// Given
	binDir := t.TempDir()

	// When
	got := goUninstallCommand(binDir, "github.com/rakyll/hey@latest")

	// Then
	want := []string{"rm", "-f", filepath.Join(binDir, "hey")}
	if !slices.Equal(got, want) {
		t.Errorf("goUninstallCommand = %v, want %v", got, want)
	}
}

func TestGoUninstallCommand_whenModuleBaseNameUnsafe(t *testing.T) {
	// Given
	binDir := t.TempDir()
	unsafeSpecs := []string{"", "/", "github.com/acme/../evil", "github.com/acme/./evil"}

	for _, spec := range unsafeSpecs {
		t.Run(spec, func(t *testing.T) {
			// When
			got := goUninstallCommand(binDir, spec)

			// Then
			if !isFailingGoUninstallCommand(got) {
				t.Errorf("goUninstallCommand(%q) = %v, want failing non-rm command", spec, got)
			}
		})
	}
}

func TestGoUninstallCommand_whenBinDirUnsafe(t *testing.T) {
	// When
	got := goUninstallCommand("relative/bin", "github.com/rakyll/hey")

	// Then
	if !isFailingGoUninstallCommand(got) {
		t.Errorf("goUninstallCommand unsafe bin dir = %v, want failing non-rm command", got)
	}
}

func TestGoUninstallCommand_whenUnsafeInputFailsExplicitly(t *testing.T) {
	// When
	got := goUninstallCommand(t.TempDir(), "")

	// Then
	if !isFailingGoUninstallCommand(got) {
		t.Fatalf("goUninstallCommand unsafe = %v, want failing shell command", got)
	}
	if !slices.Contains(got, "") {
		t.Errorf("goUninstallCommand unsafe = %v, want original package argument preserved", got)
	}
}

func TestGo_PlanCommands_whenModuleHasVersionSuffix(t *testing.T) {
	// Given
	a := Go{}

	// When / Then
	if got, want := a.PlanInstall("github.com/rakyll/hey"), []string{"go", "install", "github.com/rakyll/hey@latest"}; !slices.Equal(got, want) {
		t.Errorf("PlanInstall = %v, want %v", got, want)
	}
	if got, want := a.PlanUpgrade("github.com/rakyll/hey@v0.1.4"), []string{"go", "install", "github.com/rakyll/hey@v0.1.4"}; !slices.Equal(got, want) {
		t.Errorf("PlanUpgrade = %v, want %v", got, want)
	}
	if cmds := a.PlanClean(); cmds != nil {
		t.Errorf("PlanClean = %v, want nil", cmds)
	}
}

func TestGo_ListInstalledAndQueryVersion_areConservative(t *testing.T) {
	// When
	list, err := Go{}.ListInstalled()
	if err != nil {
		t.Fatalf("Go.ListInstalled: %v", err)
	}
	version, err := Go{}.QueryVersion("github.com/rakyll/hey")
	if err != nil {
		t.Fatalf("Go.QueryVersion: %v", err)
	}

	// Then
	if list != nil {
		t.Errorf("Go.ListInstalled = %v, want nil", list)
	}
	if version != "" {
		t.Errorf("Go.QueryVersion = %q, want empty", version)
	}
}

func writeTestFile(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("test"), 0o755); err != nil {
		t.Fatalf("writeTestFile(%q): %v", path, err)
	}
}

func isFailingGoUninstallCommand(args []string) bool {
	return len(args) >= 4 && args[0] == "sh" && args[1] == "-c" && strings.Contains(args[2], "exit 1")
}
