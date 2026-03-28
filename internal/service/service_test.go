package service

import (
	"context"
	"strings"
	"testing"

	"github.com/ks1686/genv/internal/genvfile"
	"github.com/ks1686/genv/internal/schema"
)

func TestServiceStatus(t *testing.T) {
	spec := map[string]schema.Service{
		"ok-service": {
			Start:  []string{"echo", "start"},
			Status: []string{"true"},
		},
		"modified-service": {
			Start:  []string{"echo", "new-start"},
			Status: []string{"true"},
		},
		"missing-service": {
			Start:  []string{"echo", "missing"},
			Status: []string{"true"},
		},
	}

	lock := []genvfile.LockedService{
		{
			Name:   "ok-service",
			Start:  []string{"echo", "start"},
			Status: []string{"true"},
		},
		{
			Name:   "modified-service",
			Start:  []string{"echo", "old-start"},
			Status: []string{"true"},
		},
		{
			Name:   "extra-service",
			Start:  []string{"echo", "extra"},
			Status: []string{"true"},
		},
	}

	entries := ServiceStatus(spec, lock)

	expected := map[string]ServiceStatusKind{
		"ok-service":       ServiceStatusOK,
		"modified-service": ServiceStatusModified,
		"missing-service":  ServiceStatusMissing,
		"extra-service":    ServiceStatusExtra,
	}

	if len(entries) != len(expected) {
		t.Errorf("expected %d entries, got %d", len(expected), len(entries))
	}

	for _, e := range entries {
		expKind, ok := expected[e.Name]
		if !ok {
			t.Errorf("unexpected entry for service %q", e.Name)
			continue
		}
		if e.Kind != expKind {
			t.Errorf("service %q: expected kind %s, got %s", e.Name, expKind, e.Kind)
		}
	}
}

func TestApplyServices(t *testing.T) {
	ctx := context.Background()

	// Case: start a missing service
	spec := map[string]schema.Service{
		"new-svc": {Start: []string{"true"}},
	}
	lock := []genvfile.LockedService{}

	applied, removed, errs := ApplyServices(ctx, spec, lock, false)
	if len(errs) > 0 {
		t.Errorf("ApplyServices failed: %v", errs[0])
	}
	if len(applied) != 1 || applied[0] != "new-svc" {
		t.Errorf("expected 1 applied service (new-svc), got %v", applied)
	}
	if len(removed) != 0 {
		t.Errorf("expected 0 removed services, got %v", removed)
	}

	// Case: stop an extra service
	spec = map[string]schema.Service{}
	lock = []genvfile.LockedService{
		{Name: "extra-svc", Stop: []string{"true"}},
	}

	applied, removed, errs = ApplyServices(ctx, spec, lock, false)
	if len(errs) > 0 {
		t.Errorf("ApplyServices failed: %v", errs[0])
	}
	if len(applied) != 0 {
		t.Errorf("expected 0 applied services, got %v", applied)
	}
	if len(removed) != 1 || removed[0] != "extra-svc" {
		t.Errorf("expected 1 removed service (extra-svc), got %v", removed)
	}
}

func TestApplyServicesFailure(t *testing.T) {
	// When the start command exits non-zero (direct execution path only), the
	// service must appear in errs and must NOT appear in applied.
	// systemd and launchd start services asynchronously, so this test only
	// applies to the direct-execution fallback path.
	if IsSystemdAvailable() || IsLaunchdAvailable() {
		t.Skip("skipping direct-execution failure test: systemd/launchd present")
	}

	ctx := context.Background()
	spec := map[string]schema.Service{
		"bad-svc": {Start: []string{"false"}},
	}

	applied, removed, errs := ApplyServices(ctx, spec, nil, false)
	if len(errs) == 0 {
		t.Error("expected an error when start command fails, got none")
	}
	if len(applied) != 0 {
		t.Errorf("expected 0 applied services on failure, got %v", applied)
	}
	if len(removed) != 0 {
		t.Errorf("expected 0 removed services, got %v", removed)
	}
}

func TestApplyServicesModified(t *testing.T) {
	// A service whose config changed vs the lock must be re-applied.
	ctx := context.Background()
	spec := map[string]schema.Service{
		"mod-svc": {Start: []string{"true"}},
	}
	lock := []genvfile.LockedService{
		{Name: "mod-svc", Start: []string{"echo", "old"}},
	}

	applied, _, errs := ApplyServices(ctx, spec, lock, false)
	if len(errs) > 0 {
		t.Errorf("unexpected error: %v", errs[0])
	}
	if len(applied) != 1 || applied[0] != "mod-svc" {
		t.Errorf("expected mod-svc to be re-applied, got %v", applied)
	}
}

func TestApplyServicesExtraNoStop(t *testing.T) {
	// An extra service with no stop command should be removed from tracking
	// without error (no process to stop).
	ctx := context.Background()
	lock := []genvfile.LockedService{
		{Name: "nostop-svc"},
	}

	_, removed, errs := ApplyServices(ctx, nil, lock, false)
	if len(errs) > 0 {
		t.Errorf("unexpected error: %v", errs[0])
	}
	if len(removed) != 1 || removed[0] != "nostop-svc" {
		t.Errorf("expected nostop-svc removed, got %v", removed)
	}
}

func TestSpecToLock(t *testing.T) {
	spec := map[string]schema.Service{
		"svc-b": {Start: []string{"b"}, Stop: []string{"stop-b"}},
		"svc-a": {Start: []string{"a"}, Status: []string{"check-a"}},
	}

	lock := SpecToLock(spec)

	if len(lock) != 2 {
		t.Fatalf("expected 2 locked services, got %d", len(lock))
	}
	// SpecToLock must return entries sorted by name.
	if lock[0].Name != "svc-a" || lock[1].Name != "svc-b" {
		t.Errorf("expected sorted order [svc-a, svc-b], got [%s, %s]", lock[0].Name, lock[1].Name)
	}
	if lock[0].Status[0] != "check-a" {
		t.Errorf("expected Status[0]=check-a, got %v", lock[0].Status)
	}
	if lock[1].Stop[0] != "stop-b" {
		t.Errorf("expected Stop[0]=stop-b, got %v", lock[1].Stop)
	}
}

func TestSpecToLockEmpty(t *testing.T) {
	if got := SpecToLock(nil); got != nil {
		t.Errorf("expected nil for empty spec, got %v", got)
	}
	if got := SpecToLock(map[string]schema.Service{}); got != nil {
		t.Errorf("expected nil for empty map, got %v", got)
	}
}

func TestSystemdUnitContent(t *testing.T) {
	svc := schema.Service{
		Start:   []string{"/usr/bin/my-daemon", "--config", "/etc/my.conf"},
		Stop:    []string{"/usr/bin/my-daemon", "--stop"},
		Restart: []string{"/usr/bin/my-daemon", "--reload"},
	}

	content := SystemdUnitContent("my-daemon", svc)

	checks := []string{
		"Description=genv managed service: my-daemon",
		"ExecStart=/usr/bin/my-daemon --config /etc/my.conf",
		"ExecStop=/usr/bin/my-daemon --stop",
		"ExecReload=/usr/bin/my-daemon --reload",
		"WantedBy=default.target",
	}
	for _, want := range checks {
		if !strings.Contains(content, want) {
			t.Errorf("systemd unit content missing %q\ncontent:\n%s", want, content)
		}
	}
}

func TestSystemdUnitContentMinimal(t *testing.T) {
	// Stop and Restart are optional — they must not appear when absent.
	svc := schema.Service{Start: []string{"sleep", "inf"}}
	content := SystemdUnitContent("minimal", svc)

	if !strings.Contains(content, "ExecStart=sleep inf") {
		t.Errorf("missing ExecStart in unit:\n%s", content)
	}
	if strings.Contains(content, "ExecStop") {
		t.Errorf("ExecStop must be absent when no stop command is set:\n%s", content)
	}
	if strings.Contains(content, "ExecReload") {
		t.Errorf("ExecReload must be absent when no restart command is set:\n%s", content)
	}
}

func TestLaunchdPlistContent(t *testing.T) {
	svc := schema.Service{
		Start: []string{"/usr/local/bin/myapp", "--port", "8080"},
	}

	content := LaunchdPlistContent("myapp", svc)

	checks := []string{
		`<string>genv.myapp</string>`,
		`<string>/usr/local/bin/myapp</string>`,
		`<string>--port</string>`,
		`<string>8080</string>`,
		`<true/>`,
	}
	for _, want := range checks {
		if !strings.Contains(content, want) {
			t.Errorf("launchd plist missing %q\ncontent:\n%s", want, content)
		}
	}
}

func TestSystemdUnitName_PathTraversal(t *testing.T) {
	name := "../../../evil"
	unitName := systemdUnitName(name)
	if strings.Contains(unitName, "/") || strings.Contains(unitName, "..") {
		t.Errorf("systemdUnitName(%q) = %q; want no directory traversal characters", name, unitName)
	}

	plistName := launchdPlistName(name)
	if strings.Contains(plistName, "/") || strings.Contains(plistName, "..") {
		t.Errorf("launchdPlistName(%q) = %q; want no directory traversal characters", name, plistName)
	}
}
