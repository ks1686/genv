package main

import (
	"strings"
	"testing"
)

// registeredHelpCommands is the user-facing command tree. Hidden internals
// (__complete, updates __run-once) and version (prints version, not usage)
// are omitted. New user-facing commands must be added here.
var registeredHelpCommands = [][]string{
	{"add"},
	{"remove"},
	{"adopt"},
	{"disown"},
	{"list"},
	{"apply"},
	{"config"},
	{"explain"},
	{"edit"},
	{"clean"},
	{"scan"},
	{"status"},
	{"validate"},
	{"upgrade"},
	{"pull"},
	{"migrate"},
	{"export"},
	{"map"},
	{"init"},
	{"completion"},
	{"completion", "install"},
	{"env"},
	{"env", "set"},
	{"env", "unset"},
	{"env", "list"},
	{"shell"},
	{"shell", "alias"},
	{"shell", "alias", "set"},
	{"shell", "alias", "unset"},
	{"shell", "status"},
	{"shell", "edit"},
	{"service"},
	{"service", "add"},
	{"service", "remove"},
	{"service", "list"},
	{"service", "start"},
	{"service", "stop"},
	{"service", "restart"},
	{"service", "status"},
	{"files"},
	{"files", "adopt"},
	{"profile"},
	{"profile", "list"},
	{"profile", "create"},
	{"profile", "switch"},
	{"updates"},
	{"updates", "check"},
	{"updates", "start"},
	{"updates", "stop"},
	{"updates", "status"},
}

var commandGroups = [][]string{
	{"env"},
	{"shell"},
	{"shell", "alias"},
	{"service"},
	{"files"},
	{"profile"},
	{"updates"},
}

func TestRegisteredCommands_Help_printsUsageAndExitsOK(t *testing.T) {
	for _, cmd := range registeredHelpCommands {
		args := append(append([]string{}, cmd...), "--help")
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			assertHelpOK(t, args)
		})
	}
}

func TestCommandGroups_HelpFlags_printUsageAndExitOK(t *testing.T) {
	for _, group := range commandGroups {
		for _, flag := range []string{"help", "--help", "-h"} {
			args := append(append([]string{}, group...), flag)
			t.Run(strings.Join(args, " "), func(t *testing.T) {
				assertHelpOK(t, args)
			})
		}
	}
}

func assertHelpOK(t *testing.T, args []string) {
	t.Helper()
	var code int
	errOut := captureStderr(t, func() { code = run(args) })
	if code != exitOK {
		t.Fatalf("run(%v): expected exitOK (%d), got %d\nstderr: %s", args, exitOK, code, errOut)
	}
	if !strings.Contains(strings.ToLower(errOut), "usage") {
		t.Fatalf("run(%v): stderr = %q, want usage text", args, errOut)
	}
	if strings.Contains(errOut, "unknown subcommand") || strings.Contains(errOut, "unknown command") {
		t.Fatalf("run(%v): stderr = %q, must not report unknown command for help", args, errOut)
	}
}

func TestHelp_listsApplyMismatchFlags(t *testing.T) {
	var code int
	errOut := captureStderr(t, func() { code = run([]string{"help"}) })
	if code != exitOK {
		t.Fatalf("help = %d, want %d\n%s", code, exitOK, errOut)
	}
	if !strings.Contains(errOut, "--backup") {
		t.Fatalf("help omits --backup:\n%s", errOut)
	}
	foundForce := false
	for _, line := range strings.Split(errOut, "\n") {
		if strings.Contains(line, "--force") && !strings.Contains(line, "--force-new-lock") {
			foundForce = true
			break
		}
	}
	if !foundForce {
		t.Fatalf("help omits --force:\n%s", errOut)
	}
	if strings.Contains(errOut, "common to all commands") {
		t.Fatalf("help still says --file is common to all commands:\n%s", errOut)
	}
	if !strings.Contains(errOut, "commands that read a spec") {
		t.Fatalf("help does not scope --file to spec-reading commands:\n%s", errOut)
	}
}

func TestUpdatesStatus_rejectsSpecFileFlag(t *testing.T) {
	var code int
	errOut := captureStderr(t, func() {
		code = run([]string{"updates", "status", "--file", "genv.json"})
	})
	if code != exitUsage {
		t.Fatalf("updates status --file = %d, want %d\n%s", code, exitUsage, errOut)
	}
}

func TestHelp_doesNotHardcodeStaleSchemaVersion8(t *testing.T) {
	cmds := [][]string{{"--help"}}
	for _, cmd := range registeredHelpCommands {
		cmds = append(cmds, append(append([]string{}, cmd...), "--help"))
	}
	for _, args := range cmds {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			var code int
			errOut := captureStderr(t, func() { code = run(args) })
			if code != exitOK {
				t.Fatalf("run(%v): expected exitOK (%d), got %d\nstderr: %s", args, exitOK, code, errOut)
			}
			if strings.Contains(errOut, "schemaVersion 8") {
				t.Errorf("help still hardcodes stale schemaVersion 8:\n%s", errOut)
			}
		})
	}
}
