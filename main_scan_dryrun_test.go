package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ks1686/genv/internal/adapter"
	"github.com/ks1686/genv/internal/genvfile"
)

func TestScanCmd_DryRunDoesNotWriteSpec(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	path := filepath.Join(dir, "genv.json")
	lockPath := filepath.Join(dir, "genv.lock.json")

	snap := &scanManagerNameAdapter{name: "snap", installed: []string{"jq", "ripgrep"}}
	originalAll := adapter.All
	adapter.All = []adapter.Adapter{snap}
	t.Cleanup(func() { adapter.All = originalAll })

	var code int
	out := captureStdout(t, func() {
		code = run([]string{"scan", "--file", path, "--lock-file", lockPath, "--dry-run"})
	})
	if code != exitOK {
		t.Fatalf("scan --dry-run: expected exitOK (%d), got %d\n%s", exitOK, code, out)
	}
	if !strings.Contains(out, "jq") || !strings.Contains(out, "ripgrep") {
		t.Fatalf("dry-run should list would-adopt packages; got %q", out)
	}
	if !strings.Contains(out, "dry-run") && !strings.Contains(out, "would adopt") {
		t.Fatalf("dry-run output should say it is a preview; got %q", out)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("dry-run must not create genv.json; err=%v", err)
	}
	if _, err := os.Stat(lockPath); !os.IsNotExist(err) {
		t.Fatalf("dry-run must not create lock; err=%v", err)
	}
}

func TestScanCmd_DryRunJSON(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	path := filepath.Join(dir, "genv.json")
	lockPath := filepath.Join(dir, "genv.lock.json")

	snap := &scanManagerNameAdapter{name: "snap", installed: []string{"jq"}}
	originalAll := adapter.All
	adapter.All = []adapter.Adapter{snap}
	t.Cleanup(func() { adapter.All = originalAll })

	var code int
	out := captureStdout(t, func() {
		code = run([]string{"scan", "--file", path, "--lock-file", lockPath, "--dry-run", "--json"})
	})
	if code != exitOK {
		t.Fatalf("scan --dry-run --json: expected exitOK, got %d\n%s", code, out)
	}
	var raw map[string]any
	if err := json.Unmarshal([]byte(out), &raw); err != nil {
		t.Fatalf("json: %v\n%s", err, out)
	}
	if raw["ok"] != true || raw["command"] != "scan" {
		t.Fatalf("envelope = %+v", raw)
	}
	data, _ := raw["data"].(map[string]any)
	if data["added"] != float64(1) {
		t.Fatalf("data.added = %v, want 1", data["added"])
	}
	if data["dryRun"] != true {
		t.Fatalf("data.dryRun = %v, want true", data["dryRun"])
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("dry-run --json must not write spec")
	}
}

func TestScanCmd_RequiresConfirmationWithoutYes(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	path := filepath.Join(dir, "genv.json")
	lockPath := filepath.Join(dir, "genv.lock.json")

	snap := &scanManagerNameAdapter{name: "snap", installed: []string{"jq"}}
	originalAll := adapter.All
	adapter.All = []adapter.Adapter{snap}
	t.Cleanup(func() { adapter.All = originalAll })

	var code int
	out := captureStdout(t, func() {
		code = run([]string{"scan", "--file", path, "--lock-file", lockPath})
	})
	if code != exitOK {
		t.Fatalf("scan without --yes: expected exitOK (abort), got %d\n%s", code, out)
	}
	if !strings.Contains(out, "Aborted") && !strings.Contains(out, "Continue") {
		t.Fatalf("expected confirmation abort; got %q", out)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("declined scan must not write genv.json")
	}
}

func TestScanCmd_YesWritesSpec(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	path := filepath.Join(dir, "genv.json")
	lockPath := filepath.Join(dir, "genv.lock.json")

	snap := &scanManagerNameAdapter{name: "snap", installed: []string{"jq"}}
	originalAll := adapter.All
	adapter.All = []adapter.Adapter{snap}
	t.Cleanup(func() { adapter.All = originalAll })

	code := run([]string{"scan", "--file", path, "--lock-file", lockPath, "--yes"})
	if code != exitOK {
		t.Fatalf("scan --yes: expected exitOK, got %d", code)
	}
	f, err := genvfile.Read(path)
	if err != nil {
		t.Fatalf("read spec: %v", err)
	}
	n := 0
	id := ""
	for _, b := range f.Targets {
		if b == nil {
			continue
		}
		n += len(b.Packages)
		if len(b.Packages) > 0 {
			id = b.Packages[0].ID
		}
	}
	if f.SchemaVersion != "8" || n != 1 || id != "jq" {
		t.Fatalf("scan wrote schema %q packages=%d id=%q targets=%+v", f.SchemaVersion, n, id, f.Targets)
	}
}
