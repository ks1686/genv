package upgrade

import (
	"os/exec"
	"strings"
	"testing"
)

func TestPlanSystem_selects_vendor_commands_without_executing(t *testing.T) {
	present := func(names ...string) func(string) (string, error) {
		set := map[string]string{}
		for _, n := range names {
			set[n] = "/bin/" + n
		}
		return func(file string) (string, error) {
			if p, ok := set[file]; ok {
				return p, nil
			}
			return "", exec.ErrNotFound
		}
	}

	t.Run("macos softwareupdate", func(t *testing.T) {
		plan := PlanSystem(Env{Target: "macos", GOOS: "darwin", LookPath: present("softwareupdate")})
		want := [][]string{{"sudo", "softwareupdate", "-i", "-a"}}
		if !commandPlansEqual(plan.Commands, want) {
			t.Fatalf("commands = %v, want %v", plan.Commands, want)
		}
		if plan.SkipReason != "" {
			t.Fatalf("SkipReason = %q, want empty", plan.SkipReason)
		}
	})

	t.Run("macos missing softwareupdate", func(t *testing.T) {
		plan := PlanSystem(Env{Target: "macos", GOOS: "darwin", LookPath: present()})
		if len(plan.Commands) != 0 {
			t.Fatalf("commands = %v, want none", plan.Commands)
		}
		if !strings.Contains(plan.SkipReason, "softwareupdate") {
			t.Fatalf("SkipReason = %q, want softwareupdate absence", plan.SkipReason)
		}
	})

	t.Run("windows uses Windows Update Agent COM via pwsh", func(t *testing.T) {
		plan := PlanSystem(Env{Target: "windows", GOOS: "windows", LookPath: present("pwsh")})
		if len(plan.Commands) != 1 {
			t.Fatalf("commands = %v, want one pwsh invocation", plan.Commands)
		}
		cmd := plan.Commands[0]
		if cmd[0] != "/bin/pwsh" || cmd[1] != "-NoProfile" || cmd[2] != "-Command" {
			t.Fatalf("argv prefix = %v, want pwsh -NoProfile -Command", cmd)
		}
		if !strings.Contains(cmd[3], "Microsoft.Update.Session") {
			t.Fatalf("command script %q, want Windows Update Agent COM", cmd[3])
		}
	})

	t.Run("windows falls back to powershell.exe", func(t *testing.T) {
		plan := PlanSystem(Env{Target: "windows", GOOS: "windows", LookPath: present("powershell.exe")})
		if len(plan.Commands) != 1 || plan.Commands[0][0] != "/bin/powershell.exe" {
			t.Fatalf("commands = %v, want powershell.exe", plan.Commands)
		}
	})

	t.Run("windows missing PowerShell", func(t *testing.T) {
		plan := PlanSystem(Env{Target: "windows", GOOS: "windows", LookPath: present()})
		if len(plan.Commands) != 0 || !strings.Contains(plan.SkipReason, "PowerShell") {
			t.Fatalf("plan = %#v, want PowerShell skip", plan)
		}
	})

	t.Run("arch pacman -Syu", func(t *testing.T) {
		plan := PlanSystem(Env{Target: "arch", GOOS: "linux", LookPath: present("pacman")})
		want := [][]string{{"sudo", "pacman", "-Syu", "--noconfirm"}}
		if !commandPlansEqual(plan.Commands, want) {
			t.Fatalf("commands = %v, want %v", plan.Commands, want)
		}
	})

	t.Run("wsl-arch uses pacman not paru", func(t *testing.T) {
		plan := PlanSystem(Env{Target: "wsl-arch", GOOS: "linux", LookPath: present("pacman", "paru", "yay")})
		if len(plan.Commands) != 1 || plan.Commands[0][1] != "pacman" {
			t.Fatalf("commands = %v, want sudo pacman -Syu", plan.Commands)
		}
	})

	t.Run("ubuntu apt-get", func(t *testing.T) {
		plan := PlanSystem(Env{Target: "ubuntu", GOOS: "linux", LookPath: present("apt-get", "snap")})
		want := [][]string{
			{"sudo", "apt-get", "update"},
			{"sudo", "apt-get", "upgrade", "-y"},
		}
		if !commandPlansEqual(plan.Commands, want) {
			t.Fatalf("commands = %v, want %v (snap stays tracked-only)", plan.Commands, want)
		}
	})

	t.Run("ubuntu falls back to apt", func(t *testing.T) {
		plan := PlanSystem(Env{Target: "ubuntu", GOOS: "linux", LookPath: present("apt")})
		want := [][]string{
			{"sudo", "apt", "update"},
			{"sudo", "apt", "upgrade", "-y"},
		}
		if !commandPlansEqual(plan.Commands, want) {
			t.Fatalf("commands = %v, want %v", plan.Commands, want)
		}
	})

	t.Run("linux target has no vendor updater", func(t *testing.T) {
		plan := PlanSystem(Env{Target: "linux", GOOS: "linux", LookPath: present("apt-get", "pacman")})
		if len(plan.Commands) != 0 || !strings.Contains(plan.SkipReason, "linux") {
			t.Fatalf("plan = %#v, want skip for generic linux target", plan)
		}
	})
}

func commandPlansEqual(got, want [][]string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if len(got[i]) != len(want[i]) {
			return false
		}
		for j := range got[i] {
			if got[i][j] != want[i][j] {
				return false
			}
		}
	}
	return true
}
