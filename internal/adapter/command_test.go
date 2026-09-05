package adapter

import (
	"reflect"
	"strings"
	"testing"
)

func TestSplitCommand(t *testing.T) {
	tests := []struct {
		in      string
		want    []string
		wantErr bool
	}{
		{"claude plugin install {{id}} --scope user", []string{"claude", "plugin", "install", "{{id}}", "--scope", "user"}, false},
		{`gh extension install "foo bar"`, []string{"gh", "extension", "install", "foo bar"}, false},
		{"  list   --json  ", []string{"list", "--json"}, false},
		{"", nil, true},
		{`echo "unterminated`, nil, true},
	}
	for _, tc := range tests {
		got, err := splitCommand(tc.in)
		if tc.wantErr {
			if err == nil {
				t.Fatalf("splitCommand(%q) err=nil, want error", tc.in)
			}
			continue
		}
		if err != nil {
			t.Fatalf("splitCommand(%q): %v", tc.in, err)
		}
		if !reflect.DeepEqual(got, tc.want) {
			t.Fatalf("splitCommand(%q)=%v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestExpandTemplate(t *testing.T) {
	got := expandTemplate("claude plugin install {{id}} --scope user", "slack@official")
	if got != "claude plugin install slack@official --scope user" {
		t.Fatalf("expand {{id}}: %q", got)
	}
	got = expandTemplate("tool add {{name}}", "pkg")
	if got != "tool add pkg" {
		t.Fatalf("expand {{name}}: %q", got)
	}
}

func TestParseListOutput_LinesJSONRegex(t *testing.T) {
	t.Run("lines", func(t *testing.T) {
		ids, versions, err := parseListOutput([]byte("cli/gh-copilot\t1.0.0\nowner/other 2.0\n"), CommandDef{})
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(ids, []string{"cli/gh-copilot", "owner/other"}) {
			t.Fatalf("ids=%v", ids)
		}
		if versions != nil {
			t.Fatalf("versions=%v, want nil", versions)
		}
	})

	t.Run("json array", func(t *testing.T) {
		raw := `[{"name":"slack@official","version":"1.2.3"},{"name":"other@src","version":"0.1.0"}]`
		ids, versions, err := parseListOutput([]byte(raw), CommandDef{IDField: "name", VersionField: "version"})
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(ids, []string{"slack@official", "other@src"}) {
			t.Fatalf("ids=%v", ids)
		}
		if versions["slack@official"] != "1.2.3" || versions["other@src"] != "0.1.0" {
			t.Fatalf("versions=%v", versions)
		}
	})

	t.Run("json path", func(t *testing.T) {
		raw := `{"plugins":[{"id":"a","version":"2"},{"id":"b","version":"3"}]}`
		ids, versions, err := parseListOutput([]byte(raw), CommandDef{IDField: "plugins.id", VersionField: "version"})
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(ids, []string{"a", "b"}) {
			t.Fatalf("ids=%v", ids)
		}
		if versions["a"] != "2" {
			t.Fatalf("versions=%v", versions)
		}
	})

	t.Run("regex named groups", func(t *testing.T) {
		raw := "cli/gh-copilot\tv1.2.3\ttrue\nowner/ext\tv9.0\tfalse\n"
		ids, versions, err := parseListOutput([]byte(raw), CommandDef{
			ListMatch: `(?m)^(?P<id>\S+)\s+(?P<version>\S+)`,
		})
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(ids, []string{"cli/gh-copilot", "owner/ext"}) {
			t.Fatalf("ids=%v", ids)
		}
		if versions["cli/gh-copilot"] != "v1.2.3" {
			t.Fatalf("versions=%v", versions)
		}
	})
}

func TestCommand_PlansAndInventory(t *testing.T) {
	installFakeBinary(t, "plugctl", `
case "$1" in
  list)
    printf '%s\n' 'slack@official 1.2.3' 'other@src 0.1.0'
    ;;
  version)
    if [ "$2" = "slack@official" ]; then printf '1.2.3\n'; else exit 1; fi
    ;;
  *)
    exit 0
    ;;
esac
`)
	c := NewCommand("plug", CommandDef{
		List:    "plugctl list",
		Install: "plugctl install {{id}} --user",
		Remove:  "plugctl remove {{id}}",
		Upgrade: "plugctl update {{id}}",
		Version: "plugctl version {{id}}",
	})
	if c.Name() != "plug" {
		t.Fatalf("Name=%q", c.Name())
	}
	if !c.Available() {
		t.Fatal("Available()=false, want true")
	}
	if got := c.PlanInstall("slack@official"); !reflect.DeepEqual(got, []string{"plugctl", "install", "slack@official", "--user"}) {
		t.Fatalf("PlanInstall=%v", got)
	}
	if got := c.PlanUninstall("slack@official"); !reflect.DeepEqual(got, []string{"plugctl", "remove", "slack@official"}) {
		t.Fatalf("PlanUninstall=%v", got)
	}
	if got := c.PlanUpgrade("slack@official"); !reflect.DeepEqual(got, []string{"plugctl", "update", "slack@official"}) {
		t.Fatalf("PlanUpgrade=%v", got)
	}
	if IsDefaultFallbackEligible(c) {
		t.Fatal("spec adapters must not be default fallback")
	}

	ids, err := c.ListInstalled()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(ids, []string{"slack@official", "other@src"}) {
		t.Fatalf("ListInstalled=%v", ids)
	}
	ok, err := c.Query("slack@official")
	if err != nil || !ok {
		t.Fatalf("Query installed: %v %v", ok, err)
	}
	ok, err = c.Query("missing")
	if err != nil || ok {
		t.Fatalf("Query missing: %v %v", ok, err)
	}
	ver, err := c.QueryVersion("slack@official")
	if err != nil || ver != "1.2.3" {
		t.Fatalf("QueryVersion=%q %v", ver, err)
	}
}

func TestCommand_UpgradeFallsBackToInstall(t *testing.T) {
	c := NewCommand("plug", CommandDef{Install: "plugctl add {{id}}"})
	if got := c.PlanUpgrade("x"); !reflect.DeepEqual(got, []string{"plugctl", "add", "x"}) {
		t.Fatalf("PlanUpgrade fallback=%v", got)
	}
}

func TestCommand_UnavailableWithoutBinary(t *testing.T) {
	c := NewCommand("plug", CommandDef{List: "definitely-not-a-genv-bin list", Install: "definitely-not-a-genv-bin add {{id}}"})
	if c.Available() {
		t.Fatal("Available()=true for missing binary")
	}
}

func TestSetSpecAdapters_ByNameAndRegistered(t *testing.T) {
	t.Cleanup(func() { SetSpecAdapters(nil) })
	if ByName("plug") != nil {
		t.Fatal("ByName(plug) should be nil before bind")
	}
	SetSpecAdapters(CommandsFromDefs(map[string]CommandDef{
		"plug": {List: "true", Install: "true", Remove: "true"},
	}))
	got := ByName("plug")
	if got == nil || got.Name() != "plug" {
		t.Fatalf("ByName(plug)=%v", got)
	}
	if !SpecName("plug") {
		t.Fatal("SpecName(plug)=false")
	}
	found := false
	for _, a := range Registered() {
		if a.Name() == "plug" {
			found = true
		}
	}
	if !found {
		t.Fatal("Registered() missing spec adapter")
	}
	if ByName("brew") == nil {
		t.Fatal("built-in ByName(brew) still works")
	}
	SetSpecAdapters(nil)
	if ByName("plug") != nil {
		t.Fatal("ByName(plug) after clear")
	}
}

func TestCommandsFromDefs_OutdatedImplementsLister(t *testing.T) {
	plain := CommandsFromDefs(map[string]CommandDef{
		"plain": {List: "true", Install: "true", Remove: "true"},
	})
	if _, ok := plain[0].(OutdatedLister); ok {
		t.Fatal("plain command must not implement OutdatedLister")
	}
	with := CommandsFromDefs(map[string]CommandDef{
		"with": {List: "true", Install: "true", Remove: "true", Outdated: "true"},
	})
	if _, ok := with[0].(OutdatedLister); !ok {
		t.Fatal("outdated command should implement OutdatedLister")
	}
}

func TestCommand_ListOutdated(t *testing.T) {
	installFakeBinary(t, "outdatedctl", `printf '%s\n' 'keep-me' 'skip-me'`)
	c := commandWithOutdated{Command: NewCommand("plug", CommandDef{Outdated: "outdatedctl"})}
	got, err := c.ListOutdated([]string{"keep-me"})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := got["keep-me"]; !ok {
		t.Fatalf("got=%v, want keep-me", got)
	}
	if _, ok := got["skip-me"]; ok {
		t.Fatalf("got=%v, skip-me should be filtered", got)
	}
}

func TestCommand_JSONInventoryFromFakeBin(t *testing.T) {
	installFakeBinary(t, "jsonplug", `printf '%s\n' '[{"name":"a@src","version":"9"},{"name":"b@src","version":"8"}]'`)
	c := NewCommand("jsonplug", CommandDef{
		List:         "jsonplug",
		Install:      "jsonplug add {{id}}",
		Remove:       "jsonplug rm {{id}}",
		IDField:      "name",
		VersionField: "version",
	})
	vers, err := c.ListInstalledVersions()
	if err != nil {
		t.Fatal(err)
	}
	if vers["a@src"] != "9" || vers["b@src"] != "8" {
		t.Fatalf("versions=%v", vers)
	}
	ver, err := c.QueryVersion("a@src")
	if err != nil || ver != "9" {
		t.Fatalf("QueryVersion from list=%q %v", ver, err)
	}
}

func TestCommand_EmptyListIsEmpty(t *testing.T) {
	installFakeBinary(t, "emptylist", "exit 1")
	c := NewCommand("empty", CommandDef{List: "emptylist"})
	ids, err := c.ListInstalled()
	if err != nil {
		t.Fatalf("non-zero list exit should be empty, not error: %v", err)
	}
	if len(ids) != 0 {
		t.Fatalf("ids=%v", ids)
	}
}

func TestParseListOutput_JSONFallbackToRegex(t *testing.T) {
	raw := "not-json\nfoo-id v1\n"
	ids, _, err := parseListOutput([]byte(raw), CommandDef{
		IDField:   "name",
		ListMatch: `(?m)^(?P<id>\S+)`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(ids, ","), "foo-id") {
		t.Fatalf("ids=%v", ids)
	}
}
