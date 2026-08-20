package adapter

import (
	"os"
	"testing"
)

func TestWinget_Name(t *testing.T) {
	if got := (Winget{}).Name(); got != "winget" {
		t.Errorf("Name() = %q, want %q", got, "winget")
	}
}

func TestWinget_PlanInstall(t *testing.T) {
	args := Winget{}.PlanInstall("Neovim.Neovim")
	want := []string{"winget", "install", "--exact", "--silent", "--disable-interactivity", "--no-upgrade", "--accept-package-agreements", "--accept-source-agreements", "--id", "Neovim.Neovim"}
	if len(args) != len(want) {
		t.Fatalf("PlanInstall: got %v, want %v", args, want)
	}
	for i, w := range want {
		if args[i] != w {
			t.Errorf("PlanInstall[%d] = %q, want %q", i, args[i], w)
		}
	}
}

func TestWinget_PlanUninstall(t *testing.T) {
	args := Winget{}.PlanUninstall("Neovim.Neovim")
	want := []string{"winget", "uninstall", "--exact", "--silent", "--disable-interactivity", "--id", "Neovim.Neovim"}
	if len(args) != len(want) {
		t.Fatalf("PlanUninstall: got %v, want %v", args, want)
	}
	for i, w := range want {
		if args[i] != w {
			t.Errorf("PlanUninstall[%d] = %q, want %q", i, args[i], w)
		}
	}
}

func TestWinget_PlanUpgrade_DisableInteractivity(t *testing.T) {
	args := Winget{}.PlanUpgrade("Neovim.Neovim")
	want := []string{"winget", "upgrade", "--exact", "--silent", "--disable-interactivity", "--accept-package-agreements", "--accept-source-agreements", "--id", "Neovim.Neovim"}
	if len(args) != len(want) {
		t.Fatalf("PlanUpgrade: got %v, want %v", args, want)
	}
	for i, w := range want {
		if args[i] != w {
			t.Errorf("PlanUpgrade[%d] = %q, want %q", i, args[i], w)
		}
	}
	for _, a := range args {
		if a == "--no-upgrade" {
			t.Fatal("PlanUpgrade must not pass --no-upgrade")
		}
	}
}

func TestWinget_PlanClean_ReturnsNil(t *testing.T) {
	if cmds := (Winget{}).PlanClean(); cmds != nil {
		t.Errorf("PlanClean: expected nil, got %v", cmds)
	}
}

// Real "winget list --id <id> --exact" output for an installed package,
// captured against a live Windows host:
//
//	Name   Id            Version Source
//	------------------------------------
//	Neovim Neovim.Neovim 0.12.3  winget
const wingetListInstalledSample = `Name   Id            Version Source
------------------------------------
Neovim Neovim.Neovim 0.12.3  winget`

func TestWinget_Query_Installed(t *testing.T) {
	installFakeBinary(t, "winget",
		`if [ "$1" = "list" ] && [ "$3" = "Neovim.Neovim" ]; then
	exit 0
fi
exit 1`)
	ok, err := Winget{}.Query("Neovim.Neovim")
	if err != nil {
		t.Fatalf("Query: unexpected error: %v", err)
	}
	if !ok {
		t.Error("Query: expected true for installed package")
	}
}

func TestWinget_Query_NotInstalled(t *testing.T) {
	// Real winget exits 20 ("No installed package found...") when absent.
	installFakeBinary(t, "winget", `exit 20`)
	ok, err := Winget{}.Query("Nonexistent.Package")
	if err != nil {
		t.Fatalf("Query: unexpected error: %v", err)
	}
	if ok {
		t.Error("Query: expected false for missing package")
	}
}

func TestWinget_QueryVersion(t *testing.T) {
	installFakeBinary(t, "winget", `cat <<'EOF'
`+wingetListInstalledSample+`
EOF`)
	ver, err := Winget{}.QueryVersion("Neovim.Neovim")
	if err != nil {
		t.Fatalf("QueryVersion: unexpected error: %v", err)
	}
	if ver != "0.12.3" {
		t.Errorf("QueryVersion: got %q, want %q", ver, "0.12.3")
	}
}

// Real bare "winget list" output mixes winget-manageable entries (non-empty
// Source) with registry/MSIX-discovered entries winget cannot act on (blank
// Source) — including Name values containing spaces, which is exactly why
// column-position parsing (not whitespace-splitting) is required.
const wingetListBareSample = `Name                Id                          Version        Available Source
-------------------------------------------------------------------------------
Auto Dark Mode      ArminOsaj.AutoDarkMode      11.0.0.54                winget
8th Heaven          ARP\Machine\X64\Steam App   Unknown
Neovim               Neovim.Neovim              0.12.3                   winget`

func TestWinget_ListInstalled_ExcludesUnmanagedEntries(t *testing.T) {
	installFakeBinary(t, "winget", `cat <<'EOF'
`+wingetListBareSample+`
EOF`)
	names, err := Winget{}.ListInstalled()
	if err != nil {
		t.Fatalf("ListInstalled: unexpected error: %v", err)
	}
	want := []string{"ArminOsaj.AutoDarkMode", "Neovim.Neovim"}
	if len(names) != len(want) {
		t.Fatalf("ListInstalled: got %v, want %v", names, want)
	}
	for i, w := range want {
		if names[i] != w {
			t.Errorf("ListInstalled[%d] = %q, want %q", i, names[i], w)
		}
	}
}

func TestWinget_Available(t *testing.T) {
	orig := lookPath
	t.Cleanup(func() { lookPath = orig })

	lookPath = func(name string) (string, error) {
		if name == "winget" {
			return `C:\Windows\System32\winget.exe`, nil
		}
		return "", &os.PathError{Op: "lookpath", Err: os.ErrNotExist}
	}
	if !(Winget{}).Available() {
		t.Error("Available() = false when lookPath finds winget")
	}

	lookPath = func(string) (string, error) {
		return "", &os.PathError{Op: "lookpath", Err: os.ErrNotExist}
	}
	if (Winget{}).Available() {
		t.Error("Available() = true when lookPath fails")
	}
}
