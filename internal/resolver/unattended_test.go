package resolver

import (
	"bytes"
	"context"
	"io"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/ks1686/genv/internal/adapter"
	"github.com/ks1686/genv/internal/genvfile"
)

func TestWithNoninteractiveSudo(t *testing.T) {
	tests := []struct {
		name string
		in   []string
		want []string
	}{
		{name: "inserts -n", in: []string{"sudo", "apt-get", "update"}, want: []string{"sudo", "-n", "apt-get", "update"}},
		{name: "idempotent", in: []string{"sudo", "-n", "apt-get", "update"}, want: []string{"sudo", "-n", "apt-get", "update"}},
		{name: "long flag", in: []string{"sudo", "--non-interactive", "pacman", "-Sy"}, want: []string{"sudo", "--non-interactive", "pacman", "-Sy"}},
		{name: "non sudo", in: []string{"brew", "update"}, want: []string{"brew", "update"}},
		{name: "empty", in: nil, want: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := withNoninteractiveSudo(tt.in)
			if !slices.Equal(got, tt.want) {
				t.Fatalf("withNoninteractiveSudo(%v) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestCommandNeedsElevation(t *testing.T) {
	tests := []struct {
		argv []string
		want bool
	}{
		{[]string{"sudo", "apt-get", "update"}, true},
		{[]string{"sudo", "-n", "pacman", "-Sy"}, true},
		{[]string{"choco", "upgrade", "-y", "git"}, true},
		{[]string{"choco", "list"}, false},
		{[]string{"winget", "upgrade", "--id", "Docker.DockerDesktop"}, true},
		{[]string{"winget", "install", "--id", "Git.Git"}, true},
		{[]string{"winget", "source", "update"}, false},
		{[]string{"winget", "list"}, false},
		{[]string{"paru", "-Sy", "--noconfirm"}, true},
		{[]string{"yay", "-S", "--noconfirm", "git"}, true},
		{[]string{"brew", "upgrade", "git"}, false},
		{[]string{"bun", "add", "--global", "cf"}, false},
		{nil, false},
	}
	for _, tt := range tests {
		if got := commandNeedsElevation(tt.argv); got != tt.want {
			t.Errorf("commandNeedsElevation(%v) = %v, want %v", tt.argv, got, tt.want)
		}
	}
}

func TestRefreshIndexes_unattendedSkipsElevatingRefresh(t *testing.T) {
	orig := canElevateWithoutPrompt
	canElevateWithoutPrompt = func() bool { return false }
	t.Cleanup(func() { canElevateWithoutPrompt = orig })

	swapLookupAdapter(t, map[string]adapter.Adapter{
		"apt":  &refreshTestMgr{outdatedTestMgr: outdatedTestMgr{name: "apt"}, cmd: []string{"sudo", "apt-get", "update"}},
		"brew": &refreshTestMgr{outdatedTestMgr: outdatedTestMgr{name: "brew"}, cmd: []string{"brew", "update"}},
	})
	var ran []string
	swapIndexRefresh(t, func(_ context.Context, argv []string) error {
		ran = append(ran, strings.Join(argv, " "))
		return nil
	})

	actions, keepAll, warnings := RefreshIndexes([]genvfile.LockedPackage{
		{ID: "curl", Manager: "apt", PkgName: "curl"},
		{ID: "git", Manager: "brew", PkgName: "git"},
	}, RefreshOptions{Unattended: true})

	if !slices.Equal(ran, []string{"brew update"}) {
		t.Fatalf("ran %v, want only brew", ran)
	}
	if len(keepAll) != 0 {
		t.Fatalf("keepAll = %v, want empty when refresh is skipped not failed", keepAll)
	}
	if len(actions) != 2 {
		t.Fatalf("actions = %#v, want apt then brew", actions)
	}
	aptCmd := actions[0].Cmd
	if actions[0].Manager != "apt" {
		aptCmd = actions[1].Cmd
	}
	if !slices.Equal(aptCmd, []string{"sudo", "-n", "apt-get", "update"}) {
		t.Fatalf("apt refresh cmd = %v, want sudo -n", aptCmd)
	}
	joined := strings.Join(warnings, "\n")
	if !strings.Contains(joined, "requires elevation (unattended)") {
		t.Fatalf("warnings = %v, want elevation skip", warnings)
	}
}

func TestRefreshIndexes_unattendedRunsSudoNWhenAlreadyNoninteractive(t *testing.T) {
	orig := canElevateWithoutPrompt
	canElevateWithoutPrompt = func() bool { return true }
	t.Cleanup(func() { canElevateWithoutPrompt = orig })

	swapLookupAdapter(t, map[string]adapter.Adapter{
		"apt": &refreshTestMgr{outdatedTestMgr: outdatedTestMgr{name: "apt"}, cmd: []string{"sudo", "apt-get", "update"}},
	})
	var ran []string
	swapIndexRefresh(t, func(_ context.Context, argv []string) error {
		ran = append(ran, strings.Join(argv, " "))
		return nil
	})

	_, keepAll, _ := RefreshIndexes([]genvfile.LockedPackage{
		{ID: "curl", Manager: "apt", PkgName: "curl"},
	}, RefreshOptions{Unattended: true})
	if !slices.Equal(ran, []string{"sudo -n apt-get update"}) {
		t.Fatalf("ran %v, want sudo -n", ran)
	}
	if len(keepAll) != 0 {
		t.Fatalf("keepAll = %v", keepAll)
	}
}

func TestExecuteUpgrade_unattendedSkipsWingetWithoutSpawning(t *testing.T) {
	orig := canElevateWithoutPrompt
	canElevateWithoutPrompt = func() bool { return false }
	t.Cleanup(func() { canElevateWithoutPrompt = orig })

	out := ExecuteUpgrade(context.Background(), []UpgradeAction{{
		LPs: []genvfile.LockedPackage{{ID: "docker-desktop", Manager: "winget", PkgName: "Docker.DockerDesktop"}},
		Cmd: []string{"winget", "upgrade", "--id", "Docker.DockerDesktop"},
	}}, nil, io.Discard, io.Discard, ApplyExecutionOptions{Unattended: true})
	if len(out.Errors) != 0 || len(out.Upgraded) != 0 {
		t.Fatalf("execution = %+v, want skip without errors", out)
	}
	if len(out.Skipped) != 1 || out.Skipped[0].ID != "docker-desktop" || !strings.Contains(out.Skipped[0].Reason, "requires elevation (unattended)") {
		t.Fatalf("skipped = %+v", out.Skipped)
	}
}

func TestExecuteUpgrade_interactiveStillRunsElevatingCommand(t *testing.T) {
	var buf bytes.Buffer
	cmd := []string{"true"}
	if runtime.GOOS == "windows" {
		cmd = []string{"cmd", "/C", "exit", "0"}
	}
	out := ExecuteUpgrade(context.Background(), []UpgradeAction{{
		LPs: []genvfile.LockedPackage{{ID: "true", Manager: "brew", PkgName: "true"}},
		Cmd: cmd,
	}}, nil, &buf, io.Discard)
	if len(out.Errors) != 0 {
		t.Fatalf("errors = %v", out.Errors)
	}
	if len(out.Skipped) != 0 {
		t.Fatalf("skipped = %v, interactive must not skip", out.Skipped)
	}
}
