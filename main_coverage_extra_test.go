package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ks1686/genv/internal/adapter"
)

func TestValidateCmd_ValidInvalidAndMissing(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	valid := filepath.Join(dir, "valid.json")
	if err := os.WriteFile(valid, []byte(`{"schemaVersion":"6","packages":[]}`), 0o644); err != nil {
		t.Fatalf("write valid spec: %v", err)
	}

	if code := run([]string{"validate", "--file", valid}); code != exitOK {
		t.Fatalf("validate valid spec = %d, want exitOK", code)
	}

	invalid := filepath.Join(dir, "invalid.json")
	if err := os.WriteFile(invalid, []byte(`{"schemaVersion":"99","packages":[]}`), 0o644); err != nil {
		t.Fatalf("write invalid spec: %v", err)
	}
	if code := run([]string{"validate", "--file", invalid}); code != exitValidation {
		t.Errorf("validate invalid spec = %d, want exitValidation", code)
	}

	if code := run([]string{"validate", "--file", filepath.Join(dir, "missing.json")}); code != exitValidation {
		t.Errorf("validate missing spec = %d, want exitValidation", code)
	}
}

func TestCompletionCmd_PrintsScriptsAndInstalls(t *testing.T) {
	for _, tc := range []struct {
		shell  string
		marker string
	}{
		{shell: "bash", marker: "bash completion"},
		{shell: "zsh", marker: "#compdef genv"},
		{shell: "fish", marker: "fish completion"},
		{shell: "powershell", marker: "Register-ArgumentCompleter"},
	} {
		t.Run(tc.shell, func(t *testing.T) {
			var code int
			out := captureStdout(t, func() { code = run([]string{"completion", tc.shell}) })
			if code != exitOK {
				t.Fatalf("completion %s = %d, want exitOK", tc.shell, code)
			}
			if !strings.Contains(out, tc.marker) {
				t.Errorf("completion %s output missing %q", tc.shell, tc.marker)
			}
		})
	}

	dir := t.TempDir()
	if code := run([]string{"completion", "install", "fish", "--dir", dir}); code != exitOK {
		t.Fatalf("completion install = %d, want exitOK", code)
	}
	if _, err := os.Stat(filepath.Join(dir, "genv.fish")); err != nil {
		t.Fatalf("installed completion: %v", err)
	}
	if code := run([]string{"completion", "powershell"}); code != exitOK {
		t.Errorf("completion powershell = %d, want exitOK", code)
	}
	if code := run([]string{"completion", "unknownshell"}); code != exitUsage {
		t.Errorf("completion unknown shell = %d, want exitUsage", code)
	}
}

type coverageCleanAdapter struct{}

func (coverageCleanAdapter) Name() string    { return "coverage-clean" }
func (coverageCleanAdapter) Available() bool { return true }
func (coverageCleanAdapter) NormalizeID(id string, _ map[string]string) (string, bool) {
	return id, false
}
func (coverageCleanAdapter) PlanInstall(string) []string   { return nil }
func (coverageCleanAdapter) PlanUninstall(string) []string { return nil }
func (coverageCleanAdapter) PlanUpgrade(string) []string   { return nil }
func (coverageCleanAdapter) PlanClean() [][]string         { return [][]string{{"echo", "cleaned"}} }
func (coverageCleanAdapter) Query(string) (bool, error)    { return false, nil }
func (coverageCleanAdapter) ListInstalled() ([]string, error) {
	return nil, nil
}
func (coverageCleanAdapter) QueryVersion(string) (string, error) { return "", nil }

func TestCleanCmd_DryRunPrintsPlan(t *testing.T) {
	original := adapter.All
	adapter.All = []adapter.Adapter{coverageCleanAdapter{}}
	t.Cleanup(func() { adapter.All = original })

	var code int
	out := captureStdout(t, func() { code = run([]string{"clean", "--dry-run"}) })
	if code != exitOK {
		t.Fatalf("clean --dry-run = %d, want exitOK", code)
	}
	if !strings.Contains(out, "[coverage-clean]") || !strings.Contains(out, "echo cleaned") {
		t.Errorf("clean output = %q, want manager and command", out)
	}
}

func TestEditCmdAndShellEditCmd_RunConfiguredEditor(t *testing.T) {
	dir := t.TempDir()
	editor := filepath.Join(dir, "vi")
	record := filepath.Join(dir, "editor-args")
	script := "#!/bin/sh\nprintf '%s' \"$1\" > \"$EDITOR_RECORD\"\n"
	if err := os.WriteFile(editor, []byte(script), 0o755); err != nil {
		t.Fatalf("write editor script: %v", err)
	}
	t.Setenv("EDITOR", editor)
	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR_RECORD", record)

	for _, tc := range []struct {
		name string
		args []string
	}{
		{name: "edit", args: []string{"edit", "--file", filepath.Join(dir, "edit.json")}},
		{name: "shell edit", args: []string{"shell", "edit", "--file", filepath.Join(dir, "shell.json")}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if code := run(tc.args); code != exitOK {
				t.Fatalf("%v = %d, want exitOK", tc.args, code)
			}
			got, err := os.ReadFile(record)
			if err != nil {
				t.Fatalf("read editor arguments: %v", err)
			}
			if string(got) != tc.args[len(tc.args)-1] {
				t.Errorf("editor argument = %q, want %q", got, tc.args[len(tc.args)-1])
			}
		})
	}
}

func TestPullCmd_ClonesLocalRepositoryAndCopiesSpec(t *testing.T) {
	dir := t.TempDir()
	bare := filepath.Join(dir, "remote.git")
	work := filepath.Join(dir, "work")
	runGitCommand(t, "init", "--bare", bare)
	runGitCommand(t, "init", work)
	runGitCommand(t, "-C", work, "checkout", "-b", "main")
	runGitCommand(t, "-C", work, "config", "user.email", "test@example.com")
	runGitCommand(t, "-C", work, "config", "user.name", "Test User")
	runGitCommand(t, "-C", work, "config", "commit.gpgsign", "false")
	runGitCommand(t, "-C", work, "config", "tag.gpgsign", "false")

	remoteSpec := `{"schemaVersion":"6","packages":[],"repo":{"url":"local","ref":"main"}}`
	if err := os.WriteFile(filepath.Join(work, "genv.json"), []byte(remoteSpec), 0o644); err != nil {
		t.Fatalf("write remote spec: %v", err)
	}
	runGitCommand(t, "-C", work, "add", "genv.json")
	runGitCommand(t, "-C", work, "commit", "-m", "add spec")
	runGitCommand(t, "-C", work, "remote", "add", "origin", bare)
	runGitCommand(t, "-C", work, "push", "-u", "origin", "main")
	if !isLocalBranch(work, "main") {
		t.Fatal("isLocalBranch(main) = false, want true")
	}
	if isLocalBranch(work, "missing") {
		t.Error("isLocalBranch(missing) = true, want false")
	}

	t.Setenv("XDG_CACHE_HOME", filepath.Join(dir, "cache"))
	dest := filepath.Join(dir, "config", "genv.json")
	localSpec := `{"schemaVersion":"6","packages":[],"repo":{"url":"` + bare + `","ref":"main"}}`
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		t.Fatalf("create destination directory: %v", err)
	}
	if err := os.WriteFile(dest, []byte(localSpec), 0o644); err != nil {
		t.Fatalf("write local spec: %v", err)
	}

	if code := run([]string{"pull", "--file", dest}); code != exitOK {
		t.Fatalf("pull local repository = %d, want exitOK", code)
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("read pulled spec: %v", err)
	}
	if string(got) != remoteSpec {
		t.Errorf("pulled spec = %q, want %q", got, remoteSpec)
	}
}

func TestPullHelpersAndExpandCLIPath(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", filepath.Join(dir, "cache"))
	cache, err := pullCacheDir()
	if err != nil {
		t.Fatalf("pullCacheDir: %v", err)
	}
	if want := filepath.Join(dir, "cache", "genv", "repo"); cache != want {
		t.Errorf("pullCacheDir = %q, want %q", cache, want)
	}

	src := filepath.Join(dir, "source")
	dst := filepath.Join(dir, "nested", "destination")
	if err := os.WriteFile(src, []byte("contents"), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}
	if err := copyFile(src, dst); err != nil {
		t.Fatalf("copyFile: %v", err)
	}
	if got, err := os.ReadFile(dst); err != nil || string(got) != "contents" {
		t.Errorf("copied contents = %q, err = %v", got, err)
	}

	t.Setenv("COVERAGE_PATH", filepath.Join(dir, "expanded"))
	if got, want := expandCLIPath("$COVERAGE_PATH/file"), filepath.Join(dir, "expanded", "file"); got != want {
		t.Errorf("expandCLIPath = %q, want %q", got, want)
	}
	if err := runGit("rev-parse", "--verify", "definitely-not-a-ref"); err == nil {
		t.Error("runGit expected error for missing ref")
	}
}

func runGitCommand(t *testing.T, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{
		"-c", "commit.gpgsign=false",
		"-c", "tag.gpgsign=false",
	}, args...)...)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, output)
	}
}
