package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ks1686/genv/internal/genvfile"
	"github.com/ks1686/genv/internal/testutil"
)

// writeRestartSpec writes a schemaVersion 10 spec whose services use shell
// commands that record their argv, so a test can prove the order and the exact
// binary. v10 because requires, watch and health_check are refused below it.
func writeRestartSpec(t *testing.T, services string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "genv.json")
	spec := `{"schemaVersion":"10","targets":{"macos":{"services":{` + services + `}}}}`
	if err := os.WriteFile(path, []byte(spec), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// restartArgs builds the command line for the macos target of a fixture spec.
func restartArgs(path string, name string, extra ...string) []string {
	return append([]string{"service", "restart", name, "--file", path, "--target", "macos"}, extra...)
}

func TestServiceRestartRunsStopThenStart(t *testing.T) {
	dir := t.TempDir()
	testutil.SetHome(t, dir)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, ".config"))
	logPath := filepath.Join(dir, "calls.log")
	path := writeRestartSpec(t, `"worker":{"start":["sh","-c","echo start >> `+logPath+`"],"stop":["sh","-c","echo stop >> `+logPath+`"]}`)

	out := captureStdout(t, func() {
		if code := run(restartArgs(path, "worker")); code != exitOK {
			t.Fatalf("service restart: exit %d", code)
		}
	})
	if !strings.Contains(out, "Restarted service") {
		t.Errorf("output does not report the restart: %q", out)
	}
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("no commands recorded: %v", err)
	}
	if string(data) != "stop\nstart\n" {
		t.Fatalf("command order = %q, want stop then start", data)
	}
}

func TestServiceRestartUsesDeclaredRestartCommand(t *testing.T) {
	dir := t.TempDir()
	testutil.SetHome(t, dir)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, ".config"))
	logPath := filepath.Join(dir, "calls.log")
	// The declared restart command runs once; stop/start are not also run.
	path := writeRestartSpec(t, `"worker":{"start":["sh","-c","echo start >> `+logPath+
		`"],"stop":["sh","-c","echo stop >> `+logPath+`"],"restart":["sh","-c","echo restart >> `+logPath+`"]}`)

	captureStdout(t, func() {
		if code := run(restartArgs(path, "worker")); code != exitOK {
			t.Fatalf("service restart: exit %d", code)
		}
	})
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("no commands recorded: %v", err)
	}
	if string(data) != "restart\n" {
		t.Fatalf("recorded = %q, want only the declared restart command", data)
	}
}

func TestServiceRestartFailureExitsLogic(t *testing.T) {
	dir := t.TempDir()
	testutil.SetHome(t, dir)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, ".config"))
	path := writeRestartSpec(t, `"broken":{"start":["false"],"stop":["true"]}`)

	var code int
	errOut := captureStderr(t, func() {
		captureStdout(t, func() {
			code = run(restartArgs(path, "broken"))
		})
	})
	if code != exitLogic {
		t.Fatalf("failed restart: exit %d, want %d", code, exitLogic)
	}
	if !strings.Contains(errOut, "failed to restart") {
		t.Errorf("stderr does not name the failure: %q", errOut)
	}
}

func TestServiceRestartUnknownService(t *testing.T) {
	path := writeRestartSpec(t, `"worker":{"start":["true"],"stop":["true"]}`)

	errOut := captureStderr(t, func() {
		if code := run(restartArgs(path, "nope")); code != exitLogic {
			t.Fatalf("unknown service: exit %d, want %d", code, exitLogic)
		}
	})
	if !strings.Contains(errOut, `service "nope" not found`) {
		t.Errorf("stderr = %q, want the service named as unknown", errOut)
	}
}

func TestServiceRestartRefusesWithoutStopCommand(t *testing.T) {
	path := writeRestartSpec(t, `"startonly":{"start":["true"]}`)

	errOut := captureStderr(t, func() {
		if code := run(restartArgs(path, "startonly")); code != exitLogic {
			t.Fatalf("start-only service: exit %d, want %d", code, exitLogic)
		}
	})
	if !strings.Contains(errOut, "no stop command") {
		t.Errorf("stderr = %q, want the missing stop command named", errOut)
	}
}

func TestServiceRestartRefusesScheduledTask(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "genv.json")
	spec := `{"schemaVersion":"10","targets":{"macos":{"services":{"job":{"scheduled_task":{"action":"C:\\genv.exe","trigger":"daily","at":"09:00"}}}}}}`
	if err := os.WriteFile(path, []byte(spec), 0o644); err != nil {
		t.Fatal(err)
	}

	errOut := captureStderr(t, func() {
		if code := run(restartArgs(path, "job")); code != exitLogic {
			t.Fatalf("scheduled task: exit %d, want %d", code, exitLogic)
		}
	})
	if !strings.Contains(errOut, "scheduled_task") {
		t.Errorf("stderr = %q, want the refusal to name scheduled_task", errOut)
	}
}

// A service whose dependency is down is reported, and the dependency is not
// restarted behind the operator's back.
func TestServiceRestartRefusesWhenDependencyIsDown(t *testing.T) {
	dir := t.TempDir()
	testutil.SetHome(t, dir)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, ".config"))
	dbLog := filepath.Join(dir, "db.log")
	apiLog := filepath.Join(dir, "api.log")
	path := writeRestartSpec(t,
		`"db":{"start":["sh","-c","echo start >> `+dbLog+`"],"stop":["sh","-c","echo stop >> `+dbLog+`"],"status":["false"]},`+
			`"api":{"start":["sh","-c","echo start >> `+apiLog+`"],"stop":["sh","-c","echo stop >> `+apiLog+`"],"status":["false"],"requires":["db"]}`)

	errOut := captureStderr(t, func() {
		if code := run(restartArgs(path, "api")); code != exitLogic {
			t.Fatalf("dependency down: exit %d, want %d", code, exitLogic)
		}
	})
	if !strings.Contains(errOut, "requires db") {
		t.Errorf("stderr = %q, want the down dependency named", errOut)
	}
	for _, untouched := range []string{apiLog, dbLog} {
		if _, err := os.Stat(untouched); err == nil {
			t.Errorf("%s ran; nothing may be touched when a dependency is down", untouched)
		}
	}
}

// The same dependency, reported as running, must not block the restart.
func TestServiceRestartProceedsWhenDependencyIsUp(t *testing.T) {
	dir := t.TempDir()
	testutil.SetHome(t, dir)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, ".config"))
	apiLog := filepath.Join(dir, "api.log")
	path := writeRestartSpec(t,
		`"db":{"start":["true"],"stop":["true"],"status":["true"]},`+
			`"api":{"start":["sh","-c","echo start >> `+apiLog+`"],"stop":["sh","-c","echo stop >> `+apiLog+`"],"status":["true"],"requires":["db"]}`)

	captureStdout(t, func() {
		if code := run(restartArgs(path, "api")); code != exitOK {
			t.Fatalf("restart with a live dependency: exit %d", code)
		}
	})
	data, err := os.ReadFile(apiLog)
	if err != nil || string(data) != "stop\nstart\n" {
		t.Fatalf("api restart = %q error=%v, want stop then start", data, err)
	}
}

// Readiness is a separate verdict: a restart that succeeds on an unhealthy
// service is still a failure of the command.
func TestServiceRestartReportsReadinessSeparately(t *testing.T) {
	dir := t.TempDir()
	testutil.SetHome(t, dir)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, ".config"))
	path := writeRestartSpec(t,
		`"sick":{"start":["true"],"stop":["true"],"health_check":{"command":["false"],"timeout":"200ms","interval":"50ms"}}`)

	var code int
	errOut := captureStderr(t, func() {
		captureStdout(t, func() {
			code = run(restartArgs(path, "sick"))
		})
	})
	if code != exitLogic {
		t.Fatalf("unready service: exit %d, want %d", code, exitLogic)
	}
	if !strings.Contains(errOut, "not ready") {
		t.Errorf("stderr = %q, want a readiness failure distinct from the restart", errOut)
	}
	if strings.Contains(errOut, "failed to restart") {
		t.Errorf("stderr = %q: the restart itself succeeded and must not be reported as failed", errOut)
	}
}

func TestServiceRestartHealthyReadinessSucceeds(t *testing.T) {
	dir := t.TempDir()
	testutil.SetHome(t, dir)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, ".config"))
	path := writeRestartSpec(t,
		`"well":{"start":["true"],"stop":["true"],"health_check":{"command":["true"],"timeout":"2s","interval":"50ms"}}`)

	out := captureStdout(t, func() {
		if code := run(restartArgs(path, "well")); code != exitOK {
			t.Fatalf("healthy service: exit %d", code)
		}
	})
	if !strings.Contains(out, "ready") {
		t.Errorf("output = %q, want a readiness confirmation", out)
	}
}

// The restart must work against a materialized (v10) spec with modules, since
// that is how a composed service is reached.
func TestServiceRestartOnComposedSpec(t *testing.T) {
	dir := t.TempDir()
	testutil.SetHome(t, dir)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, ".config"))
	modules := filepath.Join(dir, "modules")
	if err := os.MkdirAll(modules, 0o755); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(dir, "calls.log")
	module := `{"schemaVersion":"10","defaults":{"services":{"worker":{"start":["sh","-c","echo start >> ` + logPath +
		`"],"stop":["sh","-c","echo stop >> ` + logPath + `"]}}}}`
	if err := os.WriteFile(filepath.Join(modules, "base.json"), []byte(module), 0o644); err != nil {
		t.Fatal(err)
	}
	spec := `{"schemaVersion":"10","modules":{"base":"modules/base.json"},"targets":{"macos":{"useModules":["base"]}}}`
	path := filepath.Join(dir, "genv.json")
	if err := os.WriteFile(path, []byte(spec), 0o644); err != nil {
		t.Fatal(err)
	}

	captureStdout(t, func() {
		if code := run([]string{"service", "restart", "worker", "--file", path, "--target", "macos"}); code != exitOK {
			t.Fatalf("composed restart: exit %d", code)
		}
	})
	data, err := os.ReadFile(logPath)
	if err != nil || string(data) != "stop\nstart\n" {
		t.Fatalf("composed restart = %q error=%v", data, err)
	}

	// A module-owned service is still readable for restart even though the
	// mutating commands refuse to touch it.
	if _, err := genvfile.Read(path); err != nil {
		t.Fatalf("spec unreadable after restart: %v", err)
	}
}
