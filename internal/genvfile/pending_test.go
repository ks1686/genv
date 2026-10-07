package genvfile

import (
	"os"
	"path/filepath"
	"testing"
)

// A pending action is the crash-safety record for a dependency-aware change:
// genv writes it before touching a watched package and clears it only after the
// restart and its readiness check both succeeded. If genv dies in between, the
// next run finds the record and reports the uncertainty instead of assuming the
// service was restarted.
func TestPendingAction_round_trips_through_the_lock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "genv.lock.json")
	lf := &LockFile{SchemaVersion: "8"}
	lf.PendingActions = []PendingAction{{
		Name:       "api",
		Package:    "postgres",
		Reason:     "package upgraded; restart and readiness check did not finish",
		RecordedAt: "2026-10-06T12:00:00Z",
	}}
	if err := WriteLock(path, lf); err != nil {
		t.Fatalf("WriteLock: %v", err)
	}
	got, err := ReadLock(path)
	if err != nil {
		t.Fatalf("ReadLock: %v", err)
	}
	if len(got.PendingActions) != 1 {
		t.Fatalf("pending actions = %+v, want one", got.PendingActions)
	}
	pa := got.PendingActions[0]
	if pa.Name != "api" || pa.Package != "postgres" || pa.RecordedAt != "2026-10-06T12:00:00Z" {
		t.Errorf("pending action did not round-trip: %+v", pa)
	}
	if pa.Reason == "" {
		t.Error("pending action should carry a reason a user can act on")
	}
}

func TestPendingAction_omitted_when_empty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "genv.lock.json")
	if err := WriteLock(path, &LockFile{SchemaVersion: "8"}); err != nil {
		t.Fatalf("WriteLock: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if want := "pendingActions"; containsBytes(data, want) {
		t.Errorf("an empty pending list should be omitted, got %s", data)
	}
}

func TestLockWithPendingFor(t *testing.T) {
	lf := &LockFile{PendingActions: []PendingAction{
		{Name: "api", Package: "postgres"},
		{Name: "web", Package: "postgres"},
	}}
	got := lf.PendingFor("postgres")
	if len(got) != 2 {
		t.Errorf("PendingFor = %+v, want both api and web", got)
	}
	if len(lf.PendingFor("redis")) != 0 {
		t.Errorf("PendingFor(redis) = %+v, want none", lf.PendingFor("redis"))
	}
	var empty *LockFile
	if len(empty.PendingFor("postgres")) != 0 {
		t.Error("PendingFor on a nil lock should be empty")
	}
}

func TestClearPendingActions(t *testing.T) {
	lf := &LockFile{PendingActions: []PendingAction{{Name: "api", Package: "postgres"}}}
	if lf.ClearPendingActions("api") != 1 {
		t.Errorf("ClearPendingActions = %d, want 1", lf.ClearPendingActions("api"))
	}
	if len(lf.PendingActions) != 0 {
		t.Errorf("pending actions = %+v, want cleared", lf.PendingActions)
	}
	if lf.ClearPendingActions("api") != 0 {
		t.Error("clearing twice should report nothing removed")
	}
}

func containsBytes(haystack []byte, needle string) bool {
	return len(needle) > 0 && len(haystack) >= len(needle) &&
		stringIndex(string(haystack), needle) >= 0
}

func stringIndex(haystack, needle string) int {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}
