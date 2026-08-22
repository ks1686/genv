package main

import (
	"encoding/json"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/ks1686/genv/internal/adapter"
	"github.com/ks1686/genv/internal/genvfile"
	"github.com/ks1686/genv/internal/resolver"
)

// TestScanCmd_HangingManagerDoesNotWedgeScan is the end-to-end regression test
// for the Windows CI hang (TestScanCmd_JsonOutput timed out after a winget
// first-run stall): a manager whose ListInstalled blocks forever must be
// skipped after the per-manager deadline, and scan must still exit OK with a
// valid JSON envelope.
func TestScanCmd_HangingManagerDoesNotWedgeScan(t *testing.T) {
	origTimeout := resolver.DefaultLiveListTimeout
	resolver.DefaultLiveListTimeout = 100 * time.Millisecond
	t.Cleanup(func() { resolver.DefaultLiveListTimeout = origTimeout })

	// A cross-platform manager name so the fake participates on every OS
	// (AutomaticOnGOOS only restricts brew/linuxbrew).
	hanging := &hangingScanAdapter{name: "hangmgr"}
	healthy := &scanManagerNameAdapter{name: "healthymgr", installed: []string{"jq"}}

	origAll := adapter.All
	adapter.All = []adapter.Adapter{hanging, healthy}
	t.Cleanup(func() { adapter.All = origAll })
	origScanGOOS := scanGOOS
	scanGOOS = runtime.GOOS
	t.Cleanup(func() { scanGOOS = origScanGOOS })

	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	path := dir + "/genv.json"

	var code int
	done := make(chan struct{})
	var out string
	go func() {
		defer close(done)
		out = captureStdout(t, func() {
			code = run([]string{"scan", "--file", path, "--json"})
		})
	}()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("scan hung on a wedged manager inventory")
	}
	if code != exitOK {
		t.Fatalf("scan: expected exitOK (%d), got %d", exitOK, code)
	}
	var env map[string]any
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("scan --json output is not valid JSON: %v\noutput: %q", err, out)
	}
	if env["command"] != "scan" {
		t.Errorf("JSON command: got %v, want %q", env["command"], "scan")
	}
}

// hangingScanAdapter blocks forever in ListInstalled, standing in for a
// wedged winget first run. It also exercises the QueryVersion fallback path.
type hangingScanAdapter struct {
	name string
}

func (a *hangingScanAdapter) Name() string    { return a.name }
func (a *hangingScanAdapter) Available() bool { return true }
func (a *hangingScanAdapter) NormalizeID(id string, _ map[string]string) (string, bool) {
	return id, false
}
func (a *hangingScanAdapter) PlanInstall(pkgName string) []string { return []string{"true"} }
func (a *hangingScanAdapter) PlanUninstall(pkgName string) []string {
	return []string{"true"}
}
func (a *hangingScanAdapter) PlanUpgrade(pkgName string) []string { return []string{"true"} }
func (a *hangingScanAdapter) PlanClean() [][]string               { return nil }
func (a *hangingScanAdapter) Query(pkgName string) (bool, error)  { return false, nil }
func (a *hangingScanAdapter) ListInstalled() ([]string, error) {
	time.Sleep(time.Hour)
	return nil, nil
}
func (a *hangingScanAdapter) QueryVersion(pkgName string) (string, error) {
	time.Sleep(time.Hour)
	return "", nil
}

// TestScanCmd_TimeoutWarnsButContinues verifies the warn-and-skip contract:
// the hung manager produces a stderr warning and scan continues with the
// remaining managers instead of aborting.
func TestScanCmd_TimeoutWarnsButContinues(t *testing.T) {
	origTimeout := resolver.DefaultLiveListTimeout
	resolver.DefaultLiveListTimeout = 100 * time.Millisecond
	t.Cleanup(func() { resolver.DefaultLiveListTimeout = origTimeout })

	hanging := &hangingScanAdapter{name: "hangmgr"}
	healthy := &scanManagerNameAdapter{name: "healthymgr", installed: []string{"jq"}}

	origAll := adapter.All
	adapter.All = []adapter.Adapter{hanging, healthy}
	t.Cleanup(func() { adapter.All = origAll })
	origScanGOOS := scanGOOS
	scanGOOS = runtime.GOOS
	t.Cleanup(func() { scanGOOS = origScanGOOS })

	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	path := dir + "/genv.json"

	var code int
	var errOut string
	done := make(chan struct{})
	go func() {
		defer close(done)
		out := captureStderr(t, func() {
			code = run([]string{"scan", "--file", path, "--yes"})
		})
		errOut = out
	}()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("scan hung on a wedged manager inventory")
	}
	if code != exitOK {
		t.Fatalf("scan: expected exitOK (%d), got %d", exitOK, code)
	}
	if !strings.Contains(errOut, "hangmgr") || !strings.Contains(errOut, "timed out") {
		t.Fatalf("expected a timeout warning for hangmgr on stderr, got %q", errOut)
	}

	// The healthy manager's package should still have been adopted. On a v8
	// spec that lives under the active target, so check everywhere.
	f, err := genvfile.Read(path)
	if err != nil {
		t.Fatalf("read spec: %v", err)
	}
	found := false
	for _, p := range f.Packages {
		if p.ID == "jq" {
			found = true
		}
	}
	for _, target := range f.Targets {
		for _, p := range target.Packages {
			if p.ID == "jq" {
				found = true
			}
		}
	}
	if !found {
		t.Fatalf("healthy manager package jq not adopted; spec = %+v", f)
	}
}
