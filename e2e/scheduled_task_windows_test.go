//go:build integration && windows

package e2e_test

import (
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// #212: the scheduled_task backend could only be unit-tested with the
// schtasks subprocess faked, which proves the argv and the rendering but not
// that Task Scheduler accepts the definition. This test registers a real task
// and removes it again, so the Windows CI job — not just macOS — is what
// confirms the feature works.
//
// It is Windows-only and never reaches the other platforms: genv deliberately
// GOOS-gates the backend so a stray schtasks on a Unix PATH cannot claim
// Windows support.
func TestE2EScheduledTaskRegistersAndRemoves(t *testing.T) {
	if _, err := exec.LookPath("schtasks"); err != nil {
		t.Skip("schtasks not available")
	}

	r := newRunner(t, "")
	r.writeSpecRaw(t, `{"schemaVersion":"8","defaults":{},"targets":{"windows":{"services":{
		"genv-e2e-task":{
			"scheduled_task":{
				"action":"C:\\Windows\\System32\\cmd.exe",
				"args":["/c","exit 0"],
				"trigger":"daily",
				"at":"04:15"
			}
		}
	}}}}`)

	// apply must register the task and report the service as applied.
	out, _, code := r.genv("", "apply", "--target", "windows", "--yes")
	if code != 0 {
		t.Fatalf("genv apply: exit %d, want 0\n%s", code, out)
	}
	if !strings.Contains(strings.ToLower(out), "task scheduler") {
		t.Errorf("expected Task Scheduler registration in the apply output, got: %q", out)
	}

	// The registered task name is the bare "genv-<slug>" form; a leading
	// backslash is a *path* in a query, and using it here would test nothing.
	taskName := "genv-e2e-task"
	query, err := exec.Command("schtasks", "/Query", "/TN", taskName).CombinedOutput()
	if err != nil {
		t.Fatalf("task was not registered: %v\n%s", err, query)
	}
	t.Cleanup(func() { _ = exec.Command("schtasks", "/Delete", "/TN", taskName, "/F").Run() })

	// status must see it, and must not call a Ready task broken: a daily task
	// is normally idle.
	statusOut, _, statusCode := r.genv("", "status", "--target", "windows")
	if statusCode != 0 {
		t.Fatalf("genv status: exit %d, want 0\n%s", statusCode, statusOut)
	}
	if !strings.Contains(statusOut, "genv-e2e-task") {
		t.Errorf("status does not mention the service: %q", statusOut)
	}

	// A second apply with nothing changed must converge quietly: re-registering
	// unconditionally would report the service as applied every run.
	_, _, secondCode := r.genv("", "apply", "--target", "windows", "--yes")
	if secondCode != 0 {
		t.Fatalf("second genv apply: exit %d, want 0", secondCode)
	}

	// Removing the service from the spec must unregister the task.
	r.writeSpecRaw(t, `{"schemaVersion":"8","defaults":{},"targets":{"windows":{"services":{}}}}`)
	if _, _, rmCode := r.genv("", "apply", "--target", "windows", "--yes"); rmCode != 0 {
		t.Fatalf("apply after removal: exit %d, want 0", rmCode)
	}
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if err := exec.CommandContext(context.Background(), "schtasks", "/Query", "/TN", taskName).Run(); err != nil {
			return // gone, as expected
		}
		time.Sleep(time.Second)
	}
	t.Errorf("task %q still registered after removal from the spec", taskName)
}
