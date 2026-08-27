package service

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/ks1686/genv/internal/genvfile"
	"github.com/ks1686/genv/internal/schema"
	"github.com/ks1686/genv/internal/testutil"
)

func installServiceFakeBinary(t *testing.T, name, script string) {
	t.Helper()
	testutil.InstallFakeBinary(t, name, script)
}

func TestServiceSupervisorHelpers(t *testing.T) {
	home := t.TempDir()
	testutil.SetHome(t, home)
	installServiceFakeBinary(t, "systemctl", "exit 0")
	installServiceFakeBinary(t, "launchctl", "exit 0")
	ctx := context.Background()
	svc := schema.Service{Start: []string{"true"}, Stop: []string{"true"}}

	if err := applySystemd(ctx, "systemd-test", svc, true); err != nil {
		t.Fatalf("applySystemd: %v", err)
	}
	systemdPath := filepath.Join(home, ".config/systemd/user", systemdUnitName("systemd-test"))
	if content, err := os.ReadFile(systemdPath); err != nil || !strings.Contains(string(content), `ExecStart="true"`) {
		t.Fatalf("systemd unit = %q, read error = %v", content, err)
	}
	if err := removeSystemd(ctx, "systemd-test", true); err != nil {
		t.Fatalf("removeSystemd: %v", err)
	}
	if _, err := os.Stat(systemdPath); !os.IsNotExist(err) {
		t.Fatalf("systemd unit still exists or stat failed: %v", err)
	}

	if err := applyLaunchd(ctx, "launchd-test", svc, true); err != nil {
		t.Fatalf("applyLaunchd: %v", err)
	}
	launchdPath := filepath.Join(home, "Library/LaunchAgents", launchdPlistName("launchd-test"))
	if content, err := os.ReadFile(launchdPath); err != nil || !strings.Contains(string(content), "<key>ProgramArguments</key>") {
		t.Fatalf("launchd plist = %q, read error = %v", content, err)
	}
	if err := removeLaunchd(ctx, "launchd-test", true); err != nil {
		t.Fatalf("removeLaunchd: %v", err)
	}
	if _, err := os.Stat(launchdPath); !os.IsNotExist(err) {
		t.Fatalf("launchd plist still exists or stat failed: %v", err)
	}
}

func TestScheduledSupervisorHelpersAndHostBackend(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("host backend uses schtasks on Windows; see TestSchtasksScheduledJob_registerStatusStop")
	}
	home := t.TempDir()
	testutil.SetHome(t, home)
	installServiceFakeBinary(t, "systemctl", "exit 0")
	installServiceFakeBinary(t, "launchctl", "exit 0")
	ctx := context.Background()
	job := ScheduledJob{
		Name:        "scheduled-test",
		Command:     []string{"true"},
		Interval:    time.Minute,
		Environment: map[string]string{"PATH": "/custom/bin"},
	}

	if err := startSystemdScheduledJob(ctx, job); err != nil {
		t.Fatalf("startSystemdScheduledJob: %v", err)
	}
	if err := stopSystemdScheduledJob(ctx, job.Name); err != nil {
		t.Fatalf("stopSystemdScheduledJob: %v", err)
	}
	if err := startLaunchdScheduledJob(ctx, job); err != nil {
		t.Fatalf("startLaunchdScheduledJob: %v", err)
	}
	if err := stopLaunchdScheduledJob(ctx, job.Name); err != nil {
		t.Fatalf("stopLaunchdScheduledJob: %v", err)
	}

	backend := NewScheduledBackend()
	if !backend.Supported() {
		t.Fatal("NewScheduledBackend() should be supported with a fake platform supervisor")
	}
	if err := backend.Start(ctx, job); err != nil {
		t.Fatalf("host scheduled Start: %v", err)
	}
	if err := backend.Stop(ctx, job.Name); err != nil {
		t.Fatalf("host scheduled Stop: %v", err)
	}

	if runtime.GOOS == "darwin" {
		path := filepath.Join(home, "Library/LaunchAgents", launchdPlistName(job.Name))
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("host backend left launchd plist behind: %v", err)
		}
	} else {
		dir := filepath.Join(home, ".config/systemd/user")
		for _, name := range []string{systemdScheduledUnitName(job.Name), systemdTimerName(job.Name)} {
			if _, err := os.Stat(filepath.Join(dir, name)); !os.IsNotExist(err) {
				t.Fatalf("host backend left systemd file %s behind: %v", name, err)
			}
		}
	}

	if _, err := backend.Status(ctx, job.Name); err != nil {
		t.Fatalf("Status after stop: %v", err)
	}
}

func TestApplyServicesUsesAvailableSupervisor(t *testing.T) {
	home := t.TempDir()
	testutil.SetHome(t, home)
	installServiceFakeBinary(t, "systemctl", "exit 0")
	installServiceFakeBinary(t, "launchctl", "exit 0")

	spec := map[string]schema.Service{"managed": {Start: []string{"true"}}}
	applied, removed, errs := ApplyServices(context.Background(), spec, nil, true)
	if len(errs) != 0 || len(applied) != 1 || applied[0] != "managed" || len(removed) != 0 {
		t.Fatalf("apply = %v, remove = %v, errors = %v", applied, removed, errs)
	}

	applied, removed, errs = ApplyServices(context.Background(), nil, []genvfile.LockedService{{Name: "managed"}}, true)
	if len(errs) != 0 || len(applied) != 0 || len(removed) != 1 || removed[0] != "managed" {
		t.Fatalf("apply = %v, remove = %v, errors = %v", applied, removed, errs)
	}
}

func TestBrewServiceCommandsWithFakeBinary(t *testing.T) {
	installServiceFakeBinary(t, "brew", `
case "$2" in
list) printf 'demo started user\nother stopped user\n' ;;
start|stop|restart)
	if [ "$3" = "fail" ]; then
		printf 'fake brew failure\n'
		exit 1
	fi
	;;
esac`)

	ctx := context.Background()
	for name, command := range map[string]func(context.Context, string) error{
		"start":   BrewServicesStart,
		"stop":    BrewServicesStop,
		"restart": BrewServicesRestart,
	} {
		t.Run(name, func(t *testing.T) {
			if err := command(ctx, "demo"); err != nil {
				t.Fatalf("%s demo: %v", name, err)
			}
			if err := command(ctx, "fail"); err == nil || !strings.Contains(err.Error(), "fake brew failure") {
				t.Fatalf("%s fail error = %v, want fake brew failure", name, err)
			}
		})
	}

	if !BrewServicesRunning("demo") {
		t.Error("BrewServicesRunning(demo) = false, want true")
	}
	if BrewServicesRunning("other") {
		t.Error("BrewServicesRunning(other) = true, want false")
	}
	if output, err := BrewServicesList(); err != nil || !strings.Contains(output, "demo started") {
		t.Fatalf("BrewServicesList() = %q, %v", output, err)
	}
}
