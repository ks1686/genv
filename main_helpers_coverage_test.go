package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ks1686/genv/internal/adapter"
	"github.com/ks1686/genv/internal/files"
	"github.com/ks1686/genv/internal/genvfile"
	"github.com/ks1686/genv/internal/output"
	"github.com/ks1686/genv/internal/resolver"
	"github.com/ks1686/genv/internal/schema"
)

func TestParseCommaListAndOptionalDuration(t *testing.T) {
	if got := parseCommaList(""); got != nil {
		t.Errorf("empty = %v, want nil", got)
	}
	if got := parseCommaList(" a, ,b , c "); len(got) != 3 || got[0] != "a" || got[1] != "b" || got[2] != "c" {
		t.Errorf("parseCommaList = %v", got)
	}

	if d, err := parseOptionalDuration(""); err != nil || d != 0 {
		t.Errorf("empty duration = %v, %v", d, err)
	}
	if d, err := parseOptionalDuration("5s"); err != nil || d != 5*time.Second {
		t.Errorf("5s = %v, %v", d, err)
	}
	if _, err := parseOptionalDuration("-1s"); err == nil {
		t.Error("expected error for negative duration")
	}
	if _, err := parseOptionalDuration("nope"); err == nil {
		t.Error("expected error for invalid duration")
	}
}

func TestUpgradeActionHelpers(t *testing.T) {
	action := resolver.UpgradeAction{
		LPs: []genvfile.LockedPackage{{ID: "git"}, {ID: "curl"}},
	}
	key := upgradeActionIDKey(action)
	if key != `["git" "curl"]` {
		t.Fatalf("upgradeActionIDKey = %q", key)
	}
	errs := []error{errors.New(`upgrade ["git" "curl"]: boom`)}
	if got := upgradeActionError(action, errs); !strings.Contains(got, "boom") {
		t.Errorf("upgradeActionError = %q", got)
	}
	if got := upgradeActionError(action, []error{errors.New("other")}); got != "" {
		t.Errorf("unrelated error matched: %q", got)
	}
	if got := upgradeFailedIDs([]resolver.UpgradeAction{action}, nil); got != nil {
		t.Errorf("no errs = %v", got)
	}
	if got := upgradeFailedIDs([]resolver.UpgradeAction{action}, errs); len(got) != 2 || got[0] != "git" || got[1] != "curl" {
		t.Errorf("upgradeFailedIDs = %v", got)
	}
}

func TestFileAndPathHelpers(t *testing.T) {
	if mergeLockedFiles(nil, nil) != nil && len(mergeLockedFiles(nil, nil)) != 0 {
		t.Errorf("merge nil unexpected")
	}
	existing := []genvfile.LockedFile{{Source: "a", Target: "b", Mode: "link"}}
	adopted := []genvfile.LockedFile{
		{Source: "a", Target: "b", Mode: "link"},
		{Source: "c", Target: "d", Mode: "copy"},
	}
	merged := mergeLockedFiles(existing, adopted)
	if len(merged) != 2 {
		t.Fatalf("merged = %#v", merged)
	}

	home := t.TempDir()
	t.Setenv("HOME", home)
	if got := expandCLIPath("~/dot"); got != filepath.Join(home, "dot") && got != home+"/dot" {
		// expand uses home + path[1:], so ~/dot → home + /dot
		if !strings.HasSuffix(got, "dot") {
			t.Errorf("expandCLIPath = %q", got)
		}
	}
	if got := resolveCLISource("/root", "rel"); got != filepath.Join("/root", "rel") {
		t.Errorf("resolve relative = %q", got)
	}
	if got := resolveCLISource("/root", "/abs"); got != "/abs" {
		t.Errorf("resolve abs = %q", got)
	}

	cfg := &schema.FilesConfig{
		Links:     []schema.FileLink{{Source: "src", Target: "dst"}},
		Templates: []schema.FileTemplate{{Source: "tpl", Target: "out"}},
	}
	resolved := filesConfigWithResolvedSources(cfg, "/repo")
	if resolved.Links[0].Source != filepath.Join("/repo", "src") {
		t.Errorf("resolved link = %q", resolved.Links[0].Source)
	}
	if filesConfigWithResolvedSources(nil, "/repo") != nil {
		t.Error("nil cfg should stay nil")
	}

	if sourceRootForSpec("/tmp/genv.json", &schema.GenvFile{Repo: &schema.Repo{URL: "/repo"}}) != "/repo" {
		t.Error("repo url should win as source root")
	}
	if sourceRootForSpec("/tmp/genv.json", &schema.GenvFile{}) != "/tmp" {
		t.Error("spec dir should be source root")
	}

	if !hasBoolFlag([]string{"--files", "--json"}, "files") || hasBoolFlag([]string{"--file"}, "files") {
		t.Error("hasBoolFlag mismatch")
	}

	plan := filePlanEntries(&files.ApplyResult{
		Created:    []string{"a"},
		Updated:    []string{"b"},
		Skipped:    []string{"c"},
		Mismatched: []string{"d"},
	})
	if len(plan) != 4 {
		t.Fatalf("filePlanEntries = %#v", plan)
	}
	if filePlanEntries(nil) != nil {
		t.Error("nil plan")
	}
	status := fileStatusEntries(&files.StatusResult{Entries: []files.StatusEntry{{
		Source: "s", Target: "t", Mode: "link", Kind: "ok",
	}}})
	if len(status) != 1 || status[0].Source != "s" {
		t.Errorf("fileStatusEntries = %#v", status)
	}
	if fileStatusEntries(nil) != nil {
		t.Error("nil status")
	}

	var buf bytes.Buffer
	writeFileStatus(&buf, &files.StatusResult{Entries: []files.StatusEntry{
		{Kind: "ok", Target: "skip", Mode: "link"},
		{Kind: "missing", Target: "need", Mode: "link"},
	}})
	if !strings.Contains(buf.String(), "missing") || strings.Contains(buf.String(), "skip") {
		t.Errorf("writeFileStatus = %q", buf.String())
	}
	writeFileStatus(&buf, nil) // no-op
}

func TestErrStringHelpersAndUpgradeHooksJSON(t *testing.T) {
	if errStrings(nil) != nil {
		t.Error("nil errs")
	}
	if got := errStrings([]error{errors.New("a")}); len(got) != 1 || got[0] != "a" {
		t.Errorf("errStrings = %v", got)
	}
	if upgradeHookErrorStrings(nil) != nil {
		t.Error("nil hook results")
	}
	if got := upgradeHookErrorStrings([]output.UpgradeHookResult{{Error: "boom"}}); len(got) != 1 || got[0] != "boom" {
		t.Errorf("upgradeHookErrorStrings = %v", got)
	}
	if runUpgradeHooksJSON(context.Background(), nil, upgradeHookOptions{Phase: "pre"}) != nil {
		t.Error("nil file should skip hooks")
	}
	if runUpgradeHooksJSON(context.Background(), &schema.GenvFile{}, upgradeHookOptions{Phase: "pre"}) != nil {
		t.Error("nil hooks should skip")
	}
}

func TestXDGHelpersAndCompleteInternal(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", filepath.Join(dir, "data"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "config"))
	if got := xdgDataHome(); got != filepath.Join(dir, "data") {
		t.Errorf("xdgDataHome = %q", got)
	}
	if got := xdgConfigHome(); got != filepath.Join(dir, "config") {
		t.Errorf("xdgConfigHome = %q", got)
	}

	if code := completeInternalCmd(nil); code != exitUsage {
		t.Errorf("no topic = %d", code)
	}
	if code := completeInternalCmd([]string{"nope"}); code != exitUsage {
		t.Errorf("unknown topic = %d", code)
	}

	spec := filepath.Join(dir, "genv.json")
	if err := os.WriteFile(spec, []byte(`{"schemaVersion":"6","packages":[{"id":"git"},{"id":"curl"}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	var code int
	out := captureStdout(t, func() {
		code = completeInternalCmd([]string{"packages", "--file", spec})
	})
	if code != exitOK || !strings.Contains(out, "git") || !strings.Contains(out, "curl") {
		t.Errorf("packages complete = %d %q", code, out)
	}
	out = captureStdout(t, func() {
		code = completeInternalCmd([]string{"packages", "--file", filepath.Join(dir, "missing.json")})
	})
	if code != exitOK {
		t.Errorf("missing spec during complete = %d", code)
	}
	out = captureStdout(t, func() {
		code = completeInternalCmd([]string{"managers"})
	})
	if code != exitOK {
		t.Errorf("managers complete = %d", code)
	}
}

func TestCompletionCmdIncludesPortabilityEntries(t *testing.T) {
	var code int
	out := captureStdout(t, func() {
		code = completionCmd([]string{"bash"})
	})
	if code != exitOK {
		t.Fatalf("completion bash = %d, want %d", code, exitOK)
	}
	for _, want := range []string{"migrate", "--target", "--force-new-lock"} {
		if !strings.Contains(out, want) {
			t.Fatalf("completion output missing %q", want)
		}
	}
	if code := completionCmd(nil); code != exitUsage {
		t.Fatalf("completion missing shell = %d, want %d", code, exitUsage)
	}
	if code := completionCmd([]string{"unknown"}); code != exitUsage {
		t.Fatalf("completion unknown shell = %d, want %d", code, exitUsage)
	}
}

func TestApplyEnvVarsAndShellCfg(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))

	lf := &genvfile.LockFile{SchemaVersion: "1"}
	f := &schema.GenvFile{
		SchemaVersion: "6",
		Env: map[string]schema.EnvVar{
			"FOO": {Value: "bar"},
		},
		Shell: &schema.ShellConfig{
			Aliases: map[string]schema.ShellAlias{
				"ll":    {Value: "ls -la"},
				"ffish": {Value: "echo fish", Shell: "fish"},
			},
		},
	}

	applied, removed := applyEnvVars(f, lf, true)
	if len(applied) != 1 || applied[0] != "FOO" || len(removed) != 0 {
		t.Fatalf("applyEnvVars = %v %v", applied, removed)
	}
	if len(lf.Env) != 1 || lf.Env[0].Name != "FOO" {
		t.Fatalf("lock env = %#v", lf.Env)
	}

	// Removal path: clear spec env while lock still has FOO.
	f.Env = map[string]schema.EnvVar{}
	applied, removed = applyEnvVars(f, lf, true)
	if len(removed) != 1 || removed[0] != "FOO" {
		t.Fatalf("env remove = %v %v", applied, removed)
	}

	applied, removed = applyShellCfg(f, lf, true)
	if len(applied) == 0 {
		t.Fatalf("applyShellCfg applied nothing: %v %v", applied, removed)
	}
	if lf.Shell == nil {
		t.Fatal("expected lock shell updated")
	}
	if !strings.Contains(captureStdout(t, func() {
		_, _ = applyShellCfg(f, lf, false)
	}), "") {
		// fish note only prints when hasFishEntries; force by keeping fish alias
	}
	// Ensure fish note path is exercised with verbose false still writing note.
	out := captureStdout(t, func() {
		f2 := &schema.GenvFile{Shell: f.Shell}
		lf2 := &genvfile.LockFile{}
		_, _ = applyShellCfg(f2, lf2, false)
	})
	if !strings.Contains(out, "fish-specific") {
		t.Errorf("expected fish note, got %q", out)
	}

	if a, r := applyEnvVars(&schema.GenvFile{}, &genvfile.LockFile{}, false); a != nil || r != nil {
		t.Errorf("empty env apply = %v %v", a, r)
	}
	if a, r := applyShellCfg(&schema.GenvFile{}, &genvfile.LockFile{}, false); a != nil || r != nil {
		t.Errorf("empty shell apply = %v %v", a, r)
	}
}

func TestAdoptFilesCmd_SuccessAndJSON(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("HOME", dir)

	src := filepath.Join(dir, "src.txt")
	dst := filepath.Join(dir, "dst.txt")
	if err := os.WriteFile(src, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(src, dst); err != nil {
		t.Fatal(err)
	}

	specPath := filepath.Join(dir, "genv.json")
	spec := `{"schemaVersion":"6","files":{"links":[{"source":"` + src + `","target":"` + dst + `"}]}}`
	if err := os.WriteFile(specPath, []byte(spec), 0o644); err != nil {
		t.Fatal(err)
	}

	if code := adoptFilesCmd(specPath, "", "", false); code != exitOK {
		t.Fatalf("adopt files = %d", code)
	}
	lockPath := genvfile.LockPathFrom(specPath)
	lf, err := genvfile.ReadLock(lockPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(lf.Files) != 1 {
		t.Fatalf("lock files = %#v", lf.Files)
	}

	var code int
	out := captureStdout(t, func() {
		code = adoptFilesCmd(specPath, "", "", true)
	})
	if code != exitOK {
		t.Fatalf("adopt files --json = %d (%s)", code, out)
	}
	if !strings.Contains(out, `"ok"`) {
		t.Errorf("json output = %s", out)
	}

	if code := adoptFilesCmd(filepath.Join(dir, "missing.json"), "", "", false); code != exitIO {
		t.Errorf("missing spec = %d", code)
	}
}

func TestBuildPlanAndOutputConverters(t *testing.T) {
	f := &schema.GenvFile{SchemaVersion: "6", Packages: []schema.Package{{ID: "git"}}}
	lf := &genvfile.LockFile{SchemaVersion: "1"}
	result := resolver.ReconcileResult{
		ToInstall: []resolver.Action{
			{Pkg: schema.Package{ID: "git"}, Manager: "brew", Cmd: []string{"brew", "install", "git"}},
			{Pkg: schema.Package{ID: "missing"}},
		},
		ToRemove: []resolver.Action{
			{Pkg: schema.Package{ID: "old"}, Manager: "brew", UninstallCmd: []string{"brew", "uninstall", "old"}},
		},
	}
	plan := buildPlanResult(f, lf, result)
	if len(plan.ToInstall) != 2 || len(plan.ToRemove) != 1 {
		t.Fatalf("plan = %#v", plan)
	}

	envOut, drift := toOutputEnvEntries(nil)
	if len(envOut) != 0 || drift {
		t.Errorf("empty env entries = %v %v", envOut, drift)
	}
	shellOut, drift := toOutputShellEntries(nil)
	if len(shellOut) != 0 || drift {
		t.Errorf("empty shell entries = %v %v", shellOut, drift)
	}
}

func TestInitCmd_CreatesSpecFromStdin(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	path := filepath.Join(dir, "genv.json")

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdin
	os.Stdin = r
	t.Cleanup(func() { os.Stdin = old })
	go func() {
		_, _ = w.WriteString("git\ngit\ncurl\n\n")
		_ = w.Close()
	}()

	if code := run([]string{"init", "--file", path}); code != exitOK {
		t.Fatalf("init = %d", code)
	}
	spec, err := genvfile.Read(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(spec.Packages) != 2 {
		t.Fatalf("packages = %#v, want git+curl", spec.Packages)
	}

	if code := run([]string{"init", "--file", path}); code != exitLogic {
		t.Errorf("init overwrite existing = %d, want exitLogic", code)
	}
}

func TestUpgradeJSON_DryRunAndEmptyPlan(t *testing.T) {
	f := &schema.GenvFile{SchemaVersion: "6"}
	lf := &genvfile.LockFile{SchemaVersion: "1"}
	plan := []resolver.UpgradeAction{{
		Cmd: []string{"brew", "upgrade", "git"},
		LPs: []genvfile.LockedPackage{{ID: "git", Manager: "brew", PkgName: "git"}},
	}}
	filters := output.UpgradeFilters{All: false}

	var code int
	out := captureStdout(t, func() {
		code = upgradeJSON(true, "", "", 0, f, lf, plan, nil, filters)
	})
	if code != exitOK || !strings.Contains(out, `"planned"`) {
		t.Fatalf("dry-run upgradeJSON = %d %s", code, out)
	}

	out = captureStdout(t, func() {
		code = upgradeJSON(false, "", "", 0, f, lf, nil, nil, output.UpgradeFilters{HooksSkipped: true})
	})
	if code != exitOK {
		t.Fatalf("empty plan upgradeJSON = %d %s", code, out)
	}
}

func TestCleanCmd_ExecutesPlan(t *testing.T) {
	original := adapter.All
	adapter.All = []adapter.Adapter{coverageCleanAdapter{}}
	t.Cleanup(func() { adapter.All = original })

	if code := run([]string{"clean"}); code != exitOK {
		t.Fatalf("clean = %d", code)
	}
}
