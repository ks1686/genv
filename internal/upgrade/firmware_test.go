package upgrade

import (
	"os/exec"
	"strings"
	"testing"
)

func TestPlanFirmware_linux_fwupd_or_skip_with_reason(t *testing.T) {
	present := func(names ...string) func(string) (string, error) {
		set := map[string]string{}
		for _, n := range names {
			set[n] = "/usr/bin/" + n
		}
		return func(file string) (string, error) {
			if p, ok := set[file]; ok {
				return p, nil
			}
			return "", exec.ErrNotFound
		}
	}

	t.Run("linux fwupdmgr", func(t *testing.T) {
		plan := PlanFirmware(Env{Target: "arch", GOOS: "linux", LookPath: present("fwupdmgr")})
		want := [][]string{{"sudo", "fwupdmgr", "update"}}
		if !commandPlansEqual(plan.Commands, want) {
			t.Fatalf("commands = %v, want %v", plan.Commands, want)
		}
	})

	t.Run("linux missing fwupdmgr", func(t *testing.T) {
		plan := PlanFirmware(Env{Target: "ubuntu", GOOS: "linux", LookPath: present()})
		if len(plan.Commands) != 0 || !strings.Contains(plan.SkipReason, "fwupdmgr") {
			t.Fatalf("plan = %#v, want fwupdmgr skip", plan)
		}
	})

	t.Run("macos firmware is softwareupdate", func(t *testing.T) {
		plan := PlanFirmware(Env{Target: "macos", GOOS: "darwin", LookPath: present("fwupdmgr", "softwareupdate")})
		if len(plan.Commands) != 0 || !strings.Contains(plan.SkipReason, "softwareupdate") {
			t.Fatalf("plan = %#v, want skip pointing at system softwareupdate", plan)
		}
	})

	t.Run("windows firmware is vendor-specific", func(t *testing.T) {
		plan := PlanFirmware(Env{Target: "windows", GOOS: "windows", LookPath: present("fwupdmgr")})
		if len(plan.Commands) != 0 || plan.SkipReason == "" {
			t.Fatalf("plan = %#v, want skip with reason", plan)
		}
	})
}
