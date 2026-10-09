package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ks1686/genv/internal/schema"
	"github.com/ks1686/genv/internal/testutil"
)

// fakeSupervisor records the launchctl/systemctl argv a restart would issue.
// The real binaries are never invoked: a test that bootstrapped a live launchd
// job wrote to the developer's ~/Library/LaunchAgents once already.
type fakeSupervisor struct {
	calls  [][]string
	fail   map[string]error
	output map[string]string
}

func (f *fakeSupervisor) launchctl(_ context.Context, args ...string) error {
	f.calls = append(f.calls, append([]string{"launchctl"}, args...))
	return f.fail[strings.Join(args, " ")]
}

func (f *fakeSupervisor) launchctlPrint(_ context.Context, args ...string) ([]byte, error) {
	f.calls = append(f.calls, append([]string{"launchctl"}, args...))
	key := strings.Join(args, " ")
	if body, ok := f.output[key]; ok {
		return []byte(body), nil
	}
	return nil, errors.New("not found")
}

func (f *fakeSupervisor) systemctl(_ context.Context, args ...string) error {
	f.calls = append(f.calls, append([]string{"systemctl"}, args...))
	return f.fail[strings.Join(args, " ")]
}

func (f *fakeSupervisor) verbs() []string {
	var out []string
	for _, call := range f.calls {
		if len(call) > 1 {
			out = append(out, call[1])
		}
	}
	return out
}

func withFakeSupervisor(t *testing.T) *fakeSupervisor {
	t.Helper()
	fake := &fakeSupervisor{fail: map[string]error{}, output: map[string]string{}}
	oldLaunchctl, oldSystemctl := launchctlRun, systemctlRun
	oldOutput, oldProbeLaunchd, oldProbeSystemd := launchctlOutput, probeLaunchd, probeSystemd
	launchctlRun = fake.launchctl
	systemctlRun = fake.systemctl
	launchctlOutput = fake.launchctlPrint
	probeLaunchd = func() bool { return true }
	probeSystemd = func() bool { return true }
	t.Cleanup(func() {
		launchctlRun, systemctlRun = oldLaunchctl, oldSystemctl
		launchctlOutput = oldOutput
		probeLaunchd, probeSystemd = oldProbeLaunchd, oldProbeSystemd
	})
	return fake
}

// writePlistFixture writes the *installed* LaunchAgent location, which is
// what `genv apply` would have produced, not the template source.
// writeSystemdUnit stages the installed --user unit location.
func writeSystemdUnit(t *testing.T, home, unit string) error {
	t.Helper()
	dir := filepath.Join(home, ".config", "systemd", "user")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, unit), []byte("[Service]\nExecStart=/usr/bin/true\n"), 0o644)
}

func writePlistFixture(t *testing.T, home, label string) string {
	t.Helper()
	dir := filepath.Join(home, "Library", "LaunchAgents")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, label+".plist")
	body := `<?xml version="1.0" encoding="UTF-8"?>
<plist version="1.0"><dict>
  <key>Label</key><string>` + label + `</string>
  <key>ProgramArguments</key><array><string>/usr/bin/true</string></array>
  <key>RunAtLoad</key><true/>
</dict></plist>`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestRestartDeclaredBrewUsesBrewServicesRestart(t *testing.T) {
	var got string
	old := brewServicesRestart
	brewServicesRestart = func(ctx context.Context, formula string) error {
		got = formula
		return nil
	}
	t.Cleanup(func() { brewServicesRestart = old })

	if err := RestartDeclared(context.Background(), "db", schema.Service{BrewFormula: "postgresql@16"}, ""); err != nil {
		t.Fatalf("RestartDeclared: %v", err)
	}
	if got != "postgresql@16" {
		t.Fatalf("brew restart formula = %q, want postgresql@16", got)
	}
}

func TestRestartDeclaredBrewReportsFailure(t *testing.T) {
	old := brewServicesRestart
	brewServicesRestart = func(context.Context, string) error { return errors.New("brew failed") }
	t.Cleanup(func() { brewServicesRestart = old })

	err := RestartDeclared(context.Background(), "db", schema.Service{BrewFormula: "postgresql@16"}, "")
	if err == nil || !strings.Contains(err.Error(), "restart") {
		t.Fatalf("RestartDeclared error = %v, want a restart failure", err)
	}
}

func TestRestartDeclaredSystemdRestartsUnit(t *testing.T) {
	fake := withFakeSupervisor(t)
	home := t.TempDir()
	testutil.SetHome(t, home)
	if err := writeSystemdUnit(t, home, "api.service"); err != nil {
		t.Fatal(err)
	}

	svc := schema.Service{Systemd: &schema.SystemdSpec{Unit: filepath.Join(home, "api.service")}}
	if err := RestartDeclared(context.Background(), "api", svc, home); err != nil {
		t.Fatalf("RestartDeclared: %v", err)
	}
	want := []string{"restart", "api.service"}
	var got []string
	for _, call := range fake.calls {
		if len(call) > 2 && call[0] == "systemctl" && call[1] == "--user" {
			got = call[2:]
		}
	}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("systemctl argv = %v, want --user restart api.service (calls %v)", got, fake.calls)
	}
}

func TestRestartDeclaredSystemdPropagatesFailure(t *testing.T) {
	fake := withFakeSupervisor(t)
	fake.fail["--user restart api.service"] = errors.New("unit failed")
	home := t.TempDir()
	testutil.SetHome(t, home)
	if err := writeSystemdUnit(t, home, "api.service"); err != nil {
		t.Fatal(err)
	}

	svc := schema.Service{Systemd: &schema.SystemdSpec{Unit: filepath.Join(home, "api.service")}}
	err := RestartDeclared(context.Background(), "api", svc, home)
	if err == nil || !strings.Contains(err.Error(), "api.service") {
		t.Fatalf("RestartDeclared error = %v, want the unit named", err)
	}
}

func TestRestartDeclaredLaunchdBootsOutThenBootstraps(t *testing.T) {
	fake := withFakeSupervisor(t)
	home := t.TempDir()
	testutil.SetHome(t, home)
	plist := writePlistFixture(t, home, "com.example.api")
	if plist == "" {
		t.Fatal("no plist staged")
	}
	fake.output["print "+launchdPrintTarget("com.example.api")] = "state = running\n"

	svc := schema.Service{Launchd: &schema.LaunchdSpec{Plist: plist}}
	if err := RestartDeclared(context.Background(), "api", svc, home); err != nil {
		t.Fatalf("RestartDeclared: %v", err)
	}
	verbs := fake.verbs()
	if len(verbs) != 3 || verbs[0] != "print" || verbs[1] != "bootout" || verbs[2] != "bootstrap" {
		t.Fatalf("launchctl verbs = %v, want print, bootout, bootstrap", verbs)
	}
}

func TestRestartDeclaredRawCommandRunsOnce(t *testing.T) {
	if os.PathSeparator == '\\' {
		t.Skip("raw command fixture uses touch")
	}
	// A declared restart command is a single action, not stop-then-start.
	dir := t.TempDir()
	marker := filepath.Join(dir, "restarted")
	start := filepath.Join(dir, "started")
	stop := filepath.Join(dir, "stopped")

	svc := schema.Service{
		Restart: []string{"touch", marker},
		Start:   []string{"touch", start},
		Stop:    []string{"touch", stop},
	}
	if err := RestartDeclared(context.Background(), "worker", svc, ""); err != nil {
		t.Fatalf("RestartDeclared: %v", err)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("declared restart command did not run: %v", err)
	}
	for _, unexpected := range []string{start, stop} {
		if _, err := os.Stat(unexpected); err == nil {
			t.Fatalf("%s ran; a declared restart command must not stop/start", unexpected)
		}
	}
}

func TestRestartDeclaredRawFallsBackToStopThenStart(t *testing.T) {
	if os.PathSeparator == '\\' {
		t.Skip("raw command fixture uses a POSIX shell")
	}
	dir := t.TempDir()
	order := filepath.Join(dir, "order")
	svc := schema.Service{
		Start: []string{"sh", "-c", "echo start >> " + order},
		Stop:  []string{"sh", "-c", "echo stop >> " + order},
	}
	if err := RestartDeclared(context.Background(), "worker", svc, ""); err != nil {
		t.Fatalf("RestartDeclared: %v", err)
	}
	data, err := os.ReadFile(order)
	if err != nil {
		t.Fatalf("no stop/start recorded: %v", err)
	}
	if string(data) != "stop\nstart\n" {
		t.Fatalf("order = %q, want stop then start", data)
	}
}

func TestRestartDeclaredRefusesWithoutStopCommand(t *testing.T) {
	if os.PathSeparator == '\\' {
		t.Skip("raw command fixture uses POSIX true")
	}
	svc := schema.Service{Start: []string{"true"}}
	err := RestartDeclared(context.Background(), "worker", svc, "")
	if err == nil {
		t.Fatal("RestartDeclared restarted a service with no stop command")
	}
	if !strings.Contains(err.Error(), "no stop command") {
		t.Fatalf("error = %v, want it to name the missing stop command", err)
	}
}

func TestRestartDeclaredRefusesScheduledTask(t *testing.T) {
	svc := schema.Service{ScheduledTask: &schema.ScheduledTaskSpec{Action: "genv.exe"}}
	err := RestartDeclared(context.Background(), "job", svc, "")
	if err == nil || !strings.Contains(err.Error(), "scheduled_task") {
		t.Fatalf("error = %v, want a refusal naming scheduled_task", err)
	}
}

func TestRestartDeclaredReportsMissingRawCommandFailure(t *testing.T) {
	if os.PathSeparator == '\\' {
		t.Skip("raw command fixture uses POSIX utilities")
	}
	svc := schema.Service{Stop: []string{"false"}, Start: []string{"true"}}
	err := RestartDeclared(context.Background(), "worker", svc, "")
	if err == nil || !strings.Contains(err.Error(), "stop") {
		t.Fatalf("error = %v, want the failing stop reported", err)
	}
	// A failed stop must not be followed by a start: that is half a restart.
	marker := filepath.Join(t.TempDir(), "started")
	svc = schema.Service{Stop: []string{"false"}, Start: []string{"touch", marker}}
	if err := RestartDeclared(context.Background(), "worker", svc, ""); err == nil {
		t.Fatal("RestartDeclared reported success after a failed stop")
	}
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("start ran after a failed stop")
	}
}

func TestRestartDeclaredPrefersBrewOverOtherDeclarations(t *testing.T) {
	// A service can declare both a brew formula and raw commands; brew wins,
	// matching service start/stop.
	dir := t.TempDir()
	marker := filepath.Join(dir, "restart-ran")
	old := brewServicesRestart
	brewServicesRestart = func(context.Context, string) error { return nil }
	t.Cleanup(func() { brewServicesRestart = old })

	svc := schema.Service{
		BrewFormula: "redis",
		Restart:     []string{"touch", marker},
		Start:       []string{"touch", filepath.Join(dir, "start-ran")},
	}
	if err := RestartDeclared(context.Background(), "cache", svc, ""); err != nil {
		t.Fatalf("RestartDeclared: %v", err)
	}
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("raw restart ran for a brew-managed service")
	}
}

func TestRestartDeclaredUnavailableSupervisorRefused(t *testing.T) {
	oldLaunchd, oldSystemd := probeLaunchd, probeSystemd
	probeLaunchd = func() bool { return false }
	probeSystemd = func() bool { return false }
	t.Cleanup(func() { probeLaunchd, probeSystemd = oldLaunchd, oldSystemd })

	home := t.TempDir()
	plist := writePlistFixture(t, home, "com.example.api")
	err := RestartDeclared(context.Background(), "api", schema.Service{
		Launchd: &schema.LaunchdSpec{Plist: plist},
	}, home)
	if err == nil || !strings.Contains(err.Error(), "launchd") {
		t.Fatalf("error = %v, want a refusal naming launchd", err)
	}
}
