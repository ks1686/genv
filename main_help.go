package main

import (
	"errors"
	"flag"
)

func isHelpArg(s string) bool {
	switch s {
	case "help", "--help", "-h":
		return true
	default:
		return false
	}
}

// flagParseExit maps flag.Parse errors to CLI exit codes. --help/-h are
// success; any other parse failure is a usage error.
func flagParseExit(err error) int {
	if errors.Is(err, flag.ErrHelp) {
		return exitOK
	}
	return exitUsage
}
