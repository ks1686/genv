package service

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ks1686/genv/internal/genvfile"
	"github.com/ks1686/genv/internal/schema"
)

// These cover the parts of the scheduled_task backend that do not need a real
// Task Scheduler: the spec-to-lock round trip, drift detection, and the
// schtasks invocation, driven through the existing schtasksRun seam. Actual
// registration is Windows-only and is exercised by the Windows CI job.
func scheduledTaskService() schema.Service {
	return schema.Service{ScheduledTask: &schema.ScheduledTaskSpec{
		Action:  `C:\Program Files\Acme\agent.exe`,
		Args:    []string{"--serve"},
		Trigger: "daily",
		At:      "03:00",
	}}
}

func TestSpecToLock_RecordsScheduledTask(t *testing.T) {
	lock := SpecToLock(map[string]schema.Service{"acme": scheduledTaskService()}, "")
	if len(lock) != 1 {
		t.Fatalf("lock = %d entries, want 1", len(lock))
	}
	l := lock[0]
	if l.ScheduledTaskName == "" {
		t.Error("registered task name not recorded; removal would have nothing to delete")
	}
	if l.ScheduledTaskFingerprint == "" {
		t.Error("fingerprint not recorded; a changed spec could never be reported as drift")
	}
}

func TestSpecToLock_NoTaskForPlainService(t *testing.T) {
	lock := SpecToLock(map[string]schema.Service{"a": {Start: []string{"sleep", "1"}}}, "")
	if lock[0].ScheduledTaskFingerprint != "" || lock[0].ScheduledTaskName != "" {
		t.Errorf("a plain service must not acquire task fields: %+v", lock[0])
	}
}

func TestCompareServices_TaskChangeIsDrift(t *testing.T) {
	svc := scheduledTaskService()
	lock := SpecToLock(map[string]schema.Service{"acme": svc}, "")[0]

	if !compareServices(svc, lock) {
		t.Error("an unchanged spec must compare equal to its own lock")
	}

	for name, mutate := range map[string]func(s *schema.Service){
		"action":    func(s *schema.Service) { s.ScheduledTask.Action = `C:\other.exe` },
		"args":      func(s *schema.Service) { s.ScheduledTask.Args = []string{"--serve", "--verbose"} },
		"trigger":   func(s *schema.Service) { s.ScheduledTask.Trigger = "weekly" },
		"at":        func(s *schema.Service) { s.ScheduledTask.At = "04:00" },
		"day":       func(s *schema.Service) { s.ScheduledTask.DayOfWeek = "friday" },
		"principal": func(s *schema.Service) { s.ScheduledTask.Principal = "system" },
		"restart":   func(s *schema.Service) { s.ScheduledTask.RestartOnFailure = true },
	} {
		t.Run(name, func(t *testing.T) {
			changed := scheduledTaskService()
			mutate(&changed)
			if compareServices(changed, lock) {
				t.Errorf("changing %s must be reported as drift, otherwise the old task stays registered", name)
			}
		})
	}
}

func TestServiceStatus_ChangedTaskIsModified(t *testing.T) {
	svc := scheduledTaskService()
	lock := SpecToLock(map[string]schema.Service{"acme": svc}, "")
	svc.ScheduledTask.At = "05:00"

	entries := ServiceStatus(map[string]schema.Service{"acme": svc}, lock, false, "")
	if len(entries) != 1 || entries[0].Kind != ServiceStatusModified {
		t.Fatalf("entries = %+v, want a single modified entry", entries)
	}
}

// A task registered outside genv — or removed by hand — must come back on the
// next apply. Comparing the lock alone would call this ok forever, which is the
// same failure mode as #213.
func TestApplyServices_RecreatesUnregisteredTask(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("LOCALAPPDATA", filepath.Join(home, "AppData", "Local"))

	var created, deleted int
	orig := schtasksRun
	schtasksRun = func(ctx context.Context, args ...string) ([]byte, error) {
		joined := strings.Join(args, " ")
		switch {
		case strings.Contains(joined, "/Create"):
			created++
		case strings.Contains(joined, "/Delete"):
			deleted++
		}
		return []byte(""), nil
	}
	t.Cleanup(func() { schtasksRun = orig })

	// Force the Windows path on a non-Windows host: the seam replaces the
	// subprocess, and IsSchtasksAvailable is the only GOOS gate in the way.
	origProbe := probeSchtasksServicesFn
	probeSchtasksServicesFn = func() bool { return true }
	t.Cleanup(func() { probeSchtasksServicesFn = origProbe })

	svc := scheduledTaskService()
	lock := SpecToLock(map[string]schema.Service{"acme": svc}, "")

	applied, removed, errs := ApplyServices(context.Background(), map[string]schema.Service{"acme": svc}, lock, false, "")
	if len(errs) != 0 {
		t.Fatalf("apply errors: %v", errs)
	}
	if len(removed) != 0 {
		t.Errorf("removed = %v, want none", removed)
	}
	if created != 1 {
		t.Errorf("schtasks /Create called %d times, want 1", created)
	}
	if len(applied) == 0 {
		t.Error("apply should report the service as applied")
	}
}

func TestApplyServices_RemovesTaskWhenServiceLeavesSpec(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("LOCALAPPDATA", filepath.Join(home, "AppData", "Local"))

	var deleted int
	orig := schtasksRun
	schtasksRun = func(ctx context.Context, args ...string) ([]byte, error) {
		if strings.Contains(strings.Join(args, " "), "/Delete") {
			deleted++
		}
		return []byte(""), nil
	}
	t.Cleanup(func() { schtasksRun = orig })
	origProbe := probeSchtasksServicesFn
	probeSchtasksServicesFn = func() bool { return true }
	t.Cleanup(func() { probeSchtasksServicesFn = origProbe })

	lock := SpecToLock(map[string]schema.Service{"acme": scheduledTaskService()}, "")
	_, removed, errs := ApplyServices(context.Background(), map[string]schema.Service{}, lock, false, "")
	if len(errs) != 0 {
		t.Fatalf("apply errors: %v", errs)
	}
	if deleted != 1 {
		t.Errorf("schtasks /Delete called %d times, want 1", deleted)
	}
	if len(removed) != 1 || removed[0] != "acme" {
		t.Errorf("removed = %v, want [acme]", removed)
	}
}

// A task that vanished must be re-registered, not treated as healthy.
func TestApplyServices_ReRegistersWhenTaskIsMissing(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("LOCALAPPDATA", filepath.Join(home, "AppData", "Local"))

	var created int
	orig := schtasksRun
	schtasksRun = func(ctx context.Context, args ...string) ([]byte, error) {
		joined := strings.Join(args, " ")
		if strings.Contains(joined, "/Create") {
			created++
		}
		if strings.Contains(joined, "/Query") {
			return []byte("ERROR: The system cannot find the file specified.\n"), nil
		}
		return []byte(""), nil
	}
	t.Cleanup(func() { schtasksRun = orig })
	origProbe := probeSchtasksServicesFn
	probeSchtasksServicesFn = func() bool { return true }
	t.Cleanup(func() { probeSchtasksServicesFn = origProbe })

	svc := scheduledTaskService()
	lock := SpecToLock(map[string]schema.Service{"acme": svc}, "")
	_, _, errs := ApplyServices(context.Background(), map[string]schema.Service{"acme": svc}, lock, false, "")
	if len(errs) != 0 {
		t.Fatalf("apply errors: %v", errs)
	}
	if created != 1 {
		t.Errorf("a missing task must be re-registered; /Create called %d times", created)
	}
}

// The generated artifacts are real files the user can inspect and edit.
func TestApplyScheduledTask_WritesArtifacts(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("LOCALAPPDATA", filepath.Join(home, "AppData", "Local"))

	orig := schtasksRun
	schtasksRun = func(ctx context.Context, args ...string) ([]byte, error) { return []byte(""), nil }
	t.Cleanup(func() { schtasksRun = orig })

	if _, err := applyScheduledTask(context.Background(), "acme", scheduledTaskService().ScheduledTask, false); err != nil {
		t.Fatalf("applyScheduledTask: %v", err)
	}
	cmdPath, vbsPath, xmlPath, err := schtasksServiceFiles("acme")
	if err != nil {
		t.Fatalf("files: %v", err)
	}
	for _, p := range []string{cmdPath, vbsPath, xmlPath} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("artifact %q not written: %v", p, err)
		}
	}
	cmd, _ := os.ReadFile(cmdPath)
	if !strings.Contains(string(cmd), `--serve`) {
		t.Errorf("cmd wrapper does not carry the declared args:\n%s", cmd)
	}
}

// Non-Windows hosts must skip a declared task rather than erroring: one spec
// can target several platforms.
func TestApplyDeclared_SkipsTaskWhenSchtasksUnavailable(t *testing.T) {
	if probeSchtasksServices() {
		t.Skip("running on Windows")
	}
	changed, err := applyDeclared(context.Background(), "acme", scheduledTaskService(), "", false)
	if err != nil {
		t.Fatalf("a declared task on a host without Task Scheduler must be skipped, got: %v", err)
	}
	if changed {
		t.Error("nothing was registered, so nothing should be reported as applied")
	}
}

func TestLockedServiceTaskFieldsRoundTrip(t *testing.T) {
	// The lock is machine-local JSON, so a field added here must survive a
	// write/read cycle with omitempty intact.
	l := genvfile.LockedService{Name: "a", ScheduledTaskName: `\\ks1686\genv-a`, ScheduledTaskFingerprint: "sha256:abc"}
	if l.ScheduledTaskName == "" || l.ScheduledTaskFingerprint == "" {
		t.Fatal("fields not set")
	}
	empty := genvfile.LockedService{Name: "b"}
	if empty.ScheduledTaskName != "" {
		t.Error("empty task fields must stay empty for services that declare none")
	}
}

// A task that is already correct must be left alone. Apply runs on every
// reconcile, so a backend that always re-registers reports every service as
// changed on every run, which makes a real change indistinguishable from a
// no-op and trains the user to ignore the output.
func TestApplyScheduledTask_UnchangedTaskIsNotReapplied(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("LOCALAPPDATA", filepath.Join(home, "AppData", "Local"))

	var created int
	orig := schtasksRun
	schtasksRun = func(ctx context.Context, args ...string) ([]byte, error) {
		if strings.Contains(strings.Join(args, " "), "/Create") {
			created++
		}
		return []byte("Status:  Ready\n"), nil
	}
	t.Cleanup(func() { schtasksRun = orig })

	spec := scheduledTaskService().ScheduledTask
	changed, err := applyScheduledTask(context.Background(), "acme", spec, false)
	if err != nil {
		t.Fatalf("first apply: %v", err)
	}
	if !changed || created != 1 {
		t.Fatalf("first apply: changed=%v created=%d, want true/1", changed, created)
	}

	changed, err = applyScheduledTask(context.Background(), "acme", spec, false)
	if err != nil {
		t.Fatalf("second apply: %v", err)
	}
	if changed {
		t.Error("an unchanged task must not be reported as changed")
	}
	if created != 1 {
		t.Errorf("schtasks /Create called %d times, want 1: the task was re-registered on a no-op apply", created)
	}
}

// A changed trigger must re-register.
func TestApplyScheduledTask_ChangedSpecReregisters(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("LOCALAPPDATA", filepath.Join(home, "AppData", "Local"))

	var created int
	orig := schtasksRun
	schtasksRun = func(ctx context.Context, args ...string) ([]byte, error) {
		if strings.Contains(strings.Join(args, " "), "/Create") {
			created++
		}
		return []byte("Status:  Ready\n"), nil
	}
	t.Cleanup(func() { schtasksRun = orig })

	svc := scheduledTaskService()
	if _, err := applyScheduledTask(context.Background(), "acme", svc.ScheduledTask, false); err != nil {
		t.Fatalf("first apply: %v", err)
	}
	svc.ScheduledTask.At = "04:30"
	changed, err := applyScheduledTask(context.Background(), "acme", svc.ScheduledTask, false)
	if err != nil {
		t.Fatalf("second apply: %v", err)
	}
	if !changed || created != 2 {
		t.Errorf("a changed start time must re-register: changed=%v created=%d", changed, created)
	}
}
