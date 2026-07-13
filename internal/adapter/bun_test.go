package adapter

import (
	"os"
	"slices"
	"testing"
)

func TestBun_Name(t *testing.T) {
	if got := (Bun{}).Name(); got != "bun" {
		t.Errorf("Bun.Name() = %q, want %q", got, "bun")
	}
}

func TestBun_Available(t *testing.T) {
	orig := lookPath
	t.Cleanup(func() { lookPath = orig })

	lookPath = func(string) (string, error) { return "/opt/homebrew/bin/bun", nil }
	if !(Bun{}).Available() {
		t.Error("Bun.Available() = false when lookPath succeeds")
	}

	lookPath = func(string) (string, error) { return "", &os.PathError{Op: "lookpath", Err: os.ErrNotExist} }
	if (Bun{}).Available() {
		t.Error("Bun.Available() = true when lookPath fails")
	}
}

func TestBun_PlanInstall(t *testing.T) {
	a := Bun{}
	if got, want := a.PlanInstall("cf"), []string{"bun", "add", "--global", "cf"}; !slices.Equal(got, want) {
		t.Errorf("PlanInstall(cf) = %v, want %v", got, want)
	}
	if got, want := a.PlanInstall("cf@latest"), []string{"bun", "add", "--global", "cf@latest"}; !slices.Equal(got, want) {
		t.Errorf("PlanInstall(cf@latest) = %v, want %v", got, want)
	}
}

func TestBun_PlanUninstall(t *testing.T) {
	a := Bun{}
	if got, want := a.PlanUninstall("cf"), []string{"bun", "remove", "--global", "cf"}; !slices.Equal(got, want) {
		t.Errorf("PlanUninstall(cf) = %v, want %v", got, want)
	}
	if got, want := a.PlanUninstall("cf@latest"), []string{"bun", "remove", "--global", "cf"}; !slices.Equal(got, want) {
		t.Errorf("PlanUninstall(cf@latest) = %v, want %v", got, want)
	}
}

func TestBun_PlanUpgrade(t *testing.T) {
	a := Bun{}
	if got, want := a.PlanUpgrade("cf"), []string{"bun", "add", "--global", "cf"}; !slices.Equal(got, want) {
		t.Errorf("PlanUpgrade(cf) = %v, want %v", got, want)
	}
	if got, want := a.PlanUpgrade("cf@latest"), []string{"bun", "add", "--global", "cf"}; !slices.Equal(got, want) {
		t.Errorf("PlanUpgrade(cf@latest) = %v, want %v", got, want)
	}
}

func TestBun_PlanClean(t *testing.T) {
	a := Bun{}
	cmds := a.PlanClean()
	if len(cmds) != 1 {
		t.Fatalf("PlanClean: expected 1 command, got %v", cmds)
	}
	if want := []string{"bun", "pm", "cache", "rm", "--global"}; !slices.Equal(cmds[0], want) {
		t.Errorf("PlanClean[0] = %v, want %v", cmds[0], want)
	}
}

func TestBunBaseName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"cf", "cf"},
		{"cf@latest", "cf"},
		{"cf@1.2.3", "cf"},
		{"@scope/pkg", "@scope/pkg"},
		{"@scope/pkg@1.0.0", "@scope/pkg"},
		{"@scope/pkg@latest", "@scope/pkg"},
	}
	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			if got := bunBaseName(tc.input); got != tc.want {
				t.Errorf("bunBaseName(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestParseBunListLine(t *testing.T) {
	tests := []struct {
		line     string
		wantName string
		wantVer  string
		wantOK   bool
	}{
		{"├── cf@0.0.6", "cf", "0.0.6", true},
		{"└── cf@0.0.6", "cf", "0.0.6", true},
		{"├── add-gitignore@1.1.1", "add-gitignore", "1.1.1", true},
		{"├── @colbymchenry/codegraph@1.0.1", "@colbymchenry/codegraph", "1.0.1", true},
		{"  ├── indented@2.0.0", "indented", "2.0.0", true},
		{"├── unversioned", "unversioned", "", true},
		{"/path/to/global node_modules (164)", "", "", false},
		{"", "", "", false},
		{"   ", "", "", false},
	}
	for _, tc := range tests {
		t.Run(tc.line, func(t *testing.T) {
			gotName, gotVer, gotOK := parseBunListLine(tc.line)
			if gotName != tc.wantName || gotVer != tc.wantVer || gotOK != tc.wantOK {
				t.Errorf("parseBunListLine(%q) = (%q, %q, %v), want (%q, %q, %v)",
					tc.line, gotName, gotVer, gotOK, tc.wantName, tc.wantVer, tc.wantOK)
			}
		})
	}
}

func TestParseBunEntries(t *testing.T) {
	lines := []string{
		"/Users/ks1686/.cache/.bun/install/global node_modules (164)",
		"├── @colbymchenry/codegraph@1.0.1",
		"├── add-gitignore@1.1.1",
		"├── ajv@8.20.0",
		"├── ajv-formats@3.0.1",
		"└── cf@0.0.6",
	}
	got := parseBunEntries(lines)
	want := []bunEntry{{"@colbymchenry/codegraph", "1.0.1"}, {"add-gitignore", "1.1.1"}, {"ajv", "8.20.0"}, {"ajv-formats", "3.0.1"}, {"cf", "0.0.6"}}
	if !slices.Equal(got, want) {
		t.Errorf("parseBunEntries = %v, want %v", got, want)
	}
}

func TestParseBunEntries_Empty(t *testing.T) {
	lines := []string{
		"/Users/ks1686/.cache/.bun/install/global node_modules (0)",
	}
	got := parseBunEntries(lines)
	if len(got) != 0 {
		t.Errorf("parseBunEntries(empty) = %v, want empty", got)
	}
}

func TestBun_ListInstalled_ParsesOutput(t *testing.T) {
	installFakeBinary(t, "bun",
		`if [ "$1" = "pm" ] && [ "$2" = "ls" ] && [ "$3" = "--global" ]; then
  echo "/Users/ks1686/.cache/.bun/install/global node_modules (2)"
  echo "├── add-gitignore@1.1.1"
  echo "└── cf@0.0.6"
fi`)
	pkgs, err := Bun{}.ListInstalled()
	if err != nil {
		t.Fatalf("Bun.ListInstalled: %v", err)
	}
	want := []string{"add-gitignore", "cf"}
	if !slices.Equal(pkgs, want) {
		t.Errorf("ListInstalled = %v, want %v", pkgs, want)
	}
}

func TestBun_Query(t *testing.T) {
	installFakeBinary(t, "bun",
		`if [ "$1" = "pm" ] && [ "$2" = "ls" ] && [ "$3" = "--global" ]; then
  echo "/path/global node_modules (2)"
  echo "├── add-gitignore@1.1.1"
  echo "└── cf@0.0.6"
fi`)
	a := Bun{}

	ok, err := a.Query("cf")
	if err != nil {
		t.Fatalf("Query(cf): %v", err)
	}
	if !ok {
		t.Error("Query(cf) = false, want true")
	}

	ok, err = a.Query("cf@latest")
	if err != nil {
		t.Fatalf("Query(cf@latest): %v", err)
	}
	if !ok {
		t.Error("Query(cf@latest) = false, want true")
	}

	ok, err = a.Query("missing")
	if err != nil {
		t.Fatalf("Query(missing): %v", err)
	}
	if ok {
		t.Error("Query(missing) = true, want false")
	}
}

func TestBun_QueryVersion(t *testing.T) {
	installFakeBinary(t, "bun",
		`if [ "$1" = "pm" ] && [ "$2" = "ls" ] && [ "$3" = "--global" ]; then
  echo "/path/global node_modules (2)"
  echo "├── add-gitignore@1.1.1"
  echo "└── cf@0.0.6"
fi`)
	a := Bun{}

	ver, err := a.QueryVersion("cf")
	if err != nil {
		t.Fatalf("QueryVersion(cf): %v", err)
	}
	if ver != "0.0.6" {
		t.Errorf("QueryVersion(cf) = %q, want %q", ver, "0.0.6")
	}

	ver, err = a.QueryVersion("cf@latest")
	if err != nil {
		t.Fatalf("QueryVersion(cf@latest): %v", err)
	}
	if ver != "0.0.6" {
		t.Errorf("QueryVersion(cf@latest) = %q, want %q", ver, "0.0.6")
	}

	ver, err = a.QueryVersion("missing")
	if err != nil {
		t.Fatalf("QueryVersion(missing): %v", err)
	}
	if ver != "" {
		t.Errorf("QueryVersion(missing) = %q, want empty", ver)
	}
}

func TestBun_QueryVersion_Scoped(t *testing.T) {
	installFakeBinary(t, "bun",
		`if [ "$1" = "pm" ] && [ "$2" = "ls" ] && [ "$3" = "--global" ]; then
  echo "/path/global node_modules (1)"
  echo "└── @colbymchenry/codegraph@1.0.1"
fi`)
	ver, err := Bun{}.QueryVersion("@colbymchenry/codegraph")
	if err != nil {
		t.Fatalf("QueryVersion: %v", err)
	}
	if ver != "1.0.1" {
		t.Errorf("QueryVersion = %q, want %q", ver, "1.0.1")
	}
}

// TestBun_Real_IfAvailable exercises the adapter against the real bun binary
// when it is present on the test host. It skips cleanly when bun is absent.
func TestBun_Real_IfAvailable(t *testing.T) {
	a := Bun{}
	if !a.Available() {
		t.Skip("bun not available on this host")
	}

	installed, err := a.ListInstalled()
	if err != nil {
		t.Fatalf("ListInstalled: %v", err)
	}
	if len(installed) == 0 {
		t.Skip("no global bun packages installed")
	}

	pkg := installed[0]
	ok, err := a.Query(pkg)
	if err != nil {
		t.Fatalf("Query(%q): %v", pkg, err)
	}
	if !ok {
		t.Errorf("Query(%q) = false for package returned by ListInstalled", pkg)
	}

	ver, err := a.QueryVersion(pkg)
	if err != nil {
		t.Fatalf("QueryVersion(%q): %v", pkg, err)
	}
	if ver == "" {
		t.Errorf("QueryVersion(%q) returned empty version", pkg)
	}
}
