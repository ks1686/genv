package service

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ks1686/genv/internal/genvfile"
	"github.com/ks1686/genv/internal/schema"
	"github.com/ks1686/genv/internal/testutil"
)

func samplePlist(label, program string) string {
	return `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>` + label + `</string>
    <key>ProgramArguments</key>
    <array>
        <string>` + program + `</string>
    </array>
    <key>WorkingDirectory</key>
    <string>__HOME__</string>
</dict>
</plist>
`
}

func sampleUnit(desc, exec string) string {
	return "[Unit]\nDescription=" + desc + "\n\n[Service]\nExecStart=" + exec + "\n\n[Install]\nWantedBy=default.target\n"
}

func installSupervisorFakes(t *testing.T, home string) string {
	t.Helper()
	logPath := filepath.Join(home, "supervisor.log")
	loaded := filepath.Join(home, "launchd-loaded")
	installServiceFakeBinary(t, "launchctl", `
log="`+logPath+`"
echo "launchctl $*" >> "$log"
if [ "$1" = print ]; then
  if [ -f "`+loaded+`" ]; then
    printf 'state = running\n'
    exit 0
  fi
  echo "Could not find service" >&2
  exit 1
fi
if [ "$1" = bootstrap ]; then
  touch "`+loaded+`"
fi
if [ "$1" = bootout ]; then
  rm -f "`+loaded+`"
fi
exit 0
`)
	installServiceFakeBinary(t, "systemctl", `
log="`+logPath+`"
echo "systemctl $*" >> "$log"
if [ "$1" = --user ] && [ "$2" = is-active ]; then
  if [ -f "`+filepath.Join(home, "systemd-active")+`" ]; then
    exit 0
  fi
  exit 1
fi
if [ "$1" = --user ] && [ "$2" = enable ]; then
  touch "`+filepath.Join(home, "systemd-active")+`"
fi
if [ "$1" = --user ] && [ "$2" = stop ]; then
  rm -f "`+filepath.Join(home, "systemd-active")+`"
fi
exit 0
`)
	return logPath
}

func supervisorLog(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return ""
		}
		t.Fatalf("read supervisor log: %v", err)
	}
	return string(data)
}

func TestParseLaunchdLabel(t *testing.T) {
	got, err := ParseLaunchdLabel(samplePlist("com.example.agent", "/bin/true"))
	if err != nil {
		t.Fatalf("ParseLaunchdLabel: %v", err)
	}
	if got != "com.example.agent" {
		t.Fatalf("label = %q", got)
	}
	if _, err := ParseLaunchdLabel("<plist></plist>"); err == nil {
		t.Fatal("expected missing Label to fail")
	}
	if _, err := ParseLaunchdLabel(samplePlist("evil/label", "/bin/true")); err == nil {
		t.Fatal("expected illegal Label to fail")
	}
}

func TestApplyServices_LaunchdTemplateBootstrapAndRerender(t *testing.T) {
	home := t.TempDir()
	testutil.SetHome(t, home)
	root := t.TempDir()
	src := filepath.Join(root, "agents", "com.example.agent.plist")
	if err := os.MkdirAll(filepath.Dir(src), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(src, []byte(samplePlist("com.example.agent", "/bin/true")), 0o644); err != nil {
		t.Fatal(err)
	}
	logPath := installSupervisorFakes(t, home)
	t.Cleanup(func() {
		probeLaunchd = IsLaunchdAvailable
		probeSystemd = IsSystemdAvailable
	})
	probeLaunchd = func() bool { return true }
	probeSystemd = func() bool { return false }

	spec := map[string]schema.Service{
		"agent": {Launchd: &schema.LaunchdSpec{Plist: "agents/com.example.agent.plist"}},
	}
	applied, removed, errs := ApplyServices(context.Background(), spec, nil, true, root)
	if len(errs) != 0 || len(removed) != 0 || len(applied) != 1 || applied[0] != "agent" {
		t.Fatalf("first apply = %v remove = %v errs = %v", applied, removed, errs)
	}
	dest := filepath.Join(home, "Library", "LaunchAgents", "com.example.agent.plist")
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("read dest: %v", err)
	}
	if !strings.Contains(string(got), home) || strings.Contains(string(got), "__HOME__") {
		t.Fatalf("rendered plist should expand __HOME__, got:\n%s", got)
	}
	log := supervisorLog(t, logPath)
	if !strings.Contains(log, "launchctl bootstrap gui/") || !strings.Contains(log, dest) {
		t.Fatalf("expected bootstrap of %s, log:\n%s", dest, log)
	}
	if strings.Contains(log, "bootout") {
		t.Fatalf("first apply should not bootout, log:\n%s", log)
	}

	if err := os.WriteFile(logPath, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	applied, _, errs = ApplyServices(context.Background(), spec, SpecToLock(spec, root), false, root)
	if len(errs) != 0 {
		t.Fatalf("noop apply: %v", errs)
	}
	if len(applied) != 0 {
		t.Fatalf("unchanged loaded job should not re-apply, got %v", applied)
	}
	log = supervisorLog(t, logPath)
	if strings.Contains(log, "bootstrap") || strings.Contains(log, "bootout") {
		t.Fatalf("unchanged apply should not re-bootstrap, log:\n%s", log)
	}

	if err := os.WriteFile(src, []byte(samplePlist("com.example.agent", "/bin/false")), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(logPath, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	applied, _, errs = ApplyServices(context.Background(), spec, SpecToLock(spec, root), false, root)
	if len(errs) != 0 || len(applied) != 1 {
		t.Fatalf("rerender apply = %v errs = %v", applied, errs)
	}
	log = supervisorLog(t, logPath)
	if !strings.Contains(log, "bootout gui/") || !strings.Contains(log, "com.example.agent") {
		t.Fatalf("content change should bootout, log:\n%s", log)
	}
	if !strings.Contains(log, "bootstrap") {
		t.Fatalf("content change should bootstrap, log:\n%s", log)
	}
	got, err = os.ReadFile(dest)
	if err != nil || !strings.Contains(string(got), "/bin/false") {
		t.Fatalf("dest after rerender = %q err=%v", got, err)
	}

	if !ProbeRunning(context.Background(), "agent", spec["agent"], root) {
		t.Fatal("ProbeRunning should use launchctl print state=running")
	}

	if err := os.WriteFile(logPath, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	_, removed, errs = ApplyServices(context.Background(), nil, SpecToLock(spec, root), true, root)
	if len(errs) != 0 || len(removed) != 1 || removed[0] != "agent" {
		t.Fatalf("remove = %v errs = %v", removed, errs)
	}
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Fatalf("plist should be deleted: %v", err)
	}
	log = supervisorLog(t, logPath)
	if !strings.Contains(log, "bootout") {
		t.Fatalf("remove should bootout, log:\n%s", log)
	}
}

func TestApplyServices_SystemdTemplateEnableAndRerender(t *testing.T) {
	home := t.TempDir()
	testutil.SetHome(t, home)
	root := t.TempDir()
	src := filepath.Join(root, "units", "syncthing.service")
	if err := os.MkdirAll(filepath.Dir(src), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(src, []byte(sampleUnit("sync", "/usr/bin/syncthing")), 0o644); err != nil {
		t.Fatal(err)
	}
	logPath := installSupervisorFakes(t, home)
	t.Cleanup(func() {
		probeLaunchd = IsLaunchdAvailable
		probeSystemd = IsSystemdAvailable
	})
	probeLaunchd = func() bool { return false }
	probeSystemd = func() bool { return true }

	spec := map[string]schema.Service{
		"sync": {Systemd: &schema.SystemdSpec{Unit: "units/syncthing.service"}},
	}
	applied, _, errs := ApplyServices(context.Background(), spec, nil, true, root)
	if len(errs) != 0 || len(applied) != 1 {
		t.Fatalf("first apply = %v errs = %v", applied, errs)
	}
	dest := filepath.Join(home, ".config", "systemd", "user", "syncthing.service")
	got, err := os.ReadFile(dest)
	if err != nil || !strings.Contains(string(got), "/usr/bin/syncthing") {
		t.Fatalf("unit = %q err=%v", got, err)
	}
	log := supervisorLog(t, logPath)
	if !strings.Contains(log, "systemctl --user enable --now syncthing.service") {
		t.Fatalf("expected enable --now, log:\n%s", log)
	}

	if err := os.WriteFile(src, []byte(sampleUnit("sync", "/usr/bin/syncthing --home __HOME__")), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(logPath, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	applied, _, errs = ApplyServices(context.Background(), spec, SpecToLock(spec, root), false, root)
	if len(errs) != 0 || len(applied) != 1 {
		t.Fatalf("rerender apply = %v errs = %v", applied, errs)
	}
	got, err = os.ReadFile(dest)
	if err != nil || !strings.Contains(string(got), home) || strings.Contains(string(got), "__HOME__") {
		t.Fatalf("rendered unit should expand __HOME__, got:\n%s err=%v", got, err)
	}
	log = supervisorLog(t, logPath)
	if !strings.Contains(log, "restart syncthing.service") {
		t.Fatalf("content change should restart, log:\n%s", log)
	}

	if !ProbeRunning(context.Background(), "sync", spec["sync"], root) {
		t.Fatal("ProbeRunning should see systemd is-active")
	}

	_, removed, errs := ApplyServices(context.Background(), nil, []genvfile.LockedService{{
		Name:        "sync",
		SystemdUnit: "units/syncthing.service",
		SystemdName: "syncthing",
	}}, true, root)
	if len(errs) != 0 || len(removed) != 1 {
		t.Fatalf("remove = %v errs = %v", removed, errs)
	}
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Fatalf("unit should be deleted: %v", err)
	}
}

func TestApplyServices_SupervisorTemplateSkippedWhenUnavailable(t *testing.T) {
	t.Cleanup(func() {
		probeLaunchd = IsLaunchdAvailable
		probeSystemd = IsSystemdAvailable
	})
	probeLaunchd = func() bool { return false }
	probeSystemd = func() bool { return false }
	spec := map[string]schema.Service{
		"agent": {Launchd: &schema.LaunchdSpec{Plist: "missing.plist"}},
	}
	applied, removed, errs := ApplyServices(context.Background(), spec, nil, false, t.TempDir())
	if len(errs) != 0 || len(removed) != 0 || len(applied) != 0 {
		t.Fatalf("skip apply = %v remove = %v errs = %v", applied, removed, errs)
	}
}

func TestSpecToLock_SupervisorFields(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "com.example.agent.plist")
	if err := os.WriteFile(src, []byte(samplePlist("com.example.agent", "/bin/true")), 0o644); err != nil {
		t.Fatal(err)
	}
	lock := SpecToLock(map[string]schema.Service{
		"agent": {
			Launchd: &schema.LaunchdSpec{Plist: "com.example.agent.plist"},
			Systemd: &schema.SystemdSpec{Unit: "units/foo.service"},
		},
	}, root)
	if len(lock) != 1 {
		t.Fatalf("lock len = %d", len(lock))
	}
	if lock[0].LaunchdPlist != "com.example.agent.plist" || lock[0].LaunchdLabel != "com.example.agent" {
		t.Fatalf("launchd lock = %+v", lock[0])
	}
	if lock[0].SystemdUnit != "units/foo.service" || lock[0].SystemdName != "foo" {
		t.Fatalf("systemd lock = %+v", lock[0])
	}
}
