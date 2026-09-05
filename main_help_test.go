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
	{"service", "status"},
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
