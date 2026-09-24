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
		{
			name: "headers with required version specifiers",
			lines: []string{
				"ruff v0.6.9 [required: ruff]",
				"- ruff",
				"guild-ai-cli v0.1.0 [required:  git+ssh://git@github.com/Org/guild-ai-cli.git]",
				"- guild-ai",
			},
			want: []uvEntry{
				{name: "ruff", version: "0.6.9", required: "ruff"},
				{name: "guild-ai-cli", version: "0.1.0", required: "git+ssh://git@github.com/Org/guild-ai-cli.git"},
			},
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
// extracts tool names from version headers and skips `-` entrypoint bullets.
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

func fakeUvToolListWithSpecifiers(body string) string {
	return `if [ "$1" = "tool" ] && [ "$2" = "list" ]; then
` + body + `
fi`
}

// TestUv_Query_MatchesGitSSHURL verifies a bare git+ssh spec matches the
// installed tool name uv lists, rather than the truncated git@host fragment.
func TestUv_Query_MatchesGitSSHURL(t *testing.T) {
	installFakeBinary(t, "uv", fakeUvToolListWithSpecifiers(
		`  echo "guild-ai-cli v0.1.0 [required:  git+ssh://git@github.com/GuildEducationInc/guild-ai-cli.git]"`))
	ok, err := Uv{}.Query("git+ssh://git@github.com/GuildEducationInc/guild-ai-cli.git")
	if err != nil {
		t.Fatalf("Uv.Query: %v", err)
	}
	if !ok {
		t.Error("Uv.Query(git+ssh URL): expected true")
	}
}

// TestUv_Query_MatchesGitHTTPSURL verifies git+https specs without an @user
// still match uv's listed package name instead of the full URL.
func TestUv_Query_MatchesGitHTTPSURL(t *testing.T) {
	installFakeBinary(t, "uv", fakeUvToolListWithSpecifiers(
		`  echo "tool v1.0.0 [required: git+https://github.com/Org/tool]"`))
	ok, err := Uv{}.Query("git+https://github.com/Org/tool")
	if err != nil {
		t.Fatalf("Uv.Query: %v", err)
	}
	if !ok {
		t.Error("Uv.Query(git+https URL): expected true")
	}
}

// TestUv_Query_MatchesRequiredSpecifierWhenRepoNameDiffers verifies Query
// matches via [required: ...] when the repo basename is not the package name.
func TestUv_Query_MatchesRequiredSpecifierWhenRepoNameDiffers(t *testing.T) {
	spec := "git+ssh://git@github.com/Org/repo-name.git"
	installFakeBinary(t, "uv", fakeUvToolListWithSpecifiers(
		`  echo "actual-pkg v0.2.0 [required:  `+spec+`]"`))
	ok, err := Uv{}.Query(spec)
	if err != nil {
		t.Fatalf("Uv.Query: %v", err)
	}
	if !ok {
		t.Error("Uv.Query(git URL whose repo name differs): expected true via required specifier")
	}

	ok, err = Uv{}.Query(spec + "@v1.2.0")
	if err != nil {
		t.Fatalf("Uv.Query(git URL@ref): %v", err)
	}
	if !ok {
		t.Error("Uv.Query(git URL@ref whose repo name differs): expected true via required specifier")
	}
}

// TestUv_Query_MatchesPrefixedGitURL keeps the documented workaround of
// prefixing the package name before @git+...
func TestUv_Query_MatchesPrefixedGitURL(t *testing.T) {
	installFakeBinary(t, "uv", fakeUvToolListWithSpecifiers(
		`  echo "guild-ai-cli v0.1.0 [required:  git+ssh://git@github.com/GuildEducationInc/guild-ai-cli.git]"`))
	ok, err := Uv{}.Query("guild-ai-cli@git+ssh://git@github.com/GuildEducationInc/guild-ai-cli.git")
	if err != nil {
		t.Fatalf("Uv.Query: %v", err)
	}
	if !ok {
		t.Error("Uv.Query(prefixed git URL): expected true")
	}
}

// TestUv_Query_GitURLDoesNotMatchUnrelatedSameBasename verifies a git URL
// spec does not inherit a PyPI-installed tool that happens to share the repo name.
func TestUv_Query_GitURLDoesNotMatchUnrelatedSameBasename(t *testing.T) {
	installFakeBinary(t, "uv", fakeUvToolListWithSpecifiers(
		`  echo "ruff v0.6.9 [required: ruff]"`))
	ok, err := Uv{}.Query("git+https://github.com/Org/ruff.git")
	if err != nil {
		t.Fatalf("Uv.Query: %v", err)
	}
	if ok {
		t.Error("Uv.Query(git URL named ruff): expected false when listed ruff is not from that URL")
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

// TestUv_QueryVersion_GitURLSpec verifies QueryVersion reads the listed
// version for a bare git URL spec.
func TestUv_QueryVersion_GitURLSpec(t *testing.T) {
	installFakeBinary(t, "uv", fakeUvToolListWithSpecifiers(
		`  echo "guild-ai-cli v0.1.0 [required:  git+ssh://git@github.com/GuildEducationInc/guild-ai-cli.git]"`))
	ver, err := Uv{}.QueryVersion("git+ssh://git@github.com/GuildEducationInc/guild-ai-cli.git")
	if err != nil {
		t.Fatalf("Uv.QueryVersion: %v", err)
	}
	if ver != "0.1.0" {
		t.Errorf("version: got %q, want %q", ver, "0.1.0")
	}

	ver, err = Uv{}.QueryVersion("git+ssh://git@github.com/Org/repo-name.git@v1.2.0")
	if err != nil {
		t.Fatalf("Uv.QueryVersion: %v", err)
	}
	if ver != "" {
		t.Errorf("absent git URL version: got %q, want empty", ver)
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

// TestUv_Query_FallsBackWhenShowVersionSpecifiersUnsupported verifies older
// uv that rejects --show-version-specifiers still answers Query via plain list.
func TestUv_Query_FallsBackWhenShowVersionSpecifiersUnsupported(t *testing.T) {
	installFakeBinary(t, "uv",
		`if [ "$1" = "tool" ] && [ "$2" = "list" ] && [ "$3" = "--show-version-specifiers" ]; then
  echo "unknown argument" >&2
  exit 2
fi
if [ "$1" = "tool" ] && [ "$2" = "list" ]; then
  echo "ruff v0.6.9"
  exit 0
fi
exit 1`)
	ok, err := Uv{}.Query("ruff")
	if err != nil {
		t.Fatalf("Uv.Query: %v", err)
	}
	if !ok {
		t.Error("Uv.Query(ruff): expected true after falling back to uv tool list")
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

// TestUv_PlanUninstall_GitURLUsesToolName verifies uninstall plans use the
// installed tool name, not a truncated git+ssh://git fragment.
func TestUv_PlanUninstall_GitURLUsesToolName(t *testing.T) {
	args := Uv{}.PlanUninstall("git+ssh://git@github.com/GuildEducationInc/guild-ai-cli.git")
	want := []string{"uv", "tool", "uninstall", "guild-ai-cli"}
	if !slices.Equal(args, want) {
		t.Errorf("PlanUninstall(git+ssh) = %v, want %v", args, want)
	}

	args = Uv{}.PlanUninstall("git+https://github.com/Org/tool.git@main#egg=tool")
	want = []string{"uv", "tool", "uninstall", "tool"}
	if !slices.Equal(args, want) {
		t.Errorf("PlanUninstall(git+https) = %v, want %v", args, want)
	}
}

// TestUv_PlanUninstall_GitURLUsesListedNameWhenRepoDiffers verifies uninstall
// uses uv's listed name when the repo basename is not the package name.
func TestUv_PlanUninstall_GitURLUsesListedNameWhenRepoDiffers(t *testing.T) {
	spec := "git+ssh://git@github.com/Org/repo-name.git"
	installFakeBinary(t, "uv", fakeUvToolListWithSpecifiers(
		`  echo "actual-pkg v0.2.0 [required:  `+spec+`]"`))
	args := Uv{}.PlanUninstall(spec)
	want := []string{"uv", "tool", "uninstall", "actual-pkg"}
	if !slices.Equal(args, want) {
		t.Errorf("PlanUninstall(mismatched git URL) = %v, want %v", args, want)
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

// TestUvToolName strips @version suffixes, leaves bare names untouched, and
// derives a tool name from git URL specs instead of cutting at git@host.
func TestUvToolName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"ruff", "ruff"},
		{"ruff@0.6.0", "ruff"},
		{"ruff@latest", "ruff"},
		{"some-pkg@1.2.3", "some-pkg"},
		{"git+ssh://git@github.com/Org/tool.git", "tool"},
		{"git+ssh://git@github.com/Org/tool.git@v1.2.0", "tool"},
		{"git+https://github.com/Org/tool", "tool"},
		{"git+https://github.com/Org/tool.git@main#egg=tool", "tool"},
		{"guild-ai-cli@git+ssh://git@github.com/GuildEducationInc/guild-ai-cli.git", "guild-ai-cli"},
	}
	for _, tc := range tests {
		got := uvToolName(tc.input)
		if got != tc.want {
			t.Errorf("uvToolName(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

// TestUv_ListOutdated_GitURLSpec maps a git URL tracked spec onto the listed
// tool name so outdated detection is not skipped as a missing install.
func TestUv_ListOutdated_GitURLSpec(t *testing.T) {
	spec := "git+ssh://git@github.com/Org/repo-name.git"
	installFakeBinary(t, "uv", fakeUvToolListWithSpecifiers(
		`  echo "actual-pkg v0.2.0 [required:  `+spec+`]"`))
	defer swapPypiLatest(t, map[string]string{"actual-pkg": "0.3.0"}, nil)()

	got, err := Uv{}.ListOutdated([]string{spec})
	if err != nil {
		t.Fatalf("ListOutdated: %v", err)
	}
	want := map[string]string{"actual-pkg": "0.3.0"}
	if !maps.Equal(got, want) {
		t.Errorf("ListOutdated(git URL) = %v, want %v", got, want)
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
