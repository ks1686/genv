package resolver

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/ks1686/genv/internal/adapter"
	"github.com/ks1686/genv/internal/genvfile"
)

// swapLiveListTimeout shortens the inventory deadline for the test.
func swapLiveListTimeout(t *testing.T, d time.Duration) {
	t.Helper()
	orig := DefaultLiveListTimeout
	DefaultLiveListTimeout = d
	t.Cleanup(func() { DefaultLiveListTimeout = orig })
}

// hangingOutdatedMgr is an OutdatedLister whose query blocks forever, standing
// in for a wedged `winget upgrade` / `composer global show`.
type hangingOutdatedMgr struct {
	outdatedTestMgr
}

func (m *hangingOutdatedMgr) ListOutdated([]string) (map[string]string, error) {
	select {}
}

// TestRunTimed_SuccessAndTimeout covers the error-only variant used by
// service probes.
func TestRunTimed_SuccessAndTimeout(t *testing.T) {
	if err := RunTimed(func() error { return nil }, 50*time.Millisecond); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
	if err := RunTimed(func() error { return errors.New("boom") }, time.Second); err == nil || err.Error() != "boom" {
		t.Fatalf("expected boom to pass through, got %v", err)
	}
	started := time.Now()
	err := RunTimed(func() error { time.Sleep(time.Hour); return nil }, 50*time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("err = %v, want timeout", err)
	}
	if time.Since(started) > 2*time.Second {
		t.Fatalf("timeout took %s, want well under 2s", time.Since(started))
	}
}

// TestCallTimed_PreservesValue verifies the generic passthrough on success.
func TestCallTimed_PreservesValue(t *testing.T) {
	got, err := CallTimed(func() (int, error) { return 42, nil }, time.Second)
	if err != nil || got != 42 {
		t.Fatalf("got (%d, %v), want (42, nil)", got, err)
	}
}

// TestFilterOutdated_TimeoutKeepsAll is the silent-upgrade regression test:
// when a manager's outdated query wedges, FilterOutdated must keep ALL of that
// manager's packages (conservative) — never interpret the timeout as "nothing
// is outdated", which would silently skip real upgrades.
func TestFilterOutdated_TimeoutKeepsAll(t *testing.T) {
	swapLiveListTimeout(t, 80*time.Millisecond)
	swapLookupAdapter(t, map[string]adapter.Adapter{
		"brew": &hangingOutdatedMgr{outdatedTestMgr{name: "brew"}},
	})
	packages := []genvfile.LockedPackage{
		{ID: "wget", Manager: "brew", PkgName: "wget"},
		{ID: "jq", Manager: "brew", PkgName: "jq"},
	}
	kept, warnings := FilterOutdated(packages)
	if got := keptIDs(kept); len(got) != 2 {
		t.Fatalf("kept = %v, want all packages kept on timeout", got)
	}
	if len(warnings) == 0 || !strings.Contains(strings.Join(warnings, "\n"), "timed out") {
		t.Fatalf("warnings = %v, want a timeout warning", warnings)
	}
}

// hangingVersionListerMgr blocks in ListInstalledVersions, like a wedged
// `apk info -v`, while its mutating command succeeds.
type hangingVersionListerMgr struct {
	plainTestMgr
	listStarted chan struct{}
}

func (m *hangingVersionListerMgr) ListInstalledVersions() (map[string]string, error) {
	if m.listStarted != nil {
		close(m.listStarted)
	}
	select {}
}

// TestExecuteUpgrade_HangingVersionProbeStillRecordsUpgrade: a manager whose
// version probe wedges after a successful upgrade must not hang the lock
// update. The upgrade is still recorded; since the new version is unknown,
// the previously recorded version is carried over unchanged.
func TestExecuteUpgrade_HangingVersionProbeStillRecordsUpgrade(t *testing.T) {
	swapLiveListTimeout(t, 80*time.Millisecond)
	mgr := &hangingVersionListerMgr{plainTestMgr: plainTestMgr{name: "hanglister"}}
	plan := []UpgradeAction{
		{
			LPs: []genvfile.LockedPackage{
				{ID: "git", Manager: "hanglister", PkgName: "git", InstalledVersion: "2.44.0"},
			},
			Mgr: mgr,
			Cmd: []string{"true"},
		},
	}

	done := make(chan UpgradeExecution, 1)
	go func() {
		done <- ExecuteUpgrade(context.Background(), plan, nil, &bytes.Buffer{}, &bytes.Buffer{})
	}()
	select {
	case out := <-done:
		if len(out.Errors) != 0 {
			t.Fatalf("expected no errors, got %v", out.Errors)
		}
		if len(out.Upgraded) != 1 {
			t.Fatalf("expected the upgrade to be recorded, got %d", len(out.Upgraded))
		}
		if out.Upgraded[0].InstalledVersion != "2.44.0" {
			t.Fatalf("version = %q, want previous %q carried over (probe timed out)", out.Upgraded[0].InstalledVersion, "2.44.0")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("ExecuteUpgrade hung on a wedged version probe")
	}
}
