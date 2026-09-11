package external

import (
	"fmt"
	"os"
	"runtime"
)

// ElevationHint describes explicit system-scope elevation; it never implies a silent retry.
func ElevationHint(scope string) string {
	if scope != "system" {
		return ""
	}
	if runtime.GOOS == "windows" {
		return "requires elevated PowerShell"
	}
	return "requires sudo"
}

func wrapSystemScopeError(scope, destination string, err error) error {
	if err == nil || scope != "system" || !os.IsPermission(err) {
		return err
	}
	return fmt.Errorf("system-scope destination %s is not writable; run from an elevated session: %w", destination, err)
}
