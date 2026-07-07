package adapter

import (
	"maps"
	"os"
	"testing"
)

func TestScoop_Name(t *testing.T) {
	if got := (Scoop{}).Name(); got != "scoop" {
		t.Errorf("Name() = %q, want %q", got, "scoop")
	}
}

func TestScoop_PlanInstall(t *testing.T) {
	args := Scoop{}.PlanInstall("neovim")
	want := []string{"scoop", "install", "neovim"}
	if len(args) != len(want) {
		t.Fatalf("PlanInstall: got %v, want %v", args, want)
	}
	for i, w := range want {
		if args[i] != w {
			t.Errorf("PlanInstall[%d] = %q, want %q", i, args[i], w)
		}
	}
}

func TestScoop_PlanUninstall(t *testing.T) {
	args := Scoop{}.PlanUninstall("neovim")
	want := []string{"scoop", "uninstall", "neovim"}
	if len(args) != len(want) {
		t.Fatalf("PlanUninstall: got %v, want %v", args, want)
	}
	for i, w := range want {
		if args[i] != w {
			t.Errorf("PlanUninstall[%d] = %q, want %q", i, args[i], w)
		}
	}
}

func TestScoop_PlanClean(t *testing.T) {
	cmds := Scoop{}.PlanClean()
	if len(cmds) != 1 {
		t.Fatalf("PlanClean: expected 1 command, got %v", cmds)
	}
	want := []string{"scoop", "cache", "rm", "*"}
	if len(cmds[0]) != len(want) {
		t.Fatalf("PlanClean[0]: got %v, want %v", cmds[0], want)
	}
	for i, w := range want {
		if cmds[0][i] != w {
			t.Errorf("PlanClean[0][%d] = %q, want %q", i, cmds[0][i], w)
		}
	}
}

// Real "scoop list" output (ANSI color codes stripped here for readability;
// the parser strips them itself — see TestScoop_ListInstalled_StripsANSI).
const scoopListSample = `Installed apps:

Name        Version    Source Updated             Info
----        -------    ------ -------             ----
7zip        26.02      main   2026-06-28 21:29:03
bun         1.3.14     main   2026-06-28 21:48:26
git         2.55.0.2   main   2026-07-04 21:08:57
nodejs-lts  24.18.0    main   2026-07-01 18:39:04 `

func TestScoop_ListInstalled(t *testing.T) {
	installFakeBinary(t, "scoop", `cat <<'EOF'
`+scoopListSample+`
EOF`)
	names, err := Scoop{}.ListInstalled()
	if err != nil {
		t.Fatalf("ListInstalled: unexpected error: %v", err)
	}
	want := []string{"7zip", "bun", "git", "nodejs-lts"}
	if len(names) != len(want) {
		t.Fatalf("ListInstalled: got %v, want %v", names, want)
	}
	for i, w := range want {
		if names[i] != w {
			t.Errorf("ListInstalled[%d] = %q, want %q", i, names[i], w)
		}
	}
}

func TestScoop_ListInstalled_StripsANSI(t *testing.T) {
	// Real scoop output wraps each header cell in SGR color codes: ESC[32;1m...ESC[0m
	ansiLine := "\x1b[32;1mNames       \x1b[0m\x1b[32;1mVersion\x1b[0m"
	got := stripANSI(ansiLine)
	want := "Names       Version"
	if got != want {
		t.Errorf("stripANSI: got %q, want %q", got, want)
	}
}

func TestScoop_Query_Installed(t *testing.T) {
	installFakeBinary(t, "scoop", `cat <<'EOF'
`+scoopListSample+`
EOF`)
	ok, err := Scoop{}.Query("git")
	if err != nil {
		t.Fatalf("Query: unexpected error: %v", err)
	}
	if !ok {
		t.Error("Query: expected true for installed app")
	}
}

func TestScoop_Query_NotInstalled(t *testing.T) {
	installFakeBinary(t, "scoop", `cat <<'EOF'
`+scoopListSample+`
EOF`)
	ok, err := Scoop{}.Query("neovim")
	if err != nil {
		t.Fatalf("Query: unexpected error: %v", err)
	}
	if ok {
		t.Error("Query: expected false for app not in list")
	}
}

func TestScoop_QueryVersion(t *testing.T) {
	installFakeBinary(t, "scoop", `cat <<'EOF'
`+scoopListSample+`
EOF`)
	ver, err := Scoop{}.QueryVersion("bun")
	if err != nil {
		t.Fatalf("QueryVersion: unexpected error: %v", err)
	}
	if ver != "1.3.14" {
		t.Errorf("QueryVersion: got %q, want %q", ver, "1.3.14")
	}
}

func TestScoop_ListInstalledVersions_returnsVersionsAndExecsListOnce(t *testing.T) {
	// Given
	counterPath := t.TempDir() + "/count"
	t.Setenv("GENV_FAKE_COUNTER", counterPath)
	installFakeBinary(t, "scoop", `if [ "$1" = "list" ]; then
  count=$(cat "$GENV_FAKE_COUNTER" 2>/dev/null || printf 0)
  count=$((count + 1))
  printf "%s" "$count" > "$GENV_FAKE_COUNTER"
  cat <<'EOF'
`+scoopListSample+`
EOF
fi`)

	// When
	versions, err := Scoop{}.ListInstalledVersions()

	// Then
	if err != nil {
		t.Fatalf("ListInstalledVersions: unexpected error: %v", err)
	}
	want := map[string]string{
		"7zip":       "26.02",
		"bun":        "1.3.14",
		"git":        "2.55.0.2",
		"nodejs-lts": "24.18.0",
	}
	if !maps.Equal(versions, want) {
		t.Errorf("ListInstalledVersions: got %v, want %v", versions, want)
	}
	count, err := os.ReadFile(counterPath)
	if err != nil {
		t.Fatalf("counter: %v", err)
	}
	if string(count) != "1" {
		t.Errorf("scoop list exec count = %q, want 1", string(count))
	}
}

func TestScoop_Available(t *testing.T) {
	orig := lookPath
	t.Cleanup(func() { lookPath = orig })

	lookPath = func(name string) (string, error) {
		if name == "scoop" {
			return `C:\Users\ksmir\scoop\shims\scoop.exe`, nil
		}
		return "", &os.PathError{Op: "lookpath", Err: os.ErrNotExist}
	}
	if !(Scoop{}).Available() {
		t.Error("Available() = false when lookPath finds scoop")
	}

	lookPath = func(string) (string, error) {
		return "", &os.PathError{Op: "lookpath", Err: os.ErrNotExist}
	}
	if (Scoop{}).Available() {
		t.Error("Available() = true when lookPath fails")
	}
}
