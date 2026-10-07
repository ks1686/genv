//go:build integration

package e2e_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestE2ECompose covers schema v10 module composition through the real binary.
//
// Composition has no package-manager dependency, so unlike the other e2e tests
// this one needs no adapter and runs anywhere. It checks the property that
// matters end to end: a spec assembled from several documents behaves exactly
// like a single one, and a module-owned resource cannot be edited from the CLI.
func TestE2ECompose(t *testing.T) {
	dir := t.TempDir()
	modulesDir := filepath.Join(dir, "modules")
	if err := os.MkdirAll(modulesDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	write := func(rel, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, rel), []byte(body), 0o644); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}

	// base declares jq and EDITOR; dev requires base and declares ripgrep plus a
	// second, identical EDITOR so the two coalesce instead of conflicting.
	write("modules/base.json", `{
  "schemaVersion": "10",
  "defaults": {
    "packages": [{ "id": "jq" }],
    "env": { "EDITOR": { "value": "nvim" } }
  }
}`)
	write("modules/dev.json", `{
  "schemaVersion": "10",
  "requiresModules": ["base"],
  "defaults": {
    "packages": [{ "id": "ripgrep" }],
    "env": { "EDITOR": { "value": "nvim" } }
  }
}`)
	// Registered but never selected: validate must still load it.
	write("modules/idle.json", `{
  "schemaVersion": "10",
  "defaults": { "packages": [{ "id": "fd" }] }
}`)
	write("genv.json", `{
  "schemaVersion": "10",
  "modules": {
    "base": "modules/base.json",
    "dev": "modules/dev.json",
    "idle": "modules/idle.json"
  },
  "targets": {
    "macos": {
      "useModules": ["dev"],
      "packages": [{ "id": "git" }]
    }
  }
}`)

	spec := filepath.Join(dir, "genv.json")
	lock := filepath.Join(dir, "genv.lock.json")

	// run invokes the real binary and returns stdout+stderr together: most of
	// these assertions care that a message appeared somewhere.
	r := &runner{bin: genvBin, genvJSON: spec, lockJSON: lock}
	run := func(args ...string) (string, int) {
		t.Helper()
		stdout, stderr, code := r.rawExec("", args...)
		return stdout + stderr, code
	}

	t.Run("validate loads every registered module", func(t *testing.T) {
		out, code := run("validate", "--file", spec)
		if code != 0 {
			t.Fatalf("validate exit %d: %s", code, out)
		}
		if !strings.Contains(out, "is valid") {
			t.Errorf("validate output = %q", out)
		}
	})

	t.Run("config composes the selection", func(t *testing.T) {
		out, code := run("config", "--file", spec, "--target", "macos", "--json")
		if code != 0 {
			t.Fatalf("config exit %d: %s", code, out)
		}
		var got struct {
			Modules []string `json:"modules"`
			Counts  struct {
				Packages int `json:"packages"`
			} `json:"counts"`
			Fingerprint string `json:"fingerprint"`
		}
		if err := json.Unmarshal([]byte(out), &got); err != nil {
			t.Fatalf("parse config json: %v\n%s", err, out)
		}
		// dev pulls base in; idle is registered but not selected.
		if strings.Join(got.Modules, ",") != "base,dev" {
			t.Errorf("modules = %v, want [base dev]", got.Modules)
		}
		if got.Counts.Packages != 3 {
			t.Errorf("packages = %d, want 3 (git, jq, ripgrep)", got.Counts.Packages)
		}
		if got.Fingerprint == "" {
			t.Error("fingerprint should be set")
		}
	})

	t.Run("config registry lists unselected modules", func(t *testing.T) {
		out, code := run("config", "--file", spec, "--registry")
		if code != 0 {
			t.Fatalf("config --registry exit %d: %s", code, out)
		}
		if !strings.Contains(out, "not selected by any target") {
			t.Errorf("expected idle to be marked unselected:\n%s", out)
		}
	})

	t.Run("explain attributes to the declaring module", func(t *testing.T) {
		out, code := run("explain", "package", "ripgrep", "--file", spec, "--target", "macos")
		if code != 0 {
			t.Fatalf("explain exit %d: %s", code, out)
		}
		if !strings.Contains(out, `module "dev"`) {
			t.Errorf("explain should name module dev:\n%s", out)
		}
		if !strings.Contains(out, "owned by a module") {
			t.Errorf("explain should mark it read-only:\n%s", out)
		}
	})

	t.Run("apply dry-run plans module packages", func(t *testing.T) {
		out, code := run("apply", "--file", spec, "--lock-file", lock,
			"--target", "macos", "--dry-run", "--json")
		// Package resolution depends on the host's managers, so only the shape
		// of the response is asserted here; composition itself is covered above.
		if code != 0 {
			t.Skipf("apply --dry-run exited %d on this host: %s", code, out)
		}
		// A composed package is planned like any other, but which list it lands
		// in depends on the host: toInstall when absent, adopted/unchanged when
		// the host already has it. All three prove it reached reconciliation.
		var env struct {
			Data struct {
				ToInstall []struct {
					ID string `json:"id"`
				} `json:"toInstall"`
				Adopted []struct {
					ID string `json:"id"`
				} `json:"adopted"`
				Unchanged []struct {
					ID string `json:"id"`
				} `json:"unchanged"`
			} `json:"data"`
		}
		if err := json.Unmarshal([]byte(out), &env); err != nil {
			t.Fatalf("parse apply json: %v\n%s", err, out)
		}
		ids := map[string]bool{}
		for _, group := range [][]struct {
			ID string `json:"id"`
		}{env.Data.ToInstall, env.Data.Adopted, env.Data.Unchanged} {
			for _, p := range group {
				ids[p.ID] = true
			}
		}
		for _, want := range []string{"jq", "ripgrep", "git"} {
			if !ids[want] {
				t.Errorf("apply plan missing %q (got %v)", want, ids)
			}
		}
	})

	t.Run("module-owned package cannot be added", func(t *testing.T) {
		out, code := run("add", "ripgrep", "--file", spec, "--target", "macos",
			"--no-search", "--no-hooks")
		if code == 0 {
			t.Fatalf("adding a module-owned package should fail; output: %s", out)
		}
		if !strings.Contains(out, "dev") {
			t.Errorf("refusal should name the owning module:\n%s", out)
		}
		data, err := os.ReadFile(spec)
		if err != nil {
			t.Fatalf("read spec: %v", err)
		}
		if strings.Count(string(data), "ripgrep") != 0 {
			t.Errorf("refused add must not modify the root spec:\n%s", data)
		}
	})

	t.Run("conflicting modules fail composition", func(t *testing.T) {
		badDir := t.TempDir()
		if err := os.MkdirAll(filepath.Join(badDir, "modules"), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(filepath.Join(badDir, "modules", "a.json"),
			[]byte(`{"schemaVersion":"10","defaults":{"env":{"EDITOR":{"value":"nvim"}}}}`), 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}
		if err := os.WriteFile(filepath.Join(badDir, "modules", "b.json"),
			[]byte(`{"schemaVersion":"10","defaults":{"env":{"EDITOR":{"value":"emacs"}}}}`), 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}
		badSpec := filepath.Join(badDir, "genv.json")
		if err := os.WriteFile(badSpec, []byte(`{
  "schemaVersion": "10",
  "modules": { "a": "modules/a.json", "b": "modules/b.json" },
  "targets": { "macos": { "useModules": ["a", "b"] } }
}`), 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}
		badLock := filepath.Join(badDir, "genv.lock.json")
		out, code := run("apply", "--file", badSpec, "--lock-file", badLock, "--target", "macos", "--dry-run")
		if code == 0 {
			t.Fatalf("conflicting modules should fail; output: %s", out)
		}
		if _, err := os.Stat(badLock); err == nil {
			t.Error("a failed composition must not write a lock")
		}
	})

	t.Run("module path escaping the spec root is refused", func(t *testing.T) {
		outside := filepath.Join(t.TempDir(), "evil.json")
		if err := os.WriteFile(outside, []byte(`{"schemaVersion":"10","defaults":{}}`), 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}
		escapeSpec := filepath.Join(dir, "escape.json")
		if err := os.WriteFile(escapeSpec, []byte(`{
  "schemaVersion": "10",
  "modules": { "evil": "../evil.json" },
  "targets": { "macos": { "useModules": ["evil"] } }
}`), 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}
		if _, code := run("config", "--file", escapeSpec, "--target", "macos"); code == 0 {
			t.Errorf("a module outside the spec root must be refused")
		}
	})

	t.Run("migrate refuses v10", func(t *testing.T) {
		out, code := run("migrate", "--file", spec)
		if code == 0 {
			t.Fatalf("migrate should refuse a v10 spec; output: %s", out)
		}
	})
}
