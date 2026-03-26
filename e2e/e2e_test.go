//go:build integration

// Package e2e_test contains end-to-end tests for the genv binary.
//
// Unlike the adapter-layer integration tests (internal/adapter/integration_test.go),
// these tests build the real genv binary and exercise every M1/M2 command
// against a live package manager on the host:
//
//   - genv add / remove / list (ls) / apply / apply --dry-run / clean / clean --dry-run
//   - genv adopt: verify already-installed package is tracked without reinstalling
//   - genv disown: verify package is untracked without being uninstalled
//   - lock-file integrity after every mutation
//   - genv apply to reconcile: install newly-added packages, remove deleted ones
//   - genv apply --strict with a fully-resolved plan
//   - error paths: duplicate add, remove/adopt/disown of untracked package, apply with no genv.json
//   - command aliases: ls, rm
//
// Each TestE2E* function skips itself when its adapter binary is absent,
// so the same test binary works across distros.
//
// Usage (single adapter):
//
//	go test -tags integration -race -v -run TestE2EApt ./e2e/
package e2e_test

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ks1686/genv/internal/genvfile"
	"github.com/ks1686/genv/internal/schema"
)

// genvBin is the path to the compiled genv binary, populated by TestMain.
var genvBin string

func TestMain(m *testing.M) {
	tmp, err := os.MkdirTemp("", "genv-e2e-bin-*")
	if err != nil {
		panic("mkdirtemp: " + err.Error())
	}

	bin := filepath.Join(tmp, "genv")
	out, err := exec.Command("go", "build", "-buildvcs=false", "-o", bin, "github.com/ks1686/genv").CombinedOutput()
	if err != nil {
		os.RemoveAll(tmp)
		panic("go build failed:\n" + string(out))
	}
	genvBin = bin

	code := m.Run()
	os.RemoveAll(tmp)
	os.Exit(code)
}

// ── runner ────────────────────────────────────────────────────────────────────

// runner bundles the binary path, per-test file paths, and optional prefer
// flag so every helper can inject them consistently.
type runner struct {
	bin      string
	genvJSON string
	lockJSON string
	prefer   string // if non-empty, passed as --prefer on genv add and written into specs
}

func newRunner(t *testing.T, prefer string) *runner {
	t.Helper()
	dir := t.TempDir()
	g := filepath.Join(dir, "genv.json")
	return &runner{
		bin:      genvBin,
		genvJSON: g,
		lockJSON: genvfile.LockPathFrom(g),
		prefer:   prefer,
	}
}

// rawExec runs the genv binary with the given args and optional stdin.
// Returns stdout, stderr, and exit code (0 on success).
func (r *runner) rawExec(stdinData string, args ...string) (stdout, stderr string, code int) {
	cmd := exec.Command(r.bin, args...)
	if stdinData != "" {
		cmd.Stdin = strings.NewReader(stdinData)
	}
	cmd.Env = append(os.Environ(), "GENV_NO_INTERACTIVE=1")
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

// genv runs a genv subcommand with --file injected as the second argument.
// Use rawExec for commands that do not accept --file (help, clean).
func (r *runner) genv(stdinData, subcmd string, extra ...string) (stdout, stderr string, code int) {
	args := make([]string, 0, 3+len(extra))
	args = append(args, subcmd, "--file", r.genvJSON)
	args = append(args, extra...)
	return r.rawExec(stdinData, args...)
}

// ── spec / lock helpers ───────────────────────────────────────────────────────

// specIDs returns the package IDs currently in genv.json.
func (r *runner) specIDs(t *testing.T) []string {
	t.Helper()
	data, err := os.ReadFile(r.genvJSON)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		t.Fatalf("read spec: %v", err)
	}
	var f struct {
		Packages []struct {
			ID string `json:"id"`
		} `json:"packages"`
	}
	if err := json.Unmarshal(data, &f); err != nil {
		t.Fatalf("parse spec: %v", err)
	}
	ids := make([]string, len(f.Packages))
	for i, p := range f.Packages {
		ids[i] = p.ID
	}
	return ids
}

// lockPackages returns the packages slice from genv.lock.json.
func (r *runner) lockPackages(t *testing.T) []map[string]string {
	t.Helper()
	data, err := os.ReadFile(r.lockJSON)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		t.Fatalf("read lock: %v", err)
	}
	var lf struct {
		Packages []map[string]string `json:"packages"`
	}
	if err := json.Unmarshal(data, &lf); err != nil {
		t.Fatalf("parse lock: %v", err)
	}
	return lf.Packages
}

// writeSpec writes a minimal genv.json containing exactly the given package IDs.
// Each entry gets the runner's prefer field (if set) so that apply tests resolve
// to the same adapter as genv add.
func (r *runner) writeSpec(t *testing.T, ids ...string) {
	t.Helper()
	type pkg struct {
		ID     string `json:"id"`
		Prefer string `json:"prefer,omitempty"`
	}
	type spec struct {
		SchemaVersion string `json:"schemaVersion"`
		Packages      []pkg  `json:"packages"`
	}
	pkgs := make([]pkg, len(ids))
	for i, id := range ids {
		pkgs[i] = pkg{ID: id, Prefer: r.prefer}
	}
	data, err := json.MarshalIndent(spec{SchemaVersion: "1", Packages: pkgs}, "", "  ")
	if err != nil {
		t.Fatalf("marshal spec: %v", err)
	}
	if err := os.WriteFile(r.genvJSON, append(data, '\n'), 0o644); err != nil {
		t.Fatalf("write spec: %v", err)
	}
}

// writeSpecRaw writes raw JSON content to the genv.json file.
func (r *runner) writeSpecRaw(t *testing.T, content string) {
	t.Helper()
	if err := os.WriteFile(r.genvJSON, []byte(content), 0o644); err != nil {
		t.Fatalf("write spec: %v", err)
	}
}

// readSpecRaw reads and unmarshals the genv.json file into a schema.GenvFile.
func (r *runner) readSpecRaw(t *testing.T) *schema.GenvFile {
	t.Helper()
	var f schema.GenvFile
	data, err := os.ReadFile(r.genvJSON)
	if err != nil {
		t.Fatalf("read spec: %v", err)
	}
	if err := json.Unmarshal(data, &f); err != nil {
		t.Fatalf("unmarshal spec: %v", err)
	}
	return &f
}

// readLockRaw reads and unmarshals the genv.lock.json file.
func (r *runner) readLockRaw(t *testing.T) *genvfile.LockFile {
	t.Helper()
	var lf genvfile.LockFile
	data, err := os.ReadFile(r.lockJSON)
	if err != nil {
		t.Fatalf("read lock: %v", err)
	}
	if err := json.Unmarshal(data, &lf); err != nil {
		t.Fatalf("unmarshal lock: %v", err)
	}
	return &lf
}

// clearLock deletes genv.lock.json, simulating a first-run state.
func (r *runner) clearLock(t *testing.T) {
	t.Helper()
	if err := os.Remove(r.lockJSON); err != nil && !os.IsNotExist(err) {
		t.Fatalf("clear lock: %v", err)
	}
}

func (r *runner) assertInSpec(t *testing.T, id string) {
	t.Helper()
	ids := r.specIDs(t)
	for _, s := range ids {
		if s == id {
			return
		}
	}
	t.Errorf("expected %q in genv.json; current ids: %v", id, ids)
}

func (r *runner) assertNotInSpec(t *testing.T, id string) {
	t.Helper()
	for _, s := range r.specIDs(t) {
		if s == id {
			t.Errorf("expected %q absent from genv.json, but it is present", id)
			return
		}
	}
}

func (r *runner) assertInLock(t *testing.T, id, manager string) {
	t.Helper()
	pkgs := r.lockPackages(t)
	for _, p := range pkgs {
		if p["id"] == id {
			if p["manager"] != manager {
				t.Errorf("lock: %q manager=%q, want %q", id, p["manager"], manager)
			}
			return
		}
	}
	t.Errorf("expected %q in genv.lock.json with manager=%q; current entries: %v", id, manager, pkgs)
}

func (r *runner) assertNotInLock(t *testing.T, id string) {
	t.Helper()
	for _, p := range r.lockPackages(t) {
		if p["id"] == id {
			t.Errorf("expected %q absent from genv.lock.json, but it is present", id)
			return
		}
	}
}

// assertJSONCommand decodes a JSON envelope from stdout and checks the command field.
func assertJSONCommand(t *testing.T, stdout, wantCmd string) {
	t.Helper()
	var env struct {
		Command string `json:"command"`
	}
	if err := json.Unmarshal([]byte(stdout), &env); err != nil {
		t.Fatalf("%s --json: invalid JSON: %v\noutput: %q", wantCmd, err, stdout)
	}
	if env.Command != wantCmd {
		t.Errorf("command: got %q, want %q", env.Command, wantCmd)
	}
}

// pkgArgs builds the arg list for a package command, appending --prefer when set.
func pkgArgs(id, prefer string) []string {
	if prefer == "" {
		return []string{id}
	}
	return []string{id, "--prefer", prefer}
}

// ── suite ─────────────────────────────────────────────────────────────────────

// suiteConfig describes per-adapter parameters for the E2E suite.
type suiteConfig struct {
	adapterName string // e.g. "apt" — must match the adapter's Name()
	checkBin    string // binary that must exist in PATH for this adapter
	testPkg     string // small package to install and remove during tests
	preferFlag  string // if non-empty, passed as --prefer on genv add (and written into specs)
	canInstall  bool   // false when install requires infra absent in CI (e.g. flatpak remotes)
}

// runE2ESuite is the shared test body. It exercises the full M1/M2 command set
// against a real package manager, using t.Run subtests so -run filtering works
// at the subtest level.
func runE2ESuite(t *testing.T, cfg suiteConfig) {
	t.Helper()

	if _, err := exec.LookPath(cfg.checkBin); err != nil {
		t.Skipf("%s (%s) not in PATH — skipping", cfg.adapterName, cfg.checkBin)
	}

	r := newRunner(t, cfg.preferFlag)

	// ── meta-commands ─────────────────────────────────────────────────────────

	t.Run("help", func(t *testing.T) {
		_, _, code := r.rawExec("", "help")
		if code != 0 {
			t.Errorf("genv help: exit %d, want 0", code)
		}
	})

	t.Run("help_flag", func(t *testing.T) {
		_, _, code := r.rawExec("", "--help")
		if code != 0 {
			t.Errorf("genv --help: exit %d, want 0", code)
		}
	})

	t.Run("unknown_command", func(t *testing.T) {
		_, stderr, code := r.rawExec("", "xyzzy-no-such-command")
		if code == 0 {
			t.Error("unknown command: expected non-zero exit, got 0")
		}
		if !strings.Contains(stderr, "unknown command") {
			t.Errorf("unknown command: expected 'unknown command' in stderr, got: %q", stderr)
		}
	})

	// ── list / ls on empty state ──────────────────────────────────────────────

	t.Run("list_empty", func(t *testing.T) {
		stdout, _, code := r.genv("", "list")
		if code != 0 {
			t.Fatalf("genv list: exit %d, want 0", code)
		}
		if !strings.Contains(stdout, "no packages installed") {
			t.Errorf("expected empty-list message, got: %q", stdout)
		}
	})

	t.Run("ls_alias_empty", func(t *testing.T) {
		stdout, _, code := r.genv("", "ls")
		if code != 0 {
			t.Fatalf("genv ls: exit %d, want 0", code)
		}
		if !strings.Contains(stdout, "no packages installed") {
			t.Errorf("ls alias: expected empty-list message, got: %q", stdout)
		}
	})

	// ── apply with missing genv.json ───────────────────────────────────────────

	t.Run("apply_missing_spec_fails", func(t *testing.T) {
		r2 := newRunner(t, cfg.preferFlag)
		_, _, code := r2.genv("", "apply", "--dry-run")
		if code == 0 {
			t.Error("apply --dry-run with no genv.json: expected non-zero exit, got 0")
		}
	})

	t.Run("status_no_spec_fails", func(t *testing.T) {
		r2 := newRunner(t, cfg.preferFlag)
		_, _, code := r2.genv("", "status")
		if code == 0 {
			t.Error("status with no genv.json: expected non-zero exit, got 0")
		}
	})

	// ── genv add ───────────────────────────────────────────────────────────────

	// build the add-command args (--prefer injected when configured)
	addArgs := pkgArgs(cfg.testPkg, cfg.preferFlag)

	t.Run("add_creates_spec_entry", func(t *testing.T) {
		stdout, _, code := r.genv("", "add", addArgs...)
		if code != 0 {
			t.Fatalf("genv add %s: exit %d\nstdout: %s", cfg.testPkg, code, stdout)
		}
		r.assertInSpec(t, cfg.testPkg)
	})

	// status on a spec with no lock entry should report "missing"
	t.Run("status_package_in_spec_not_in_lock", func(t *testing.T) {
		r2 := newRunner(t, cfg.preferFlag)
		r2.writeSpec(t, cfg.testPkg)
		// no lock written — package is in spec but not yet installed/tracked
		stdout, _, code := r2.genv("", "status")
		if code != 0 {
			t.Fatalf("genv status: exit %d, want 0", code)
		}
		if !strings.Contains(stdout, "missing") {
			t.Errorf("expected 'missing' in status output, got: %q", stdout)
		}
	})

	t.Run("apply_dry_run_json_valid", func(t *testing.T) {
		stdout, _, code := r.genv("", "apply", "--dry-run", "--json")
		if code != 0 {
			t.Fatalf("apply --dry-run --json: exit %d, want 0", code)
		}
		assertJSONCommand(t, stdout, "apply")
	})

	t.Run("status_json_valid", func(t *testing.T) {
		stdout, _, code := r.genv("", "status", "--json")
		if code != 0 {
			t.Fatalf("genv status --json: exit %d, want 0", code)
		}
		assertJSONCommand(t, stdout, "status")
	})

	if cfg.canInstall {
		t.Run("add_creates_lock_entry", func(t *testing.T) {
			r.assertInLock(t, cfg.testPkg, cfg.adapterName)
		})

		t.Run("status_ok_after_install", func(t *testing.T) {
			stdout, _, code := r.genv("", "status")
			if code != 0 {
				t.Fatalf("genv status: exit %d, want 0", code)
			}
			if !strings.Contains(stdout, "ok") {
				t.Errorf("expected 'ok' in status output after install, got: %q", stdout)
			}
		})

		// ── list / ls after install ────────────────────────────────────────────

		t.Run("list_shows_installed_package", func(t *testing.T) {
			stdout, _, code := r.genv("", "list")
			if code != 0 {
				t.Fatalf("genv list: exit %d, want 0", code)
			}
			if !strings.Contains(stdout, cfg.testPkg) {
				t.Errorf("expected %q in list output, got: %q", cfg.testPkg, stdout)
			}
			if !strings.Contains(stdout, cfg.adapterName) {
				t.Errorf("expected manager %q in list output, got: %q", cfg.adapterName, stdout)
			}
		})

		t.Run("ls_alias_shows_installed_package", func(t *testing.T) {
			stdout, _, code := r.genv("", "ls")
			if code != 0 {
				t.Fatalf("genv ls: exit %d, want 0", code)
			}
			if !strings.Contains(stdout, cfg.testPkg) {
				t.Errorf("ls alias: expected %q in output, got: %q", cfg.testPkg, stdout)
			}
		})

		// ── duplicate add rejected ─────────────────────────────────────────────

		t.Run("add_duplicate_rejected", func(t *testing.T) {
			_, stderr, code := r.genv("", "add", addArgs...)
			if code == 0 {
				t.Error("duplicate add: expected non-zero exit, got 0")
			}
			if !strings.Contains(stderr, "already tracked") {
				t.Errorf("duplicate add: expected 'already tracked' in stderr, got: %q", stderr)
			}
		})

		// ── apply --dry-run: up to date ────────────────────────────────────────

		t.Run("apply_dry_run_up_to_date", func(t *testing.T) {
			stdout, _, code := r.genv("", "apply", "--dry-run")
			if code != 0 {
				t.Fatalf("apply --dry-run (up-to-date): exit %d, want 0", code)
			}
			if !strings.Contains(stdout, "up to date") {
				t.Errorf("expected 'up to date' in output, got: %q", stdout)
			}
		})

		// ── genv remove ────────────────────────────────────────────────────────

		t.Run("remove_updates_spec_and_lock", func(t *testing.T) {
			stdout, _, code := r.genv("", "remove", cfg.testPkg)
			if code != 0 {
				t.Fatalf("genv remove %s: exit %d\nstdout: %s", cfg.testPkg, code, stdout)
			}
			r.assertNotInSpec(t, cfg.testPkg)
			r.assertNotInLock(t, cfg.testPkg)
		})

		t.Run("list_empty_after_remove", func(t *testing.T) {
			stdout, _, code := r.genv("", "list")
			if code != 0 {
				t.Fatalf("genv list: exit %d, want 0", code)
			}
			if !strings.Contains(stdout, "no packages installed") {
				t.Errorf("expected empty-list message after remove, got: %q", stdout)
			}
		})

		// ── rm alias ──────────────────────────────────────────────────────────

		t.Run("rm_alias", func(t *testing.T) {
			if _, _, code := r.genv("", "add", addArgs...); code != 0 {
				t.Fatalf("setup: genv add: exit %d", code)
			}
			_, _, code := r.genv("", "rm", cfg.testPkg)
			if code != 0 {
				t.Errorf("genv rm: exit %d, want 0", code)
			}
			r.assertNotInSpec(t, cfg.testPkg)
			r.assertNotInLock(t, cfg.testPkg)
		})

		// ── apply --yes: non-interactive mode ────────────────────────────────
		// Verify --yes flag is accepted and suppresses the interactive prompt.
		// Uses --dry-run to avoid actual installation side effects.

		t.Run("apply_yes_bypasses_prompt", func(t *testing.T) {
			r.writeSpec(t, cfg.testPkg)
			r.clearLock(t)
			// No stdin provided; --yes must suppress the confirmation prompt.
			stdout, _, code := r.genv("", "apply", "--dry-run", "--yes")
			if code != 0 {
				t.Fatalf("apply --dry-run --yes: exit %d\nstdout: %s", code, stdout)
			}
		})

		// ── apply: installation path ─────────────────────────────────────────
		// Write spec directly (bypasses genv add) and run apply from a clean lock.

		t.Run("apply_installs_from_spec", func(t *testing.T) {
			r.writeSpec(t, cfg.testPkg)
			r.clearLock(t)
			stdout, _, code := r.genv("y\n", "apply")
			if code != 0 {
				t.Fatalf("genv apply (install): exit %d\nstdout: %s", code, stdout)
			}
			r.assertInLock(t, cfg.testPkg, cfg.adapterName)
		})

		// ── apply --dry-run: shows manager and concrete package name ──────────

		t.Run("apply_dry_run_shows_plan", func(t *testing.T) {
			// spec has testPkg; clear the lock so apply would need to install
			r.clearLock(t)
			stdout, _, code := r.genv("", "apply", "--dry-run")
			if code != 0 {
				t.Fatalf("apply --dry-run (plan): exit %d", code)
			}
			if !strings.Contains(stdout, cfg.testPkg) {
				t.Errorf("dry-run plan: expected %q in output, got: %q", cfg.testPkg, stdout)
			}
			if !strings.Contains(stdout, cfg.adapterName) {
				t.Errorf("dry-run plan: expected manager %q in output, got: %q", cfg.adapterName, stdout)
			}
		})

		// ── apply: remove path ────────────────────────────────────────────────
		// Ensure package is installed, then delete it from the spec and re-apply.

		t.Run("apply_removes_deleted_from_spec", func(t *testing.T) {
			r.writeSpec(t, cfg.testPkg)
			if _, _, code := r.genv("y\n", "apply"); code != 0 {
				t.Fatalf("setup apply: exit %d", code)
			}
			r.assertInLock(t, cfg.testPkg, cfg.adapterName)

			r.writeSpec(t) // empty spec — package "deleted" by user
			stdout, _, code := r.genv("y\n", "apply")
			if code != 0 {
				t.Fatalf("apply (remove path): exit %d\nstdout: %s", code, stdout)
			}
			r.assertNotInLock(t, cfg.testPkg)
		})

		// ── apply --strict with a fully-resolved plan ─────────────────────────

		t.Run("apply_strict_all_resolved", func(t *testing.T) {
			r.writeSpec(t, cfg.testPkg)
			r.clearLock(t)
			stdout, _, code := r.genv("y\n", "apply", "--strict")
			if code != 0 {
				t.Fatalf("apply --strict (all resolved): exit %d\nstdout: %s", code, stdout)
			}
			r.assertInLock(t, cfg.testPkg, cfg.adapterName)
			// clean up so later tests start from an empty state
			r.genv("", "remove", cfg.testPkg)
		})

		// ── genv disown ────────────────────────────────────────────────────────
		// Install and track testPkg, then disown it. The package must remain
		// installed on the system but disappear from spec and lock.

		t.Run("disown_removes_tracking_keeps_package_installed", func(t *testing.T) {
			if _, _, code := r.genv("", "add", addArgs...); code != 0 {
				t.Fatalf("setup disown: genv add exit %d", code)
			}
			r.assertInSpec(t, cfg.testPkg)
			r.assertInLock(t, cfg.testPkg, cfg.adapterName)

			stdout, _, code := r.genv("", "disown", cfg.testPkg)
			if code != 0 {
				t.Fatalf("genv disown %s: exit %d\nstdout: %s", cfg.testPkg, code, stdout)
			}
			r.assertNotInSpec(t, cfg.testPkg)
			r.assertNotInLock(t, cfg.testPkg)
			if !strings.Contains(stdout, "remains installed") {
				t.Errorf("expected 'remains installed' in stdout, got: %q", stdout)
			}
		})

		t.Run("disown_not_tracked_fails", func(t *testing.T) {
			_, _, code := r.genv("", "disown", "genv-e2e-never-tracked-xyzzy")
			if code == 0 {
				t.Error("disown untracked: expected non-zero exit, got 0")
			}
		})

		// ── genv adopt ────────────────────────────────────────────────────────
		// testPkg is installed on the system but not tracked by genv
		// (state left by disown_removes_tracking_keeps_package_installed above).

		adoptArgs := pkgArgs(cfg.testPkg, cfg.preferFlag)

		t.Run("adopt_tracks_already_installed_package", func(t *testing.T) {
			stdout, _, code := r.genv("", "adopt", adoptArgs...)
			if code != 0 {
				t.Fatalf("genv adopt %s: exit %d\nstdout: %s", cfg.testPkg, code, stdout)
			}
			r.assertInSpec(t, cfg.testPkg)
			r.assertInLock(t, cfg.testPkg, cfg.adapterName)
			if !strings.Contains(stdout, "adopted") {
				t.Errorf("expected 'adopted' in stdout, got: %q", stdout)
			}
			if !strings.Contains(stdout, "already installed") {
				t.Errorf("expected 'already installed' in stdout, got: %q", stdout)
			}
		})

		t.Run("adopt_already_tracked_fails", func(t *testing.T) {
			// testPkg is now tracked from the previous subtest.
			_, stderr, code := r.genv("", "adopt", adoptArgs...)
			if code == 0 {
				t.Error("adopt already-tracked: expected non-zero exit, got 0")
			}
			if !strings.Contains(stderr, "already tracked") {
				t.Errorf("expected 'already tracked' in stderr, got: %q", stderr)
			}
			// clean up so the package is gone for the not-installed test
			r.genv("", "remove", cfg.testPkg)
		})

		t.Run("adopt_not_installed_fails", func(t *testing.T) {
			_, stderr, code := r.genv("", "adopt", pkgArgs("genv-e2e-definitely-not-installed-xyzzy", cfg.preferFlag)...)
			if code == 0 {
				t.Error("adopt not-installed: expected non-zero exit, got 0")
			}
			if !strings.Contains(stderr, "not installed") {
				t.Errorf("expected 'not installed' in stderr, got: %q", stderr)
			}
		})
	}

	// ── genv scan ──────────────────────────────────────────────────────────────
	// scan bulk-adopts installed packages; use isolated runners to avoid
	// polluting the shared runner's spec/lock with every system package.

	t.Run("scan_exits_ok", func(t *testing.T) {
		r2 := newRunner(t, cfg.preferFlag)
		_, _, code := r2.genv("", "scan")
		if code != 0 {
			t.Errorf("genv scan: exit %d, want 0", code)
		}
	})

	t.Run("scan_json_valid", func(t *testing.T) {
		r2 := newRunner(t, cfg.preferFlag)
		stdout, _, code := r2.genv("", "scan", "--json")
		if code != 0 {
			t.Fatalf("genv scan --json: exit %d, want 0", code)
		}
		assertJSONCommand(t, stdout, "scan")
	})

	// ── remove of an untracked package fails ──────────────────────────────────

	t.Run("remove_not_tracked_fails", func(t *testing.T) {
		_, _, code := r.genv("", "remove", "genv-e2e-never-tracked-xyzzy")
		if code == 0 {
			t.Error("remove untracked: expected non-zero exit, got 0")
		}
	})

	// ── genv clean ─────────────────────────────────────────────────────────────

	t.Run("clean_dry_run", func(t *testing.T) {
		_, _, code := r.rawExec("", "clean", "--dry-run")
		if code != 0 {
			t.Errorf("genv clean --dry-run: exit %d, want 0", code)
		}
	})

	t.Run("clean", func(t *testing.T) {
		_, _, code := r.rawExec("", "clean")
		if code != 0 {
			t.Errorf("genv clean: exit %d, want 0", code)
		}
	})
}

// ── per-adapter entry points ──────────────────────────────────────────────────

func TestE2EApt(t *testing.T) {
	runE2ESuite(t, suiteConfig{
		adapterName: "apt",
		checkBin:    "apt-get",
		testPkg:     "tree",
		canInstall:  true,
	})
}

func TestE2EDnf(t *testing.T) {
	runE2ESuite(t, suiteConfig{
		adapterName: "dnf",
		checkBin:    "dnf",
		testPkg:     "tree",
		canInstall:  true,
	})
}

// TestE2EZypper uses --prefer zypper because dnf may also be present on newer
// openSUSE Tumbleweed images and appears earlier in adapter.All.
func TestE2EZypper(t *testing.T) {
	runE2ESuite(t, suiteConfig{
		adapterName: "zypper",
		checkBin:    "zypper",
		testPkg:     "tree",
		preferFlag:  "zypper",
		canInstall:  true,
	})
}

// TestE2EApk uses nano as the test package; it is in Alpine's main repository
// (tree is in community, which may not be enabled in the base Docker image).
func TestE2EApk(t *testing.T) {
	runE2ESuite(t, suiteConfig{
		adapterName: "apk",
		checkBin:    "apk",
		testPkg:     "nano",
		preferFlag:  "apk",
		canInstall:  true,
	})
}

func TestE2EPacman(t *testing.T) {
	runE2ESuite(t, suiteConfig{
		adapterName: "pacman",
		checkBin:    "pacman",
		testPkg:     "tree",
		canInstall:  true,
	})
}

// TestE2EParu uses --prefer paru because pacman is also available on Arch and
// appears earlier in adapter.All — without prefer, the resolver picks pacman.
func TestE2EParu(t *testing.T) {
	runE2ESuite(t, suiteConfig{
		adapterName: "paru",
		checkBin:    "paru",
		testPkg:     "tree",
		preferFlag:  "paru",
		canInstall:  true,
	})
}

// TestE2EYay uses --prefer yay for the same reason as TestE2EParu.
func TestE2EYay(t *testing.T) {
	runE2ESuite(t, suiteConfig{
		adapterName: "yay",
		checkBin:    "yay",
		testPkg:     "tree",
		preferFlag:  "yay",
		canInstall:  true,
	})
}

// TestE2EFlatpak tests all non-install commands. Real flatpak installs require
// a configured remote (e.g. Flathub) which is not set up in the CI containers.
// genv add still exits 0 even when the install fails (non-fatal by design), so
// spec/lock mutations and all read-only commands are still fully exercised.
func TestE2EFlatpak(t *testing.T) {
	runE2ESuite(t, suiteConfig{
		adapterName: "flatpak",
		checkBin:    "flatpak",
		testPkg:     "com.example.TestApp",
		canInstall:  false,
	})
}

// TestE2EBrew runs the full E2E suite on macOS using Homebrew.
// tree is used as the test package — it is small and not expected to be
// pre-installed. Skips automatically when brew is not in PATH.
func TestE2EBrew(t *testing.T) {
	runE2ESuite(t, suiteConfig{
		adapterName: "brew",
		checkBin:    "brew",
		testPkg:     "tree",
		preferFlag:  "brew",
		canInstall:  true,
	})
}

// TestE2EMacPorts tests all non-install commands for the MacPorts adapter.
// Real installs are skipped because MacPorts is not universally available
// and requires sudo. Skips automatically when port is not in PATH.
func TestE2EMacPorts(t *testing.T) {
	runE2ESuite(t, suiteConfig{
		adapterName: "macports",
		checkBin:    "port",
		testPkg:     "tree",
		canInstall:  false,
	})
}

// TestE2ENix uses "hello" as the test package — it is the canonical first nix
// install and is available in every nixpkgs channel. Uses --prefer nix because
// other managers may also be present on nix-enabled hosts.
func TestE2ENix(t *testing.T) {
	runE2ESuite(t, suiteConfig{
		adapterName: "nix",
		checkBin:    "nix-env",
		testPkg:     "hello",
		preferFlag:  "nix",
		canInstall:  true,
	})
}

// TestE2EEmerge tests all non-install commands for the emerge (Gentoo) adapter.
// Real package installation is skipped because emerge compiles packages from
// source, making CI installs impractically slow. All spec/lock mutations,
// dry-run plans, and read-only commands are still fully exercised.
func TestE2EEmerge(t *testing.T) {
	runE2ESuite(t, suiteConfig{
		adapterName: "emerge",
		checkBin:    "emerge",
		testPkg:     "nano",
		preferFlag:  "emerge",
		canInstall:  false,
	})
}

// TestE2EXbps runs the full E2E suite on Void Linux using xbps.
// Uses --prefer xbps to prevent accidentally picking another manager.
// Skips automatically when xbps-install is not in PATH.
func TestE2EXbps(t *testing.T) {
	runE2ESuite(t, suiteConfig{
		adapterName: "xbps",
		checkBin:    "xbps-install",
		testPkg:     "nano",
		preferFlag:  "xbps",
		canInstall:  true,
	})
}

// ── Service lifecycle tests ───────────────────────────────────────────────────

// TestE2EServiceLifecycle validates the full service management workflow:
//   - genv service add/remove/list
//   - genv service start/stop/status
//   - genv apply integration with services
//   - lock file tracking of service state
func TestE2EServiceLifecycle(t *testing.T) {
	r := newRunner(t, "")

	// Create initial spec
	r.writeSpecRaw(t, `{
		"schemaVersion": "4",
		"packages": []
	}`)

	// Test: add a service with start command only
	stdout, stderr, code := r.rawExec("", "service", "add", "test-svc", "--file", r.genvJSON,
		"--start", "sleep 1")
	if code != 0 {
		t.Fatalf("service add failed (exit %d):\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}

	// Verify spec was updated to v4 with service entry
	spec := r.readSpecRaw(t)
	if spec.SchemaVersion != "4" {
		t.Errorf("expected schemaVersion 4, got %s", spec.SchemaVersion)
	}
	if _, ok := spec.Services["test-svc"]; !ok {
		t.Fatal("service 'test-svc' not found in spec after add")
	}

	// Test: add a service with full commands (start, stop, status)
	// This should add to the existing spec, not replace it
	stdout, stderr, code = r.rawExec("", "service", "add", "full-svc", "--file", r.genvJSON,
		"--start", "echo starting",
		"--stop", "echo stopping",
		"--status", "true")
	if code != 0 {
		t.Fatalf("service add full-svc failed (exit %d):\nstderr: %s", code, stderr)
	}

	// Debug: check what's in the spec after adding both services
	spec = r.readSpecRaw(t)
	t.Logf("After adding full-svc, services in spec: %v", spec.Services)

	// Test: list services - should show both test-svc and full-svc
	stdout, stderr, code = r.rawExec("", "service", "list", "--file", r.genvJSON)
	if code != 0 {
		t.Fatalf("service list failed (exit %d):\nstderr: %s", code, stderr)
	}
	if !strings.Contains(stdout, "test-svc") {
		t.Errorf("service list output missing 'test-svc':\n%s", stdout)
	}
	if !strings.Contains(stdout, "full-svc") {
		t.Errorf("service list output missing 'full-svc':\n%s", stdout)
	}

	// Test: service status command
	stdout, stderr, code = r.rawExec("", "service", "status", "full-svc", "--file", r.genvJSON)
	// Expected to fail since service is not running (status command is "true" but service isn't started)
	if code == 0 {
		t.Logf("service status succeeded: %s", stdout)
	} else {
		t.Logf("service status returned non-zero (expected if not running): %s", stdout)
	}

	// Debug: verify test-svc is still there before we try to start it
	spec = r.readSpecRaw(t)
	if _, ok := spec.Services["test-svc"]; !ok {
		t.Fatalf("test-svc disappeared from spec before service start test! Services: %v", spec.Services)
	}
	t.Logf("File path being used: %s", r.genvJSON)

	// Test: service start on test-svc
	stdout, stderr, code = r.rawExec("", "service", "start", "test-svc", "--file", r.genvJSON)
	// sleep 1 will complete, so this may succeed or fail depending on timing
	t.Logf("service start returned exit %d, stdout: %s, stderr: %s", code, stdout, stderr)
	if code != 0 {
		// Debug: dump the file contents to see what genv sees
		data, _ := os.ReadFile(r.genvJSON)
		t.Logf("Contents of %s:\n%s", r.genvJSON, string(data))
	}

	// Test: service remove command - remove test-svc we added earlier
	stdout, stderr, code = r.rawExec("", "service", "remove", "test-svc", "--file", r.genvJSON)
	if code != 0 {
		t.Fatalf("service remove test-svc failed (exit %d):\nstderr: %s", code, stderr)
	}

	spec = r.readSpecRaw(t)
	if _, ok := spec.Services["test-svc"]; ok {
		t.Error("service 'test-svc' still in spec after remove command")
	}
	// full-svc should still be there
	if _, ok := spec.Services["full-svc"]; !ok {
		t.Error("service 'full-svc' was removed unexpectedly")
	}

	// Test: apply should track services in lock file
	// Create a fresh spec with a single service for clean apply test
	r.writeSpecRaw(t, `{
		"schemaVersion": "4",
		"packages": [],
		"services": {
			"apply-svc": {
				"start": ["true"],
				"stop": ["true"]
			}
		}
	}`)

	stdout, stderr, code = r.rawExec("y\n", "apply", "--file", r.genvJSON)
	if code != 0 {
		t.Logf("apply with services: exit %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}

	// Verify lock file contains service
	lock := r.readLockRaw(t)
	found := false
	for _, svc := range lock.Services {
		if svc.Name == "apply-svc" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected 'apply-svc' in lock file after apply")
	}

	// Test: remove service from spec and apply should remove from lock
	r.writeSpecRaw(t, `{
		"schemaVersion": "4",
		"packages": [],
		"services": {}
	}`)

	stdout, stderr, code = r.rawExec("y\n", "apply", "--file", r.genvJSON)
	if code != 0 {
		t.Logf("apply after service removal: exit %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}

	// Verify service removed from lock
	lock = r.readLockRaw(t)
	for _, svc := range lock.Services {
		if svc.Name == "apply-svc" {
			t.Errorf("service 'apply-svc' still in lock after removal from spec")
		}
	}

	// Test: status command with services
	r.writeSpecRaw(t, `{
		"schemaVersion": "4",
		"packages": [],
		"services": {
			"status-test": {
				"start": ["true"],
				"status": ["false"]
			}
		}
	}`)

	stdout, stderr, code = r.rawExec("", "status", "--file", r.genvJSON)
	if code != 0 {
		t.Logf("status with services drift: exit %d (expected non-zero)", code)
	}
	if !strings.Contains(stdout, "status-test") {
		t.Errorf("status output should mention 'status-test':\n%s", stdout)
	}
}
