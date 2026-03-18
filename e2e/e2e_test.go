//go:build integration

// Package e2e_test contains end-to-end tests for the gpm binary.
//
// Unlike the adapter-layer integration tests (internal/adapter/integration_test.go),
// these tests build the real gpm binary and exercise every M1/M2 command
// against a live package manager on the host:
//
//   - gpm add / remove / list (ls) / apply / apply --dry-run / clean / clean --dry-run
//   - gpm adopt: verify already-installed package is tracked without reinstalling
//   - gpm disown: verify package is untracked without being uninstalled
//   - lock-file integrity after every mutation
//   - gpm apply reconcile: install newly-added packages, remove deleted ones
//   - gpm apply --strict with a fully-resolved plan
//   - error paths: duplicate add, remove/adopt/disown of untracked package, apply with no gpm.json
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
)

// gpmBin is the path to the compiled gpm binary, populated by TestMain.
var gpmBin string

func TestMain(m *testing.M) {
	tmp, err := os.MkdirTemp("", "gpm-e2e-bin-*")
	if err != nil {
		panic("mkdirtemp: " + err.Error())
	}

	bin := filepath.Join(tmp, "gpm")
	out, err := exec.Command("go", "build", "-buildvcs=false", "-o", bin, "github.com/ks1686/gpm").CombinedOutput()
	if err != nil {
		os.RemoveAll(tmp)
		panic("go build failed:\n" + string(out))
	}
	gpmBin = bin

	code := m.Run()
	os.RemoveAll(tmp)
	os.Exit(code)
}

// ── runner ────────────────────────────────────────────────────────────────────

// runner bundles the binary path, per-test file paths, and optional prefer
// flag so every helper can inject them consistently.
type runner struct {
	bin      string
	gpmJSON  string
	lockJSON string
	prefer   string // if non-empty, passed as --prefer on gpm add and written into specs
}

func newRunner(t *testing.T, prefer string) *runner {
	t.Helper()
	dir := t.TempDir()
	g := filepath.Join(dir, "gpm.json")
	return &runner{
		bin:      gpmBin,
		gpmJSON:  g,
		lockJSON: strings.TrimSuffix(g, ".json") + ".lock.json",
		prefer:   prefer,
	}
}

// rawExec runs the gpm binary with the given args and optional stdin.
// Returns stdout, stderr, and exit code (0 on success).
func (r *runner) rawExec(stdinData string, args ...string) (stdout, stderr string, code int) {
	cmd := exec.Command(r.bin, args...)
	if stdinData != "" {
		cmd.Stdin = strings.NewReader(stdinData)
	}
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

// gpm runs a gpm subcommand with --file injected as the second argument.
// Use rawExec for commands that do not accept --file (help, clean).
func (r *runner) gpm(stdinData, subcmd string, extra ...string) (stdout, stderr string, code int) {
	args := make([]string, 0, 3+len(extra))
	args = append(args, subcmd, "--file", r.gpmJSON)
	args = append(args, extra...)
	return r.rawExec(stdinData, args...)
}

// ── spec / lock helpers ───────────────────────────────────────────────────────

// specIDs returns the package IDs currently in gpm.json.
func (r *runner) specIDs(t *testing.T) []string {
	t.Helper()
	data, err := os.ReadFile(r.gpmJSON)
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

// lockPackages returns the packages slice from gpm.lock.json.
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

// writeSpec writes a minimal gpm.json containing exactly the given package IDs.
// Each entry gets the runner's prefer field (if set) so that apply tests resolve
// to the same adapter as gpm add.
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
	data, _ := json.MarshalIndent(spec{SchemaVersion: "1", Packages: pkgs}, "", "  ")
	if err := os.WriteFile(r.gpmJSON, append(data, '\n'), 0o644); err != nil {
		t.Fatalf("write spec: %v", err)
	}
}

// clearLock deletes gpm.lock.json, simulating a first-run state.
func (r *runner) clearLock(t *testing.T) {
	t.Helper()
	if err := os.Remove(r.lockJSON); err != nil && !os.IsNotExist(err) {
		t.Fatalf("clear lock: %v", err)
	}
}

func (r *runner) assertInSpec(t *testing.T, id string) {
	t.Helper()
	for _, s := range r.specIDs(t) {
		if s == id {
			return
		}
	}
	t.Errorf("expected %q in gpm.json; current ids: %v", id, r.specIDs(t))
}

func (r *runner) assertNotInSpec(t *testing.T, id string) {
	t.Helper()
	for _, s := range r.specIDs(t) {
		if s == id {
			t.Errorf("expected %q absent from gpm.json, but it is present", id)
			return
		}
	}
}

func (r *runner) assertInLock(t *testing.T, id, manager string) {
	t.Helper()
	for _, p := range r.lockPackages(t) {
		if p["id"] == id {
			if p["manager"] != manager {
				t.Errorf("lock: %q manager=%q, want %q", id, p["manager"], manager)
			}
			return
		}
	}
	t.Errorf("expected %q in gpm.lock.json with manager=%q; current entries: %v", id, manager, r.lockPackages(t))
}

func (r *runner) assertNotInLock(t *testing.T, id string) {
	t.Helper()
	for _, p := range r.lockPackages(t) {
		if p["id"] == id {
			t.Errorf("expected %q absent from gpm.lock.json, but it is present", id)
			return
		}
	}
}

// ── suite ─────────────────────────────────────────────────────────────────────

// suiteConfig describes per-adapter parameters for the E2E suite.
type suiteConfig struct {
	adapterName string // e.g. "apt" — must match the adapter's Name()
	checkBin    string // binary that must exist in PATH for this adapter
	testPkg     string // small package to install and remove during tests
	preferFlag  string // if non-empty, passed as --prefer on gpm add (and written into specs)
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
			t.Errorf("gpm help: exit %d, want 0", code)
		}
	})

	t.Run("help_flag", func(t *testing.T) {
		_, _, code := r.rawExec("", "--help")
		if code != 0 {
			t.Errorf("gpm --help: exit %d, want 0", code)
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
		stdout, _, code := r.gpm("", "list")
		if code != 0 {
			t.Fatalf("gpm list: exit %d, want 0", code)
		}
		if !strings.Contains(stdout, "no packages installed") {
			t.Errorf("expected empty-list message, got: %q", stdout)
		}
	})

	t.Run("ls_alias_empty", func(t *testing.T) {
		stdout, _, code := r.gpm("", "ls")
		if code != 0 {
			t.Fatalf("gpm ls: exit %d, want 0", code)
		}
		if !strings.Contains(stdout, "no packages installed") {
			t.Errorf("ls alias: expected empty-list message, got: %q", stdout)
		}
	})

	// ── apply with missing gpm.json ───────────────────────────────────────────

	t.Run("apply_missing_spec_fails", func(t *testing.T) {
		r2 := newRunner(t, cfg.preferFlag)
		_, _, code := r2.gpm("", "apply", "--dry-run")
		if code == 0 {
			t.Error("apply --dry-run with no gpm.json: expected non-zero exit, got 0")
		}
	})

	// ── gpm add ───────────────────────────────────────────────────────────────

	// build the add-command args (--prefer injected when configured)
	addArgs := []string{cfg.testPkg}
	if cfg.preferFlag != "" {
		addArgs = append(addArgs, "--prefer", cfg.preferFlag)
	}

	t.Run("add_creates_spec_entry", func(t *testing.T) {
		stdout, _, code := r.gpm("", "add", addArgs...)
		if code != 0 {
			t.Fatalf("gpm add %s: exit %d\nstdout: %s", cfg.testPkg, code, stdout)
		}
		r.assertInSpec(t, cfg.testPkg)
	})

	if cfg.canInstall {
		t.Run("add_creates_lock_entry", func(t *testing.T) {
			r.assertInLock(t, cfg.testPkg, cfg.adapterName)
		})

		// ── list / ls after install ────────────────────────────────────────────

		t.Run("list_shows_installed_package", func(t *testing.T) {
			stdout, _, code := r.gpm("", "list")
			if code != 0 {
				t.Fatalf("gpm list: exit %d, want 0", code)
			}
			if !strings.Contains(stdout, cfg.testPkg) {
				t.Errorf("expected %q in list output, got: %q", cfg.testPkg, stdout)
			}
			if !strings.Contains(stdout, cfg.adapterName) {
				t.Errorf("expected manager %q in list output, got: %q", cfg.adapterName, stdout)
			}
		})

		t.Run("ls_alias_shows_installed_package", func(t *testing.T) {
			stdout, _, code := r.gpm("", "ls")
			if code != 0 {
				t.Fatalf("gpm ls: exit %d, want 0", code)
			}
			if !strings.Contains(stdout, cfg.testPkg) {
				t.Errorf("ls alias: expected %q in output, got: %q", cfg.testPkg, stdout)
			}
		})

		// ── duplicate add rejected ─────────────────────────────────────────────

		t.Run("add_duplicate_rejected", func(t *testing.T) {
			_, stderr, code := r.gpm("", "add", addArgs...)
			if code == 0 {
				t.Error("duplicate add: expected non-zero exit, got 0")
			}
			if !strings.Contains(stderr, "already tracked") {
				t.Errorf("duplicate add: expected 'already tracked' in stderr, got: %q", stderr)
			}
		})

		// ── apply --dry-run: up to date ────────────────────────────────────────

		t.Run("apply_dry_run_up_to_date", func(t *testing.T) {
			stdout, _, code := r.gpm("", "apply", "--dry-run")
			if code != 0 {
				t.Fatalf("apply --dry-run (up-to-date): exit %d, want 0", code)
			}
			if !strings.Contains(stdout, "up to date") {
				t.Errorf("expected 'up to date' in output, got: %q", stdout)
			}
		})

		// ── gpm remove ────────────────────────────────────────────────────────

		t.Run("remove_updates_spec_and_lock", func(t *testing.T) {
			stdout, _, code := r.gpm("", "remove", cfg.testPkg)
			if code != 0 {
				t.Fatalf("gpm remove %s: exit %d\nstdout: %s", cfg.testPkg, code, stdout)
			}
			r.assertNotInSpec(t, cfg.testPkg)
			r.assertNotInLock(t, cfg.testPkg)
		})

		t.Run("list_empty_after_remove", func(t *testing.T) {
			stdout, _, code := r.gpm("", "list")
			if code != 0 {
				t.Fatalf("gpm list: exit %d, want 0", code)
			}
			if !strings.Contains(stdout, "no packages installed") {
				t.Errorf("expected empty-list message after remove, got: %q", stdout)
			}
		})

		// ── rm alias ──────────────────────────────────────────────────────────

		t.Run("rm_alias", func(t *testing.T) {
			if _, _, code := r.gpm("", "add", addArgs...); code != 0 {
				t.Fatalf("setup: gpm add: exit %d", code)
			}
			_, _, code := r.gpm("", "rm", cfg.testPkg)
			if code != 0 {
				t.Errorf("gpm rm: exit %d, want 0", code)
			}
			r.assertNotInSpec(t, cfg.testPkg)
			r.assertNotInLock(t, cfg.testPkg)
		})

		// ── apply: install path ───────────────────────────────────────────────
		// Write spec directly (bypasses gpm add) and run apply from a clean lock.

		t.Run("apply_installs_from_spec", func(t *testing.T) {
			r.writeSpec(t, cfg.testPkg)
			r.clearLock(t)
			stdout, _, code := r.gpm("y\n", "apply")
			if code != 0 {
				t.Fatalf("gpm apply (install): exit %d\nstdout: %s", code, stdout)
			}
			r.assertInLock(t, cfg.testPkg, cfg.adapterName)
		})

		// ── apply --dry-run: shows manager and concrete package name ──────────

		t.Run("apply_dry_run_shows_plan", func(t *testing.T) {
			// spec has testPkg; clear the lock so apply would need to install
			r.clearLock(t)
			stdout, _, code := r.gpm("", "apply", "--dry-run")
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
			if _, _, code := r.gpm("y\n", "apply"); code != 0 {
				t.Fatalf("setup apply: exit %d", code)
			}
			r.assertInLock(t, cfg.testPkg, cfg.adapterName)

			r.writeSpec(t) // empty spec — package "deleted" by user
			stdout, _, code := r.gpm("y\n", "apply")
			if code != 0 {
				t.Fatalf("apply (remove path): exit %d\nstdout: %s", code, stdout)
			}
			r.assertNotInLock(t, cfg.testPkg)
		})

		// ── apply --strict with a fully-resolved plan ─────────────────────────

		t.Run("apply_strict_all_resolved", func(t *testing.T) {
			r.writeSpec(t, cfg.testPkg)
			r.clearLock(t)
			stdout, _, code := r.gpm("y\n", "apply", "--strict")
			if code != 0 {
				t.Fatalf("apply --strict (all resolved): exit %d\nstdout: %s", code, stdout)
			}
			r.assertInLock(t, cfg.testPkg, cfg.adapterName)
			// clean up so later tests start from an empty state
			r.gpm("", "remove", cfg.testPkg)
		})

		// ── gpm disown ────────────────────────────────────────────────────────
		// Install and track testPkg, then disown it. The package must remain
		// installed on the system but disappear from spec and lock.

		t.Run("disown_removes_tracking_keeps_package_installed", func(t *testing.T) {
			if _, _, code := r.gpm("", "add", addArgs...); code != 0 {
				t.Fatalf("setup disown: gpm add exit %d", code)
			}
			r.assertInSpec(t, cfg.testPkg)
			r.assertInLock(t, cfg.testPkg, cfg.adapterName)

			stdout, _, code := r.gpm("", "disown", cfg.testPkg)
			if code != 0 {
				t.Fatalf("gpm disown %s: exit %d\nstdout: %s", cfg.testPkg, code, stdout)
			}
			r.assertNotInSpec(t, cfg.testPkg)
			r.assertNotInLock(t, cfg.testPkg)
			if !strings.Contains(stdout, "remains installed") {
				t.Errorf("expected 'remains installed' in stdout, got: %q", stdout)
			}
		})

		t.Run("disown_not_tracked_fails", func(t *testing.T) {
			_, _, code := r.gpm("", "disown", "gpm-e2e-never-tracked-xyzzy")
			if code == 0 {
				t.Error("disown untracked: expected non-zero exit, got 0")
			}
		})

		// ── gpm adopt ────────────────────────────────────────────────────────
		// testPkg is installed on the system but not tracked by gpm
		// (state left by disown_removes_tracking_keeps_package_installed above).

		adoptArgs := []string{cfg.testPkg}
		if cfg.preferFlag != "" {
			adoptArgs = append(adoptArgs, "--prefer", cfg.preferFlag)
		}

		t.Run("adopt_tracks_already_installed_package", func(t *testing.T) {
			stdout, _, code := r.gpm("", "adopt", adoptArgs...)
			if code != 0 {
				t.Fatalf("gpm adopt %s: exit %d\nstdout: %s", cfg.testPkg, code, stdout)
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
			_, stderr, code := r.gpm("", "adopt", adoptArgs...)
			if code == 0 {
				t.Error("adopt already-tracked: expected non-zero exit, got 0")
			}
			if !strings.Contains(stderr, "already tracked") {
				t.Errorf("expected 'already tracked' in stderr, got: %q", stderr)
			}
			// clean up so the package is gone for the not-installed test
			r.gpm("", "remove", cfg.testPkg)
		})

		t.Run("adopt_not_installed_fails", func(t *testing.T) {
			notInstalledArgs := []string{"gpm-e2e-definitely-not-installed-xyzzy"}
			if cfg.preferFlag != "" {
				notInstalledArgs = append(notInstalledArgs, "--prefer", cfg.preferFlag)
			}
			_, stderr, code := r.gpm("", "adopt", notInstalledArgs...)
			if code == 0 {
				t.Error("adopt not-installed: expected non-zero exit, got 0")
			}
			if !strings.Contains(stderr, "not installed") {
				t.Errorf("expected 'not installed' in stderr, got: %q", stderr)
			}
		})
	}

	// ── remove of an untracked package fails ──────────────────────────────────

	t.Run("remove_not_tracked_fails", func(t *testing.T) {
		_, _, code := r.gpm("", "remove", "gpm-e2e-never-tracked-xyzzy")
		if code == 0 {
			t.Error("remove untracked: expected non-zero exit, got 0")
		}
	})

	// ── gpm clean ─────────────────────────────────────────────────────────────

	t.Run("clean_dry_run", func(t *testing.T) {
		_, _, code := r.rawExec("", "clean", "--dry-run")
		if code != 0 {
			t.Errorf("gpm clean --dry-run: exit %d, want 0", code)
		}
	})

	t.Run("clean", func(t *testing.T) {
		_, _, code := r.rawExec("", "clean")
		if code != 0 {
			t.Errorf("gpm clean: exit %d, want 0", code)
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
// gpm add still exits 0 even when the install fails (non-fatal by design), so
// spec/lock mutations and all read-only commands are still fully exercised.
func TestE2EFlatpak(t *testing.T) {
	runE2ESuite(t, suiteConfig{
		adapterName: "flatpak",
		checkBin:    "flatpak",
		testPkg:     "com.example.TestApp",
		canInstall:  false,
	})
}
