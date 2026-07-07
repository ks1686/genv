package adapter

import (
	"maps"
	"os"
	"testing"
)

func TestChoco_Name(t *testing.T) {
	if got := (Choco{}).Name(); got != "choco" {
		t.Errorf("Name() = %q, want %q", got, "choco")
	}
}

func TestChoco_PlanInstall(t *testing.T) {
	args := Choco{}.PlanInstall("neovim")
	want := []string{"choco", "install", "-y", "neovim"}
	if len(args) != len(want) {
		t.Fatalf("PlanInstall: got %v, want %v", args, want)
	}
	for i, w := range want {
		if args[i] != w {
			t.Errorf("PlanInstall[%d] = %q, want %q", i, args[i], w)
		}
	}
}

func TestChoco_PlanUninstall(t *testing.T) {
	args := Choco{}.PlanUninstall("neovim")
	want := []string{"choco", "uninstall", "-y", "neovim"}
	if len(args) != len(want) {
		t.Fatalf("PlanUninstall: got %v, want %v", args, want)
	}
	for i, w := range want {
		if args[i] != w {
			t.Errorf("PlanUninstall[%d] = %q, want %q", i, args[i], w)
		}
	}
}

func TestChoco_PlanClean(t *testing.T) {
	cmds := Choco{}.PlanClean()
	if len(cmds) != 1 {
		t.Fatalf("PlanClean: expected 1 command, got %v", cmds)
	}
	want := []string{"choco", "cache", "remove", "--all"}
	if len(cmds[0]) != len(want) {
		t.Fatalf("PlanClean[0]: got %v, want %v", cmds[0], want)
	}
	for i, w := range want {
		if cmds[0][i] != w {
			t.Errorf("PlanClean[0][%d] = %q, want %q", i, cmds[0][i], w)
		}
	}
}

// Real "choco list" output on Chocolatey CLI v2+ (bare "list" now shows
// locally installed packages; --local-only was removed).
const chocoListSample = `Chocolatey v2.7.3
chocolatey 2.7.3
chocolatey-core.extension 1.4.0
codex 0.142.5
dotnet-8.0-desktopruntime 8.0.28`

func TestChoco_ListInstalled(t *testing.T) {
	installFakeBinary(t, "choco", `cat <<'EOF'
`+chocoListSample+`
EOF`)
	names, err := Choco{}.ListInstalled()
	if err != nil {
		t.Fatalf("ListInstalled: unexpected error: %v", err)
	}
	want := []string{"chocolatey", "chocolatey-core.extension", "codex", "dotnet-8.0-desktopruntime"}
	if len(names) != len(want) {
		t.Fatalf("ListInstalled: got %v, want %v", names, want)
	}
	for i, w := range want {
		if names[i] != w {
			t.Errorf("ListInstalled[%d] = %q, want %q", i, names[i], w)
		}
	}
}

func TestChoco_ListInstalled_SkipsBannerAndSummary(t *testing.T) {
	installFakeBinary(t, "choco", `cat <<'EOF'
Chocolatey v2.7.3
codex 0.142.5
2 packages installed.
EOF`)
	names, err := Choco{}.ListInstalled()
	if err != nil {
		t.Fatalf("ListInstalled: unexpected error: %v", err)
	}
	if len(names) != 1 || names[0] != "codex" {
		t.Errorf("ListInstalled: got %v, want [codex]", names)
	}
}

func TestChoco_Query_Installed(t *testing.T) {
	installFakeBinary(t, "choco", `cat <<'EOF'
`+chocoListSample+`
EOF`)
	ok, err := Choco{}.Query("codex")
	if err != nil {
		t.Fatalf("Query: unexpected error: %v", err)
	}
	if !ok {
		t.Error("Query: expected true for installed package")
	}
}

func TestChoco_Query_NotInstalled(t *testing.T) {
	installFakeBinary(t, "choco", `cat <<'EOF'
`+chocoListSample+`
EOF`)
	ok, err := Choco{}.Query("neovim")
	if err != nil {
		t.Fatalf("Query: unexpected error: %v", err)
	}
	if ok {
		t.Error("Query: expected false for package not in list")
	}
}

func TestChoco_QueryVersion(t *testing.T) {
	installFakeBinary(t, "choco", `cat <<'EOF'
`+chocoListSample+`
EOF`)
	ver, err := Choco{}.QueryVersion("codex")
	if err != nil {
		t.Fatalf("QueryVersion: unexpected error: %v", err)
	}
	if ver != "0.142.5" {
		t.Errorf("QueryVersion: got %q, want %q", ver, "0.142.5")
	}
}

func TestChoco_ListInstalledVersions_returnsVersionsAndExecsListOnce(t *testing.T) {
	// Given
	counterPath := t.TempDir() + "/count"
	t.Setenv("GENV_FAKE_COUNTER", counterPath)
	installFakeBinary(t, "choco", `if [ "$1" = "list" ]; then
  count=$(cat "$GENV_FAKE_COUNTER" 2>/dev/null || printf 0)
  count=$((count + 1))
  printf "%s" "$count" > "$GENV_FAKE_COUNTER"
  cat <<'EOF'
`+chocoListSample+`
EOF
fi`)

	// When
	versions, err := Choco{}.ListInstalledVersions()

	// Then
	if err != nil {
		t.Fatalf("ListInstalledVersions: unexpected error: %v", err)
	}
	want := map[string]string{
		"chocolatey":                "2.7.3",
		"chocolatey-core.extension": "1.4.0",
		"codex":                     "0.142.5",
		"dotnet-8.0-desktopruntime": "8.0.28",
	}
	if !maps.Equal(versions, want) {
		t.Errorf("ListInstalledVersions: got %v, want %v", versions, want)
	}
	count, err := os.ReadFile(counterPath)
	if err != nil {
		t.Fatalf("counter: %v", err)
	}
	if string(count) != "1" {
		t.Errorf("choco list exec count = %q, want 1", string(count))
	}
}

func TestChoco_Available(t *testing.T) {
	orig := lookPath
	t.Cleanup(func() { lookPath = orig })

	lookPath = func(name string) (string, error) {
		if name == "choco" {
			return `C:\ProgramData\chocolatey\bin\choco.exe`, nil
		}
		return "", &os.PathError{Op: "lookpath", Err: os.ErrNotExist}
	}
	if !(Choco{}).Available() {
		t.Error("Available() = false when lookPath finds choco")
	}

	lookPath = func(string) (string, error) {
		return "", &os.PathError{Op: "lookpath", Err: os.ErrNotExist}
	}
	if (Choco{}).Available() {
		t.Error("Available() = true when lookPath fails")
	}
}
