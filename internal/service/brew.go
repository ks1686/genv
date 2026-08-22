package service

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"

	"github.com/ks1686/genv/internal/resolver"
)

// IsBrewServicesAvailable reports whether `brew services` can be used.
// It requires macOS and brew to be on the PATH.
func IsBrewServicesAvailable() bool {
	if runtime.GOOS != "darwin" {
		return false
	}
	_, err := exec.LookPath("brew")
	return err == nil
}

// BrewServicesStart starts a brew-managed service via `brew services start <formula>`.
func BrewServicesStart(ctx context.Context, formula string) error {
	out, err := exec.CommandContext(ctx, "brew", "services", "start", formula).CombinedOutput()
	if err != nil {
		return fmt.Errorf("brew services start %q: %w\n%s", formula, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// BrewServicesStop stops a brew-managed service via `brew services stop <formula>`.
func BrewServicesStop(ctx context.Context, formula string) error {
	out, err := exec.CommandContext(ctx, "brew", "services", "stop", formula).CombinedOutput()
	if err != nil {
		return fmt.Errorf("brew services stop %q: %w\n%s", formula, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// BrewServicesRestart restarts a brew-managed service via `brew services restart <formula>`.
func BrewServicesRestart(ctx context.Context, formula string) error {
	out, err := exec.CommandContext(ctx, "brew", "services", "restart", formula).CombinedOutput()
	if err != nil {
		return fmt.Errorf("brew services restart %q: %w\n%s", formula, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// BrewServicesRunning reports whether a brew-managed service is currently running.
// It parses the output of `brew services list` and looks for a "started" status.
// The probe is capped like every other manager inventory so a wedged brew
// cannot stall `genv service list` forever.
func BrewServicesRunning(formula string) bool {
	out, err := resolver.CallTimed(func() ([]byte, error) {
		return exec.Command("brew", "services", "list").Output()
	}, resolver.DefaultLiveListTimeout)
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		// Output format: <name> <status> <user> <plist>
		if len(fields) >= 2 && fields[0] == formula && fields[1] == "started" {
			return true
		}
	}
	return false
}

// BrewServicesList returns the raw output of `brew services list`.
// Capped like BrewServicesRunning so a wedged brew cannot stall callers.
func BrewServicesList() (string, error) {
	out, err := resolver.CallTimed(func() ([]byte, error) {
		return exec.Command("brew", "services", "list").Output()
	}, resolver.DefaultLiveListTimeout)
	if err != nil {
		return "", fmt.Errorf("brew services list: %w", err)
	}
	return string(out), nil
}
