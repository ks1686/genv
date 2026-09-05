package adapter

import (
	"maps"
	"os"
	"slices"
	"testing"
)

func TestParseUvToolList(t *testing.T) {
	tests := []struct {
		name  string
		lines []string
		want  []uvEntry
	}{
		{
			name: "headers with multiple dash entrypoints",
			lines: []string{
				"ruff v0.6.9",
				"- ruff",
				"httpie v3.2.4",
				"- http",
				"- https",
				"- httpie",
			},
			want: []uvEntry{{name: "ruff", version: "0.6.9"}, {name: "httpie", version: "3.2.4"}},
		},
		{
			name: "indented entrypoints under a header",
			lines: []string{
				"black v24.10.0",
				"  black",
				"  blackd",
			},
			want: []uvEntry{{name: "black", version: "24.10.0"}},
		},
		{
			name: "mixed dash and indented entrypoints",
			lines: []string{
				"ruff v0.11.2",
				"- ruff",
				"black v24.10.0",
				"  black",
				"  blackd",
			},
			want: []uvEntry{{name: "ruff", version: "0.11.2"}, {name: "black", version: "24.10.0"}},
		},
		{
			name: "skips lone dash and dash-prefixed noise",
			lines: []string{
				"-",
				"- ruff",
				"ruff v0.6.9",
				"- ruff",
			},
			want: []uvEntry{{name: "ruff", version: "0.6.9"}},
		},
		{
			name: "skips lines that are not name-v-version headers",
			lines: []string{
				"black 24.10.0",
				"note: something",
				"ruff v0.6.9",
			},
			want: []uvEntry{{name: "ruff", version: "0.6.9"}},
		},
		{
			name:  "empty input",
			lines: []string{"", "   "},
			want:  nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseUvToolList(tt.lines)
			if !slices.Equal(got, tt.want) {
				t.Errorf("parseUvToolList = %#v, want %#v", got, tt.want)
			}
			for _, entry := range got {
				if entry.name == "-" {
					t.Error("parseUvToolList proposed '-' from an entrypoint line")
				}
			}
		})
	}
}

// TestUv_ListInstalled_ParsesToolsAndEntrypoints verifies that ListInstalled
// extracts tool names from top-level lines and skips indented entrypoint lines.
func TestUv_ListInstalled_ParsesToolsAndEntrypoints(t *testing.T) {
	installFakeBinary(t, "uv",
		`if [ "$1" = "tool" ] && [ "$2" = "list" ]; then
  echo "ruff v0.6.9"
  echo "- ruff"
  echo "black v24.10.0"
  echo "- black"
  echo "- blackd"
fi`)
	pkgs, err := Uv{}.ListInstalled()
	if err != nil {
		t.Fatalf("Uv.ListInstalled: %v", err)
	}
	want := []string{"ruff", "black"}
	if len(pkgs) != len(want) {
		t.Fatalf("got %v, want %v", pkgs, want)
	}
	for i, w := range want {
		if pkgs[i] != w {
			t.Errorf("[%d]: got %q, want %q", i, pkgs[i], w)
		}
	}
}

// TestUv_ListInstalled_EmptyOutput verifies that an empty "uv tool list" yields
// an empty slice, not a nil-with-error.
func TestUv_ListInstalled_EmptyOutput(t *testing.T) {
	installFakeBinary(t, "uv",
		`if [ "$1" = "tool" ] && [ "$2" = "list" ]; then
  echo ""
fi`)
	pkgs, err := Uv{}.ListInstalled()
	if err != nil {
		t.Fatalf("Uv.ListInstalled: %v", err)
	}
	if len(pkgs) != 0 {
		t.Errorf("expected empty list, got %v", pkgs)
	}
}

// TestUv_Query_MatchesBareName verifies Query returns true for an installed
// tool requested without a version specifier.
func TestUv_Query_MatchesBareName(t *testing.T) {
	installFakeBinary(t, "uv",
		`if [ "$1" = "tool" ] && [ "$2" = "list" ]; then
  echo "ruff v0.6.9"
fi`)
	ok, err := Uv{}.Query("ruff")
	if err != nil {
		t.Fatalf("Uv.Query: %v", err)
	}
	if !ok {
		t.Error("Uv.Query(ruff): expected true")
	}
}

// TestUv_Query_MatchesVersionSpecifier verifies Query tolerates a @version
// suffix by matching only the tool name portion.
func TestUv_Query_MatchesVersionSpecifier(t *testing.T) {
	installFakeBinary(t, "uv",
		`if [ "$1" = "tool" ] && [ "$2" = "list" ]; then
  echo "ruff v0.6.9"
fi`)
	ok, err := Uv{}.Query("ruff@0.6.0")
	if err != nil {
		t.Fatalf("Uv.Query: %v", err)
	}
	if !ok {
		t.Error("Uv.Query(ruff@0.6.0): expected true")
	}
}

// TestUv_Query_AbsentTool verifies Query returns false for a missing tool.
func TestUv_Query_AbsentTool(t *testing.T) {
	installFakeBinary(t, "uv",
		`if [ "$1" = "tool" ] && [ "$2" = "list" ]; then
  echo "ruff v0.6.9"
fi`)
	ok, err := Uv{}.Query("black")
	if err != nil {
		t.Fatalf("Uv.Query: %v", err)
	}
	if ok {
		t.Error("Uv.Query(black): expected false")
	}
}

// TestUv_QueryVersion_ParsesVersion verifies the version is extracted and the
// leading "v" is stripped.
func TestUv_QueryVersion_ParsesVersion(t *testing.T) {
	installFakeBinary(t, "uv",
		`if [ "$1" = "tool" ] && [ "$2" = "list" ]; then
  echo "ruff v0.6.9"
  echo "black v24.10.0"
fi`)
	ver, err := Uv{}.QueryVersion("ruff")
	if err != nil {
		t.Fatalf("Uv.QueryVersion: %v", err)
	}
	if ver != "0.6.9" {
		t.Errorf("version: got %q, want %q", ver, "0.6.9")
	}

	ver, err = Uv{}.QueryVersion("black@24.10.0")
	if err != nil {
		t.Fatalf("Uv.QueryVersion: %v", err)
	}
	if ver != "24.10.0" {
		t.Errorf("version: got %q, want %q", ver, "24.10.0")
	}
}

// TestUv_QueryVersion_AbsentTool verifies QueryVersion returns empty when the
// tool is not listed.
func TestUv_QueryVersion_AbsentTool(t *testing.T) {
	installFakeBinary(t, "uv",
		`if [ "$1" = "tool" ] && [ "$2" = "list" ]; then
  echo "ruff v0.6.9"
fi`)
	ver, err := Uv{}.QueryVersion("black")
	if err != nil {
		t.Fatalf("Uv.QueryVersion: %v", err)
	}
	if ver != "" {
		t.Errorf("version: got %q, want empty", ver)
	}
}

func TestUv_ListInstalledVersions_returnsVersionsAndExecsListOnce(t *testing.T) {
	// Given
	counterPath := t.TempDir() + "/count"
	t.Setenv("GENV_FAKE_COUNTER", counterPath)
	installFakeBinary(t, "uv",
		`if [ "$1" = "tool" ] && [ "$2" = "list" ]; then
  count=$(cat "$GENV_FAKE_COUNTER" 2>/dev/null || printf 0)
  count=$((count + 1))
  printf "%s" "$count" > "$GENV_FAKE_COUNTER"
  echo "ruff v0.6.9"
  echo "- ruff"
  echo "black v24.10.0"
  echo "- black"
fi`)

	// When
	versions, err := Uv{}.ListInstalledVersions()

	// Then
	if err != nil {
		t.Fatalf("Uv.ListInstalledVersions: %v", err)
	}
	want := map[string]string{"ruff": "0.6.9", "black": "24.10.0"}
	if !maps.Equal(versions, want) {
		t.Errorf("ListInstalledVersions = %v, want %v", versions, want)
	}
	count, err := os.ReadFile(counterPath)
	if err != nil {
		t.Fatalf("counter: %v", err)
	}
	if string(count) != "1" {
		t.Errorf("uv tool list exec count = %q, want 1", string(count))
	}
}

// TestUv_PlanInstall_IncludesSpecifier verifies PlanInstall passes the package
// name through unchanged, preserving any @version suffix.
func TestUv_PlanInstall_IncludesSpecifier(t *testing.T) {
	args := Uv{}.PlanInstall("ruff@0.6.0")
	want := []string{"uv", "tool", "install", "ruff@0.6.0"}
	if len(args) != len(want) {
		t.Fatalf("got %v, want %v", args, want)
	}
	for i, w := range want {
		if args[i] != w {
			t.Errorf("[%d]: got %q, want %q", i, args[i], w)
		}
	}
}

// TestUv_PlanUninstall_StripsSpecifier verifies PlanUninstall strips any
// @version suffix because uv tool uninstall expects a bare tool name.
func TestUv_PlanUninstall_StripsSpecifier(t *testing.T) {
	args := Uv{}.PlanUninstall("ruff@0.6.0")
	want := []string{"uv", "tool", "uninstall", "ruff"}
	if len(args) != len(want) {
		t.Fatalf("got %v, want %v", args, want)
	}
	for i, w := range want {
		if args[i] != w {
			t.Errorf("[%d]: got %q, want %q", i, args[i], w)
		}
	}
}

// TestUv_PlanUpgrade_IncludesUpgradeFlag verifies PlanUpgrade uses the
// "--upgrade" flag with "uv tool install".
func TestUv_PlanUpgrade_IncludesUpgradeFlag(t *testing.T) {
	args := Uv{}.PlanUpgrade("ruff")
	want := []string{"uv", "tool", "install", "--upgrade", "ruff"}
	if len(args) != len(want) {
		t.Fatalf("got %v, want %v", args, want)
	}
	for i, w := range want {
		if args[i] != w {
			t.Errorf("[%d]: got %q, want %q", i, args[i], w)
		}
	}
}

// TestUv_PlanClean verifies that Uv.PlanClean runs uv's global cache clean.
func TestUv_PlanClean(t *testing.T) {
	cmds := Uv{}.PlanClean()
	if len(cmds) != 1 {
		t.Fatalf("Uv.PlanClean: expected 1 command, got %v", cmds)
	}
	want := []string{"uv", "cache", "clean"}
	if len(cmds[0]) != len(want) {
		t.Fatalf("Uv.PlanClean[0]: got %v, want %v", cmds[0], want)
	}
	for i, w := range want {
		if cmds[0][i] != w {
			t.Errorf("Uv.PlanClean[0][%d] = %q, want %q", i, cmds[0][i], w)
		}
	}
}

// TestUvToolName strips @version suffixes and leaves bare names untouched.
func TestUvToolName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"ruff", "ruff"},
		{"ruff@0.6.0", "ruff"},
		{"ruff@latest", "ruff"},
		{"some-pkg@1.2.3", "some-pkg"},
	}
	for _, tc := range tests {
		got := uvToolName(tc.input)
		if got != tc.want {
			t.Errorf("uvToolName(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

// TestUv_Available uses the shared lookPath mock to confirm Uv.Available
// delegates correctly.
func TestUv_Available(t *testing.T) {
	orig := lookPath
	t.Cleanup(func() { lookPath = orig })

	lookPath = func(string) (string, error) { return "/usr/bin/uv", nil }
	a := Uv{}
	if !a.Available() {
		t.Error("Uv.Available() = false when lookPath succeeds")
	}

	lookPath = func(string) (string, error) { return "", &os.PathError{Op: "lookpath", Err: os.ErrNotExist} }
	if a.Available() {
		t.Error("Uv.Available() = true when lookPath fails")
	}
}
