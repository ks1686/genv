package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/ks1686/genv/internal/adapter"
	"github.com/ks1686/genv/internal/genvfile"
	"github.com/ks1686/genv/internal/schema"
)

// versionBumpAdapter reports a different installed version once its upgrade
// command has run, which is what a real manager does when it moves a package
// forward. The existing upgradeNoHooksAdapter always answers 2.0.0, which is
// right for its tests but cannot express "the version moved".
type versionBumpAdapter struct {
	upgraded bool
	marker   string
}

func (a *versionBumpAdapter) Name() string    { return "bump-manager" }
func (a *versionBumpAdapter) Available() bool { return true }
func (a *versionBumpAdapter) NormalizeID(id string, _ map[string]string) (string, bool) {
	return id, false
}
func (a *versionBumpAdapter) PlanInstall(pkgName string) []string {
	return shellAppendMarker("install", a.marker)
}
func (a *versionBumpAdapter) PlanUninstall(pkgName string) []string {
	return shellAppendMarker("uninstall", a.marker)
}
func (a *versionBumpAdapter) PlanUpgrade(pkgName string) []string {
	a.upgraded = true
	return shellAppendMarker("upgrade", a.marker)
}
func (a *versionBumpAdapter) PlanClean() [][]string            { return nil }
func (a *versionBumpAdapter) Query(string) (bool, error)       { return true, nil }
func (a *versionBumpAdapter) ListInstalled() ([]string, error) { return []string{"postgres"}, nil }
func (a *versionBumpAdapter) QueryVersion(string) (string, error) {
	if a.upgraded {
		return "2.0.0", nil
	}
	return "1.0.0", nil
}

// staticVersionAdapter never changes version, so an upgrade against it is a
// no-op.
type staticVersionAdapter struct {
	version string
	marker  string
}

func (a *staticVersionAdapter) Name() string    { return "static-manager" }
func (a *staticVersionAdapter) Available() bool { return true }
func (a *staticVersionAdapter) NormalizeID(id string, _ map[string]string) (string, bool) {
	return id, false
}
func (a *staticVersionAdapter) PlanInstall(pkgName string) []string {
	return shellAppendMarker("install", a.marker)
}
func (a *staticVersionAdapter) PlanUninstall(pkgName string) []string {
	return shellAppendMarker("uninstall", a.marker)
}
func (a *staticVersionAdapter) PlanUpgrade(pkgName string) []string {
	return shellAppendMarker("upgrade", a.marker)
}
func (a *staticVersionAdapter) PlanClean() [][]string            { return nil }
func (a *staticVersionAdapter) Query(string) (bool, error)       { return true, nil }
func (a *staticVersionAdapter) ListInstalled() ([]string, error) { return []string{"postgres"}, nil }
func (a *staticVersionAdapter) QueryVersion(string) (string, error) {
	return a.version, nil
}

func registerBumpAdapter(t *testing.T, a adapter.Adapter) {
	t.Helper()
	original := adapter.All
	adapter.All = append([]adapter.Adapter{a}, original...)
	originalKnown := schema.KnownManagers[a.Name()]
	schema.KnownManagers[a.Name()] = true
	t.Cleanup(func() {
		adapter.All = original
		if originalKnown {
			schema.KnownManagers[a.Name()] = true
		} else {
			delete(schema.KnownManagers, a.Name())
		}
	})
}

// watchedUpgradeSpec builds a v10 spec whose service watches one package.
// Command arrays are JSON-encoded so a Windows temp path cannot break the spec,
// and status goes through sh so the same argv works on the Windows runner.
func watchedUpgradeSpec(t *testing.T, manager, serviceMarker string) string {
	t.Helper()
	marker := serviceMarker
	if runtime.GOOS == "windows" {
		marker = filepath.ToSlash(serviceMarker)
	}
	start, err := json.Marshal([]string{"sh", "-c", "printf start, >> " + marker})
	if err != nil {
		t.Fatalf("marshal start: %v", err)
	}
	stop, err := json.Marshal([]string{"sh", "-c", "printf stop, >> " + marker})
	if err != nil {
		t.Fatalf("marshal stop: %v", err)
	}
	status, err := json.Marshal([]string{"sh", "-c", "exit 0"})
	if err != nil {
		t.Fatalf("marshal status: %v", err)
	}
	return fmt.Sprintf(`{
	  "schemaVersion": "10",
	  "targets": {
	    "arch": {
	      "packages": [{ "id": "postgres", "prefer": %q }],
	      "services": {
	        "api": {
	          "start": %s,
	          "stop": %s,
	          "status": %s,
	          "watch": ["postgres"],
	          "restart_policy": "ifRunning"
	        }
	      }
	    }
	  }
	}`, manager, start, stop, status)
}

// TestUpgrade_restarts_watched_service_end_to_end is the test that matters most
// in this feature: it runs the real `genv upgrade` and requires that the
// service is actually restarted.
//
// The unit tests around the coordinator all pass against a snapshot taken after
// the upgrade — at which point the lock has already been overwritten and every
// comparison reads "unchanged". Only a test that runs the command and watches
// the service's marker file catches that.
func TestUpgrade_restarts_watched_service_end_to_end(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "xdg"))
	specPath := filepath.Join(dir, "genv.json")
	lockPath := filepath.Join(dir, "genv.lock.json")
	serviceMarker := filepath.Join(dir, "service.log")

	bump := &versionBumpAdapter{marker: filepath.Join(dir, "manager.log")}
	registerBumpAdapter(t, bump)

	// The service reports itself running via its status command, and records
	// every lifecycle call so the test can prove a restart happened.
	// Commands are marshaled so a Windows path's backslashes stay valid JSON.
	writeTestFile(t, specPath, watchedUpgradeSpec(t, "bump-manager", serviceMarker))

	// The lock claims an older version, so a successful upgrade moves it.
	writeLockFile(t, lockPath, &genvfile.LockFile{
		SchemaVersion: "8",
		Target:        "arch",
		GOOS:          runtime.GOOS,
		Packages: []genvfile.LockedPackage{
			{ID: "postgres", Manager: "bump-manager", PkgName: "postgres", InstalledVersion: "1.0.0"},
		},
	})

	var code int
	out := captureStdout(t, func() {
		code = run([]string{"upgrade", "--file", specPath, "--lock-file", lockPath,
			"--target", "arch", "--yes", "--no-hooks", "--all"})
	})
	if code != exitOK {
		t.Fatalf("upgrade exit = %d, want %d\n%s", code, exitOK, out)
	}

	data, err := os.ReadFile(serviceMarker)
	if err != nil {
		t.Fatalf("service was never touched; output:\n%s", out)
	}
	var calls []string
	for _, c := range strings.Split(string(data), ",") {
		if c = strings.TrimSpace(c); c != "" {
			calls = append(calls, c)
		}
	}
	if strings.Join(calls, ",") != "stop,start" {
		t.Errorf("service lifecycle calls = %v, want stop then start (output:\n%s)", calls, out)
	}
	if !strings.Contains(out, "restarted") {
		t.Errorf("upgrade should report the restart:\n%s", out)
	}

	// A confirmed restart must leave nothing pending behind.
	lf, err := genvfile.ReadLock(lockPath)
	if err != nil {
		t.Fatalf("read lock: %v", err)
	}
	if len(lf.PendingActions) != 0 {
		t.Errorf("pending actions = %+v, want cleared", lf.PendingActions)
	}
}

// TestUpgrade_no_op_upgrade_does_not_restart_service is the other half: an
// upgrade that leaves the installed version alone must not restart anything.
func TestUpgrade_no_op_upgrade_does_not_restart_service(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "xdg"))
	specPath := filepath.Join(dir, "genv.json")
	lockPath := filepath.Join(dir, "genv.lock.json")
	serviceMarker := filepath.Join(dir, "service.log")

	// QueryVersion never changes, so the upgrade is a no-op.
	static := &staticVersionAdapter{version: "1.0.0", marker: filepath.Join(dir, "manager.log")}
	registerBumpAdapter(t, static)

	writeTestFile(t, specPath, watchedUpgradeSpec(t, "static-manager", serviceMarker))
	writeLockFile(t, lockPath, &genvfile.LockFile{
		SchemaVersion: "8",
		Target:        "arch",
		GOOS:          runtime.GOOS,
		Packages: []genvfile.LockedPackage{
			{ID: "postgres", Manager: "static-manager", PkgName: "postgres", InstalledVersion: "1.0.0"},
		},
	})

	out := captureStdout(t, func() {
		run([]string{"upgrade", "--file", specPath, "--lock-file", lockPath,
			"--target", "arch", "--yes", "--no-hooks", "--all"})
	})

	if _, err := os.Stat(serviceMarker); err == nil {
		data, _ := os.ReadFile(serviceMarker)
		t.Errorf("a no-op upgrade must not restart the service, calls = %q (output:\n%s)", data, out)
	}
}

// An output flag must not change machine state. `genv upgrade --json` used to
// return before the restart phase, so it upgraded the binary a service runs and
// left the service on the old code while reporting success.
func TestUpgrade_json_restarts_watched_service_end_to_end(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "xdg"))
	specPath := filepath.Join(dir, "genv.json")
	lockPath := filepath.Join(dir, "genv.lock.json")
	serviceMarker := filepath.Join(dir, "service.log")

	bump := &versionBumpAdapter{marker: filepath.Join(dir, "manager.log")}
	registerBumpAdapter(t, bump)

	writeTestFile(t, specPath, watchedUpgradeSpec(t, "bump-manager", serviceMarker))
	writeLockFile(t, lockPath, &genvfile.LockFile{
		SchemaVersion: "8",
		Target:        "arch",
		GOOS:          runtime.GOOS,
		Packages: []genvfile.LockedPackage{
			{ID: "postgres", Manager: "bump-manager", PkgName: "postgres", InstalledVersion: "1.0.0"},
		},
	})

	var code int
	out := captureStdout(t, func() {
		code = run([]string{"upgrade", "--file", specPath, "--lock-file", lockPath,
			"--target", "arch", "--yes", "--no-hooks", "--all", "--json"})
	})
	if code != exitOK {
		t.Fatalf("upgrade --json exit = %d, want %d\n%s", code, exitOK, out)
	}

	data, err := os.ReadFile(serviceMarker)
	if err != nil {
		t.Fatalf("service was never restarted under --json; the output flag changed machine state\n%s", out)
	}
	var calls []string
	for _, c := range strings.Split(string(data), ",") {
		if c = strings.TrimSpace(c); c != "" {
			calls = append(calls, c)
		}
	}
	if strings.Join(calls, ",") != "stop,start" {
		t.Errorf("service lifecycle calls = %v, want stop then start", calls)
	}

	// The envelope has to report it, not just do it.
	var env struct {
		Data struct {
			Services []struct {
				Service string `json:"service"`
				Action  string `json:"action"`
			} `json:"services"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("stdout is not one JSON object: %v\n%s", err, out)
	}
	if len(env.Data.Services) != 1 {
		t.Fatalf("services = %+v, want one restart reported\n%s", env.Data.Services, out)
	}
	if env.Data.Services[0].Action != "restarted" {
		t.Errorf("action = %q, want %q", env.Data.Services[0].Action, "restarted")
	}

	lf, err := genvfile.ReadLock(lockPath)
	if err != nil {
		t.Fatalf("read lock: %v", err)
	}
	if len(lf.PendingActions) != 0 {
		t.Errorf("pending actions = %+v, want cleared", lf.PendingActions)
	}
}

// A JSON dry run plans and does not act, exactly like the text path.
func TestUpgrade_json_dry_run_does_not_restart_service(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "xdg"))
	specPath := filepath.Join(dir, "genv.json")
	lockPath := filepath.Join(dir, "genv.lock.json")
	serviceMarker := filepath.Join(dir, "service.log")

	bump := &versionBumpAdapter{marker: filepath.Join(dir, "manager.log")}
	registerBumpAdapter(t, bump)

	writeTestFile(t, specPath, watchedUpgradeSpec(t, "bump-manager", serviceMarker))
	writeLockFile(t, lockPath, &genvfile.LockFile{
		SchemaVersion: "8",
		Target:        "arch",
		GOOS:          runtime.GOOS,
		Packages: []genvfile.LockedPackage{
			{ID: "postgres", Manager: "bump-manager", PkgName: "postgres", InstalledVersion: "1.0.0"},
		},
	})

	var code int
	out := captureStdout(t, func() {
		code = run([]string{"upgrade", "--file", specPath, "--lock-file", lockPath,
			"--target", "arch", "--no-hooks", "--all", "--json", "--dry-run"})
	})
	if code != exitOK {
		t.Fatalf("upgrade --json --dry-run exit = %d, want %d\n%s", code, exitOK, out)
	}
	if _, err := os.Stat(serviceMarker); err == nil {
		t.Errorf("dry run touched the service; a plan that runs commands is not a plan\n%s", out)
	}
	if strings.Contains(out, `"action"`) {
		t.Errorf("dry run reported a restart action:\n%s", out)
	}
}
