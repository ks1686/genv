//go:build integration

// Package e2e_test contains end-to-end tests for the genv binary.
//
// This file holds the S1-S6 scenarios for the genv files block (schema v5).
// The tests compile against the current genv binary but exercise CLI flags and
// schema fields that do not exist yet, so they are expected to be RED until
// the schema v5, files apply, template renderer, and status --files work land.
package e2e_test

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/ks1686/genv/internal/genvfile"
)

// fileRunner executes genv commands inside an isolated temp HOME.
type fileRunner struct {
	bin      string
	home     string
	genvJSON string
}

// newFileRunner creates a runner with a temp HOME and a genv.json path.
// The HOME directory is used for all target paths and for os.UserHomeDir().
func newFileRunner(t *testing.T) *fileRunner {
	t.Helper()
	home := t.TempDir()
	return &fileRunner{
		bin:      genvBin,
		home:     home,
		genvJSON: filepath.Join(t.TempDir(), "genv.json"),
	}
}

// repoDir returns the absolute path to the checked-in fixture source tree.
func repoDir(t *testing.T) string {
	t.Helper()
	_, testFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Join(filepath.Dir(testFile), "testdata", "files-v5", "repo")
}

// env returns an environment with HOME overridden to the isolated temp dir.
func (fr *fileRunner) env() []string {
	base := os.Environ()
	filtered := make([]string, 0, len(base))
	for _, e := range base {
		if strings.HasPrefix(e, "HOME=") || strings.HasPrefix(e, "XDG_CONFIG_HOME=") {
			continue
		}
		filtered = append(filtered, e)
	}
	return append(filtered, "HOME="+fr.home, "XDG_CONFIG_HOME="+filepath.Join(fr.home, ".config"), "GENV_NO_INTERACTIVE=1")
}

// genv runs a genv subcommand with --file and the isolated HOME injected.
func (fr *fileRunner) genv(stdinData, subcmd string, extra ...string) (stdout, stderr string, code int) {
	args := append([]string{subcmd, "--file", fr.genvJSON}, extra...)
	cmd := exec.Command(fr.bin, args...)
	if stdinData != "" {
		cmd.Stdin = strings.NewReader(stdinData)
	}
	cmd.Env = fr.env()
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	if err := cmd.Run(); err != nil {
		if ex, ok := err.(*exec.ExitError); ok {
			code = ex.ExitCode()
		} else {
			code = -1
		}
	}
	return outBuf.String(), errBuf.String(), code
}

// fileEntry mirrors a single v5 files-block record used by the S1-S6 helpers.
type fileEntry struct {
	Source   string `json:"source"`
	Target   string `json:"target"`
	Mode     string `json:"mode"`
	Template bool   `json:"template,omitempty"`
	Backup   bool   `json:"backup,omitempty"`
}

// writeFilesSpec writes a v5 genv.json pointing at the checked-in fixture repo.
// It maps the flat fileEntry convenience type into the real schema shape:
//
//	repo: {url, ref?}
//	files: {links: [...], templates: [...], dirs: [...]}
func (fr *fileRunner) writeFilesSpec(t *testing.T, entries []fileEntry) {
	t.Helper()
	spec := map[string]any{
		"schemaVersion": "5",
		"repo": map[string]string{
			"url": repoDir(t),
			"ref": "main",
		},
		"files": map[string]any{
			"links":     []map[string]any{},
			"templates": []map[string]any{},
			"dirs":      []map[string]any{},
		},
	}
	files := spec["files"].(map[string]any)
	for _, e := range entries {
		rec := map[string]any{
			"source": e.Source,
			"target": e.Target,
		}
		if e.Backup {
			rec["backup"] = true
		}
		switch e.Mode {
		case "link", "managed-link":
			if e.Mode != "" {
				rec["mode"] = e.Mode
			}
			files["links"] = append(files["links"].([]map[string]any), rec)
		case "copy", "copy-template":
			files["templates"] = append(files["templates"].([]map[string]any), rec)
		default:
			t.Fatalf("unsupported file entry mode %q", e.Mode)
		}
	}
	data, err := json.MarshalIndent(spec, "", "  ")
	if err != nil {
		t.Fatalf("marshal files spec: %v", err)
	}
	if err := os.WriteFile(fr.genvJSON, append(data, '\n'), 0o644); err != nil {
		t.Fatalf("write genv.json: %v", err)
	}
}

// targetPath expands a ~-prefixed target inside the isolated HOME.
func (fr *fileRunner) targetPath(target string) string {
	if strings.HasPrefix(target, "~/") {
		return filepath.Join(fr.home, target[2:])
	}
	return target
}

// ── scenarios S1-S6 ───────────────────────────────────────────────────────────

// TestFiles_S1_FreshEmptyHome verifies that status --files reports every target
// as missing when the HOME directory is empty.
func TestFiles_S1_FreshEmptyHome(t *testing.T) {
	r := newFileRunner(t)
	r.writeFilesSpec(t, []fileEntry{
		{Source: "simple.txt", Target: "~/.genv-test/simple.txt", Mode: "link"},
	})

	stdout, stderr, code := r.genv("", "status", "--files")
	out := stdout + stderr

	if code != 4 {
		t.Errorf("status --files fresh HOME: exit %d, want 4\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	if !strings.Contains(out, "missing") {
		t.Errorf("status --files fresh HOME: expected 'missing' in output, got: %q", out)
	}
	if !strings.Contains(out, "~/.genv-test/simple.txt") && !strings.Contains(out, ".genv-test/simple.txt") {
		t.Errorf("status --files fresh HOME: expected target path in output, got: %q", out)
	}
}

// TestFiles_S2_ApplyClean verifies that apply creates links and a subsequent
// status --files exits 0.
func TestFiles_S2_ApplyClean(t *testing.T) {
	r := newFileRunner(t)
	r.writeFilesSpec(t, []fileEntry{
		{Source: "simple.txt", Target: "~/.genv-test/simple.txt", Mode: "link"},
	})

	stdout, stderr, code := r.genv("", "apply", "--yes")
	if code != 0 {
		t.Fatalf("apply clean: exit %d, want 0\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}

	stdout, stderr, code = r.genv("", "status", "--files")
	out := stdout + stderr
	if code != 0 {
		t.Errorf("status --files after apply: exit %d, want 0\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	if !strings.Contains(out, "ok") && !strings.Contains(out, "up to date") {
		t.Errorf("status --files after apply: expected 'ok' or 'up to date', got: %q", out)
	}
}

// TestFiles_S3_MismatchNoForce verifies that apply refuses to overwrite a
// regular file that blocks a link target and leaves the file untouched.
func TestFiles_S3_MismatchNoForce(t *testing.T) {
	r := newFileRunner(t)
	r.writeFilesSpec(t, []fileEntry{
		{Source: "simple.txt", Target: "~/.genv-test/simple.txt", Mode: "link"},
	})

	target := r.targetPath("~/.genv-test/simple.txt")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("mkdir target parent: %v", err)
	}
	original := []byte("i am a hand-written file\n")
	if err := os.WriteFile(target, original, 0o644); err != nil {
		t.Fatalf("write planted file: %v", err)
	}

	stdout, stderr, code := r.genv("", "apply")
	if code != 4 {
		t.Errorf("apply mismatch no-force: exit %d, want 4\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}

	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read planted file after apply: %v", err)
	}
	if !bytes.Equal(got, original) {
		t.Errorf("apply mismatch no-force: target was modified; want %q, got %q", original, got)
	}
	if fi, err := os.Lstat(target); err != nil || fi.Mode()&os.ModeSymlink != 0 {
		t.Errorf("apply mismatch no-force: target should remain a regular file")
	}
}

// TestFiles_S4_MismatchForceBackup verifies that apply --force backs up an
// existing regular file and replaces it with a symlink.
func TestFiles_S4_MismatchForceBackup(t *testing.T) {
	r := newFileRunner(t)
	r.writeFilesSpec(t, []fileEntry{
		{Source: "simple.txt", Target: "~/.genv-test/simple.txt", Mode: "link", Backup: true},
	})

	target := r.targetPath("~/.genv-test/simple.txt")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("mkdir target parent: %v", err)
	}
	original := []byte("i am a hand-written file\n")
	if err := os.WriteFile(target, original, 0o644); err != nil {
		t.Fatalf("write planted file: %v", err)
	}

	stdout, stderr, code := r.genv("", "apply", "--force", "--yes")
	if code != 0 {
		t.Fatalf("apply --force backup: exit %d, want 0\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}

	if fi, err := os.Lstat(target); err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Errorf("apply --force backup: target should be a symlink, got mode %v err %v", fi.Mode(), err)
	}

	matches, err := filepath.Glob(target + ".backup.*")
	if err != nil {
		t.Fatalf("glob backups: %v", err)
	}
	if len(matches) != 1 {
		t.Errorf("apply --force backup: expected exactly one backup file, got %v", matches)
	}

	stdout, stderr, code = r.genv("", "status", "--files")
	if code != 0 {
		t.Errorf("status --files after force apply: exit %d, want 0\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
}

// TestFiles_S5_CodexTemplatedDrift verifies that a copy-template target whose
// on-disk rendered content is stale is detected as mismatched, then fixed by
// apply --force with a backup.
func TestFiles_S5_CodexTemplatedDrift(t *testing.T) {
	r := newFileRunner(t)
	r.writeFilesSpec(t, []fileEntry{
		{Source: "codex-config.toml", Target: "~/.config/codex/config.toml", Mode: "copy-template", Template: true, Backup: true},
	})

	target := r.targetPath("~/.config/codex/config.toml")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("mkdir target parent: %v", err)
	}
	oldRendered := []byte("[core]\nhome = \"/old/home\"\nuser = \"olduser\"\n")
	if err := os.WriteFile(target, oldRendered, 0o644); err != nil {
		t.Fatalf("write old rendered file: %v", err)
	}

	stdout, stderr, code := r.genv("", "status", "--files")
	out := stdout + stderr
	if code != 4 {
		t.Errorf("status --files templated drift: exit %d, want 4\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	if !strings.Contains(out, "mismatch") {
		t.Errorf("status --files templated drift: expected 'mismatch' in output, got: %q", out)
	}

	stdout, stderr, code = r.genv("", "apply", "--force", "--yes")
	if code != 0 {
		t.Fatalf("apply --force templated drift: exit %d, want 0\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}

	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read rendered file after apply: %v", err)
	}
	if !strings.Contains(string(got), r.home) {
		t.Errorf("apply --force templated drift: rendered file should contain HOME %q, got: %q", r.home, got)
	}
	if strings.Contains(string(got), "__HOME__") {
		t.Errorf("apply --force templated drift: rendered file should not contain literal __HOME__, got: %q", got)
	}

	matches, err := filepath.Glob(target + ".backup.*")
	if err != nil {
		t.Fatalf("glob backups: %v", err)
	}
	if len(matches) != 1 {
		t.Errorf("apply --force templated drift: expected exactly one backup file, got %v", matches)
	}

	stdout, stderr, code = r.genv("", "status", "--files")
	if code != 0 {
		t.Errorf("status --files after templated fix: exit %d, want 0\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
}

// TestFiles_S6_DryRun verifies that apply --dry-run reports a planned change
// but writes nothing to disk.
func TestFiles_S6_DryRun(t *testing.T) {
	r := newFileRunner(t)
	r.writeFilesSpec(t, []fileEntry{
		{Source: "simple.txt", Target: "~/.genv-test/simple.txt", Mode: "link", Backup: true},
	})

	target := r.targetPath("~/.genv-test/simple.txt")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("mkdir target parent: %v", err)
	}
	original := []byte("i am a hand-written file\n")
	if err := os.WriteFile(target, original, 0o644); err != nil {
		t.Fatalf("write planted file: %v", err)
	}

	stdout, stderr, code := r.genv("", "apply", "--dry-run", "--force", "--yes")
	if code != 0 {
		t.Errorf("apply --dry-run: exit %d, want 0\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}

	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read planted file after dry-run: %v", err)
	}
	if !bytes.Equal(got, original) {
		t.Errorf("apply --dry-run: target was modified; want %q, got %q", original, got)
	}

	matches, err := filepath.Glob(target + ".backup.*")
	if err != nil {
		t.Fatalf("glob backups: %v", err)
	}
	if len(matches) != 0 {
		t.Errorf("apply --dry-run: expected no backup files, got %v", matches)
	}

	_ = stdout
	_ = stderr
}

func TestFiles_S7_AdoptFilesRegistersRenderedConfig(t *testing.T) {
	r := newFileRunner(t)
	r.writeFilesSpec(t, []fileEntry{
		{Source: "codex-config.toml", Target: "~/.config/codex/config.toml", Mode: "copy-template", Template: true, Backup: true},
	})

	target := r.targetPath("~/.config/codex/config.toml")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("mkdir target parent: %v", err)
	}
	rendered := "[core]\nhome = \"" + r.home + "\"\nuser = \"" + os.Getenv("USER") + "\"\n"
	if err := os.WriteFile(target, []byte(rendered), 0o644); err != nil {
		t.Fatalf("write rendered target: %v", err)
	}

	stdout, stderr, code := r.genv("", "adopt", "--files")
	if code != 0 {
		t.Fatalf("adopt --files: exit %d, want 0\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	matches, err := filepath.Glob(target + ".backup.*")
	if err != nil {
		t.Fatalf("glob backups: %v", err)
	}
	if len(matches) != 0 {
		t.Fatalf("adopt --files created backups: %v", matches)
	}
	lockPath := filepath.Join(r.home, ".config", "genv", "genv.lock.json")
	lf, err := genvfile.ReadLock(lockPath)
	if err != nil {
		t.Fatalf("read lock: %v", err)
	}
	if len(lf.Files) != 1 || lf.Files[0].Mode != "copy" || lf.Files[0].Target != "~/.config/codex/config.toml" {
		t.Fatalf("locked files = %+v, want one copied codex config", lf.Files)
	}

	stdout, stderr, code = r.genv("", "status", "--files")
	if code != 0 {
		t.Fatalf("status --files after adopt: exit %d, want 0\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
}

func TestFiles_S8_AdoptFilesKeepsSpecRepoClean(t *testing.T) {
	r := newFileRunner(t)
	specRepo := t.TempDir()
	if out, err := exec.Command("git", "init", specRepo).CombinedOutput(); err != nil {
		t.Skipf("git init unavailable: %v: %s", err, out)
	}
	source := filepath.Join(specRepo, "simple.txt")
	if err := os.WriteFile(source, []byte("hello"), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}
	target := r.targetPath("~/.genv-test/simple.txt")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("mkdir target parent: %v", err)
	}
	if err := os.Symlink(source, target); err != nil {
		t.Fatalf("symlink target: %v", err)
	}
	r.genvJSON = filepath.Join(specRepo, "genv.json")
	spec := map[string]any{
		"schemaVersion": "5",
		"repo":          map[string]string{"url": specRepo},
		"files": map[string]any{
			"links": []map[string]any{{"source": "simple.txt", "target": "~/.genv-test/simple.txt"}},
		},
	}
	data, err := json.MarshalIndent(spec, "", "  ")
	if err != nil {
		t.Fatalf("marshal spec: %v", err)
	}
	if err := os.WriteFile(r.genvJSON, append(data, '\n'), 0o644); err != nil {
		t.Fatalf("write spec: %v", err)
	}
	if out, err := exec.Command("git", "-C", specRepo, "add", ".").CombinedOutput(); err != nil {
		t.Fatalf("git add: %v: %s", err, out)
	}
	if out, err := exec.Command(
		"git",
		"-C", specRepo,
		"-c", "user.name=genv e2e",
		"-c", "user.email=genv-e2e@example.invalid",
		"-c", "commit.gpgsign=false",
		"-c", "tag.gpgsign=false",
		"commit",
		"--no-gpg-sign",
		"-m", "fixture",
	).CombinedOutput(); err != nil {
		t.Skipf("git commit unavailable: %v: %s", err, out)
	}

	stdout, stderr, code := r.genv("", "adopt", "--files")
	if code != 0 {
		t.Fatalf("adopt --files: exit %d, want 0\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	out, err := exec.Command("git", "-C", specRepo, "status", "--porcelain").CombinedOutput()
	if err != nil {
		t.Fatalf("git status: %v: %s", err, out)
	}
	if strings.TrimSpace(string(out)) != "" {
		t.Fatalf("spec repo dirty after adopt --files: %q", out)
	}
}
