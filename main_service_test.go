package main

import (
	"path/filepath"
	"testing"

	"github.com/ks1686/genv/internal/genvfile"
)

func TestServiceCLICommands(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	path := filepath.Join(dir, "genv.json")

	if code := run([]string{
		"service", "add", "managed", "--file", path,
		"--start", `true "quoted argument"`,
		"--stop", `true "quoted argument"`,
		"--status", `true "quoted argument"`,
	}); code != exitOK {
		t.Fatalf("service add: got exit code %d, want %d", code, exitOK)
	}

	spec, err := genvfile.Read(path)
	if err != nil {
		t.Fatalf("read service spec: %v", err)
	}
	if got := spec.Services["managed"].Start; len(got) != 2 || got[1] != "quoted argument" {
		t.Fatalf("parsed start command = %q, want quoted argument preserved", got)
	}

	if code := run([]string{
		"service", "add", "failing", "--file", path,
		"--start", "false", "--stop", "false", "--status", "false",
	}); code != exitOK {
		t.Fatalf("add failing service: got exit code %d, want %d", code, exitOK)
	}

	if code := run([]string{"service", "list", "--file", path}); code != exitOK {
		t.Errorf("service list: got exit code %d, want %d", code, exitOK)
	}
	if code := run([]string{"service", "ls", "--file", path}); code != exitOK {
		t.Errorf("service ls: got exit code %d, want %d", code, exitOK)
	}

	for _, command := range []string{"start", "stop", "status"} {
		if code := run([]string{"service", command, "managed", "--file", path}); code != exitOK {
			t.Errorf("service %s success path: got exit code %d, want %d", command, code, exitOK)
		}
		if code := run([]string{"service", command, "failing", "--file", path}); code != exitLogic {
			t.Errorf("service %s failure path: got exit code %d, want %d", command, code, exitLogic)
		}
	}

	if code := run([]string{"service", "rm", "failing", "--file", path}); code != exitOK {
		t.Errorf("service rm: got exit code %d, want %d", code, exitOK)
	}
	if code := run([]string{"service", "remove", "managed", "--file", path}); code != exitOK {
		t.Errorf("service remove: got exit code %d, want %d", code, exitOK)
	}
	if code := run([]string{"service", "remove", "missing", "--file", path}); code != exitLogic {
		t.Errorf("service remove missing: got exit code %d, want %d", code, exitLogic)
	}
}

func TestServiceCLIUsageErrors(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "genv.json")

	tests := []struct {
		name string
		args []string
	}{
		{"missing subcommand", []string{"service"}},
		{"unknown subcommand", []string{"service", "restart"}},
		{"add missing name", []string{"service", "add", "--file", path, "--start", "true"}},
		{"add missing start", []string{"service", "add", "missing-start", "--file", path}},
		{"remove missing name", []string{"service", "remove", "--file", path}},
		{"start missing name", []string{"service", "start", "--file", path}},
		{"stop missing name", []string{"service", "stop", "--file", path}},
		{"status missing name", []string{"service", "status", "--file", path}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if code := run(tt.args); code != exitUsage {
				t.Fatalf("run(%q): got exit code %d, want %d", tt.args, code, exitUsage)
			}
		})
	}
}
