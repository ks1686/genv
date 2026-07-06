// Package host detects the current host name and filters schema records by
// their HostPredicate.
package host

import (
	"fmt"
	"os"
	"runtime"
	"strings"

	"github.com/ks1686/genv/internal/schema"
)

// Current returns the host name used for HostPredicate matching.
//
// It prefers the GENV_HOST environment variable, falling back to os.Hostname().
// Callers that receive an error should log a warning and treat all non-empty
// host predicates as non-matching.
func Current() (string, error) {
	if h := os.Getenv("GENV_HOST"); h != "" {
		return h, nil
	}
	return os.Hostname()
}

// Classify returns the host class used for HostPredicate matching.
//
// It prefers the GENV_HOST environment variable, then detects the platform:
//   - macOS -> "macos"
//   - WSL2  -> "wsl2"
//   - Arch Linux -> "arch"
//
// Callers that receive an error should log a warning and treat all non-empty
// host predicates as non-matching.
func Classify() (string, error) {
	if h := os.Getenv("GENV_HOST"); h != "" {
		return h, nil
	}
	switch runtime.GOOS {
	case "darwin":
		return "macos", nil
	case "linux":
		if isWSL() {
			return "wsl2", nil
		}
		if isArch() {
			return "arch", nil
		}
		return "", fmt.Errorf("unrecognized Linux distribution on %q; set GENV_HOST", runtime.GOOS)
	default:
		return "", fmt.Errorf("unrecognized OS %q; set GENV_HOST", runtime.GOOS)
	}
}

func isWSL() bool {
	data, err := os.ReadFile("/proc/version")
	if err != nil {
		return false
	}
	s := strings.ToLower(string(data))
	return strings.Contains(s, "microsoft") || strings.Contains(s, "wsl")
}

func isArch() bool {
	data, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return false
	}
	s := string(data)
	return strings.Contains(s, "ID=arch") || strings.Contains(s, "ID_LIKE=arch")
}

// Match reports whether predicate matches host. An empty predicate matches
// every host. A non-empty predicate matches only when host is non-empty and
// appears in predicate.
//
// WSL2 inherits Arch records: a record whose Host predicate contains "arch"
// also applies when host is "wsl2".
func Match(predicate schema.HostPredicate, host string) bool {
	if len(predicate) == 0 {
		return true
	}
	if host == "" {
		return false
	}
	for _, h := range predicate {
		if h == host {
			return true
		}
		if host == "wsl2" && h == "arch" {
			return true
		}
	}
	return false
}
