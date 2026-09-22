package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ks1686/genv/internal/adapter"
	"github.com/ks1686/genv/internal/genvfile"
	"github.com/ks1686/genv/internal/verify"
)

type verifyQueryAdapter struct {
	name      string
	installed map[string]bool
	err       error
	calls     []string
}

func (a *verifyQueryAdapter) Name() string    { return a.name }
func (a *verifyQueryAdapter) Available() bool { return true }
func (a *verifyQueryAdapter) NormalizeID(id string, managers map[string]string) (string, bool) {
	if n, ok := managers[a.name]; ok {
		return n, true
	}
	return id, false
}
func (a *verifyQueryAdapter) PlanInstall(pkgName string) []string   { return []string{"true"} }
func (a *verifyQueryAdapter) PlanUninstall(pkgName string) []string { return []string{"true"} }
func (a *verifyQueryAdapter) PlanUpgrade(pkgName string) []string   { return []string{"true"} }
func (a *verifyQueryAdapter) PlanClean() [][]string                 { return nil }
func (a *verifyQueryAdapter) Query(pkgName string) (bool, error) {
	a.calls = append(a.calls, pkgName)
	if a.err != nil {
		return false, a.err
	}
	return a.installed[pkgName], nil
}
func (a *verifyQueryAdapter) ListInstalled() ([]string, error) { return nil, nil }
func (a *verifyQueryAdapter) QueryVersion(pkgName string) (string, error) {
	if a.installed[pkgName] {
		return "1.2.3", nil
	}
	return "", nil
}

func installVerifyAdapter(t *testing.T, a adapter.Adapter) {
	t.Helper()
	orig := adapter.All
	adapter.All = []adapter.Adapter{a}
	t.Cleanup(func() { adapter.All = orig })
}

func TestStatusCmd_VerifyProvesInstalled(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	path := filepath.Join(dir, "genv.json")
	lockPath := genvfile.LockPathFrom(path)
	writeTestFile(t, path, `{"schemaVersion":"1","packages":[{"id":"git","prefer":"pacman"}]}`)
	writeLock(t, lockPath, []genvfile.LockedPackage{
		{ID: "git", Manager: "pacman", PkgName: "git", InstalledVersion: "2.43.0"},
	})
	stub := &verifyQueryAdapter{name: "pacman", installed: map[string]bool{"git": true}}
	installVerifyAdapter(t, stub)

	var code int
	out := captureStdout(t, func() {
		code = run([]string{"status", "--file", path, "--verify", "--json"})
	})
	if code != exitOK {
		t.Fatalf("status --verify = %d, want %d\n%s", code, exitOK, out)
	}
	if len(stub.calls) == 0 {
		t.Fatal("status --verify must Query the manager")
	}
	var env map[string]any
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("json: %v\n%s", err, out)
	}
	if env["ok"] != true {
		t.Fatalf("ok = %v, want true\n%s", env["ok"], out)
	}
}

func TestStatusCmd_VerifyLockedMissingIsDrift(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	path := filepath.Join(dir, "genv.json")
	lockPath := genvfile.LockPathFrom(path)
	writeTestFile(t, path, `{"schemaVersion":"1","packages":[{"id":"git","prefer":"pacman"}]}`)
	writeLock(t, lockPath, []genvfile.LockedPackage{
		{ID: "git", Manager: "pacman", PkgName: "git", InstalledVersion: "2.43.0"},
	})
	installVerifyAdapter(t, &verifyQueryAdapter{name: "pacman", installed: map[string]bool{}})

	var code int
	out := captureStdout(t, func() {
		code = run([]string{"status", "--file", path, "--verify", "--json"})
	})
	if code != exitLogic {
		t.Fatalf("status --verify drift = %d, want %d\n%s", code, exitLogic, out)
	}
	if !strings.Contains(out, `"kind":"drift"`) {
		t.Fatalf("want drift kind, got %s", out)
	}

	out = captureStdout(t, func() {
		code = run([]string{"status", "--file", path, "--verify"})
	})
	if code != exitLogic {
		t.Fatalf("status --verify human drift = %d, want %d\n%s", code, exitLogic, out)
	}
	if !strings.Contains(out, "not installed via pacman") {
		t.Fatalf("human output missing absent-install note:\n%s", out)
	}
}

func TestStatusCmd_VerifyOfflineConflict(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "genv.json")
	writeTestFile(t, path, `{"schemaVersion":"1","packages":[]}`)
	var code int
	errOut := captureStderr(t, func() {
		code = run([]string{"status", "--file", path, "--verify", "--offline"})
	})
	if code != exitUsage {
		t.Fatalf("status --verify --offline = %d, want %d", code, exitUsage)
	}
	if !strings.Contains(errOut, "--verify") || !strings.Contains(errOut, "--offline") {
		t.Fatalf("stderr = %q, want conflict between --verify and --offline", errOut)
	}
}

func TestStatusCmd_VerifyFilesConflict(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "genv.json")
	writeTestFile(t, path, `{"schemaVersion":"1","packages":[]}`)
	var code int
	errOut := captureStderr(t, func() {
		code = run([]string{"status", "--file", path, "--verify", "--files"})
	})
	if code != exitUsage {
		t.Fatalf("status --verify --files = %d, want %d", code, exitUsage)
	}
	if !strings.Contains(errOut, "--verify") || !strings.Contains(errOut, "--files") {
		t.Fatalf("stderr = %q, want conflict between --verify and --files", errOut)
	}
}

func TestStatusCmd_VerifyHelp(t *testing.T) {
	var code int
	errOut := captureStderr(t, func() { code = run([]string{"status", "--help"}) })
	if code != exitOK {
		t.Fatalf("status --help = %d, want %d\n%s", code, exitOK, errOut)
	}
	if !strings.Contains(errOut, "--verify") {
		t.Fatalf("status --help missing --verify:\n%s", errOut)
	}
}

func TestExportCmd_VerifyProvesInstalled(t *testing.T) {
	dir := t.TempDir()
	specPath := filepath.Join(dir, "genv.json")
	outDir := filepath.Join(dir, "out")
	writeTestFile(t, specPath, `{
	  "schemaVersion":"8",
	  "targets":{"arch":{"packages":[{"id":"git","prefer":"pacman"}]}}
	}`)
	stub := &verifyQueryAdapter{name: "pacman", installed: map[string]bool{"git": true}}
	installVerifyAdapter(t, stub)

	var code int
	errOut := captureStderr(t, func() {
		code = run([]string{"export", "--file", specPath, "--target", "arch", "--out", outDir, "--verify", "--strict"})
	})
	if code != exitOK {
		t.Fatalf("export --verify --strict = %d, want %d; stderr=%s", code, exitOK, errOut)
	}
	if len(stub.calls) == 0 {
		t.Fatal("export --verify must Query the manager")
	}
	if _, err := genvfile.Read(filepath.Join(outDir, "genv.json")); err != nil {
		t.Fatalf("exported snapshot: %v", err)
	}
}

func TestExportCmd_VerifyMissingFailsStrict(t *testing.T) {
	dir := t.TempDir()
	specPath := filepath.Join(dir, "genv.json")
	outDir := filepath.Join(dir, "out")
	writeTestFile(t, specPath, `{
	  "schemaVersion":"8",
	  "targets":{"arch":{"packages":[{"id":"ripgrep","prefer":"pacman"}]}}
	}`)
	installVerifyAdapter(t, &verifyQueryAdapter{name: "pacman", installed: map[string]bool{}})

	code := run([]string{"export", "--file", specPath, "--target", "arch", "--out", outDir, "--verify", "--strict"})
	if code != exitLogic {
		t.Fatalf("export --verify --strict missing = %d, want %d", code, exitLogic)
	}
	data, err := os.ReadFile(filepath.Join(outDir, "report.json"))
	if err != nil {
		t.Fatalf("report.json: %v", err)
	}
	if !strings.Contains(string(data), verify.CodeNotInstalled) {
		t.Fatalf("report.json missing %s:\n%s", verify.CodeNotInstalled, data)
	}
}

func TestExportCmd_VerifyHelp(t *testing.T) {
	var code int
	errOut := captureStderr(t, func() { code = run([]string{"export", "--help"}) })
	if code != exitOK {
		t.Fatalf("export --help = %d, want %d\n%s", code, exitOK, errOut)
	}
	if !strings.Contains(errOut, "--verify") {
		t.Fatalf("export --help missing --verify:\n%s", errOut)
	}
}
