package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ks1686/genv/internal/schema"
)

func TestFirstLaunchdProgramArgument(t *testing.T) {
	plist := LaunchdScheduledPlistContent("updates", []string{"/opt/homebrew/bin/genv", "updates", "__run-once"}, time.Hour, nil)
	got, err := FirstLaunchdProgramArgument([]byte(plist))
	if err != nil {
		t.Fatalf("FirstLaunchdProgramArgument: %v", err)
	}
	if got != "/opt/homebrew/bin/genv" {
		t.Fatalf("got %q, want /opt/homebrew/bin/genv", got)
	}
}

func TestFirstSystemdExecStartArgument(t *testing.T) {
	unit := SystemdScheduledUnitContent("updates", []string{"/usr/local/bin/genv", "updates", "__run-once"}, nil)
	got, err := FirstSystemdExecStartArgument(unit)
	if err != nil {
		t.Fatalf("FirstSystemdExecStartArgument: %v", err)
	}
	if got != "/usr/local/bin/genv" {
		t.Fatalf("got %q, want /usr/local/bin/genv", got)
	}
}

func TestExecutablePathStatus_missing(t *testing.T) {
	err := ExecutablePathStatus(filepath.Join(t.TempDir(), "missing-bin"))
	if err == nil {
		t.Fatal("expected error for missing path")
	}
}

func TestExecutablePathStatus_ok(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bin")
	if err := os.WriteFile(path, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := ExecutablePathStatus(path); err != nil {
		t.Fatalf("ExecutablePathStatus: %v", err)
	}
}

func TestListManagedAgentProgramIssues_reportsMissingLaunchdArgv0(t *testing.T) {
	home := t.TempDir()
	agentDir := filepath.Join(home, "Library", "LaunchAgents")
	if err := os.MkdirAll(agentDir, 0o755); err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(home, "gone", "genv")
	plist := LaunchdScheduledPlistContent("updates", []string{missing, "updates", "__run-once"}, time.Hour, nil)
	if err := os.WriteFile(filepath.Join(agentDir, "genv.updates.plist"), []byte(plist), 0o644); err != nil {
		t.Fatal(err)
	}
	// Non-genv agent should be ignored.
	if err := os.WriteFile(filepath.Join(agentDir, "com.example.other.plist"), []byte(plist), 0o644); err != nil {
		t.Fatal(err)
	}
	// Placeholder true command should be ignored.
	truePlist := LaunchdPlistContent("mod-svc", schemaServiceTrue())
	if err := os.WriteFile(filepath.Join(agentDir, "genv.mod-svc.plist"), []byte(truePlist), 0o644); err != nil {
		t.Fatal(err)
	}

	issues := ListManagedAgentProgramIssues(home)
	if len(issues) != 1 {
		t.Fatalf("issues = %#v, want one", issues)
	}
	if issues[0].Label != "genv.updates" || !strings.Contains(issues[0].Detail, missing) {
		t.Fatalf("issue = %#v, want genv.updates missing path", issues[0])
	}
}

func TestListManagedAgentProgramIssues_reportsMissingSystemdExecStart(t *testing.T) {
	home := t.TempDir()
	unitDir := filepath.Join(home, ".config", "systemd", "user")
	if err := os.MkdirAll(unitDir, 0o755); err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(home, "gone", "genv")
	unit := SystemdScheduledUnitContent("updates", []string{missing, "updates", "__run-once"}, nil)
	if err := os.WriteFile(filepath.Join(unitDir, "genv-updates.service"), []byte(unit), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(unitDir, "other.service"), []byte(unit), 0o644); err != nil {
		t.Fatal(err)
	}

	issues := ListManagedAgentProgramIssues(home)
	if len(issues) != 1 {
		t.Fatalf("issues = %#v, want one", issues)
	}
	if issues[0].Label != "genv-updates" || !strings.Contains(issues[0].Detail, missing) {
		t.Fatalf("issue = %#v, want genv-updates missing path", issues[0])
	}
}

func TestFirstLaunchdProgramArgument_missing(t *testing.T) {
	if _, err := FirstLaunchdProgramArgument([]byte(`<plist></plist>`)); err == nil {
		t.Fatal("expected error")
	}
}

func TestFirstSystemdExecStartArgument_variants(t *testing.T) {
	got, err := FirstSystemdExecStartArgument("ExecStart=/usr/bin/true\n")
	if err != nil || got != "/usr/bin/true" {
		t.Fatalf("got %q err %v", got, err)
	}
	got, err = FirstSystemdExecStartArgument(`ExecStart="/usr/bin/env" "genv" "updates"`)
	if err != nil || got != "/usr/bin/env" {
		t.Fatalf("got %q err %v", got, err)
	}
	if _, err := FirstSystemdExecStartArgument("Description=no exec\n"); err == nil {
		t.Fatal("expected error")
	}
}

func TestExecutablePathStatus_emptyAndDirectory(t *testing.T) {
	if err := ExecutablePathStatus("  "); err == nil {
		t.Fatal("expected empty path error")
	}
	dir := t.TempDir()
	if err := ExecutablePathStatus(dir); err == nil {
		t.Fatal("expected directory error")
	}
	path := filepath.Join(dir, "noexec")
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ExecutablePathStatus(path); err == nil {
		t.Fatal("expected not-executable error")
	}
}

func schemaServiceTrue() schema.Service {
	return schema.Service{Start: []string{"true"}}
}
