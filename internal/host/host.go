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
// It detects the platform:
//   - macOS -> "macos"
//   - WSL Arch-like -> "wsl-arch"
//   - Arch Linux -> "arch"
//   - Ubuntu-like Linux -> "ubuntu"
//   - Windows -> "windows" (the native Windows host, not WSL2)
//
// Callers that receive an error should log a warning and treat all non-empty
// host predicates as non-matching.
func Classify() (string, error) {
	switch runtime.GOOS {
	case "darwin":
		return "macos", nil
	case "windows":
		return "windows", nil
	case "linux":
		osRelease, err := os.ReadFile("/etc/os-release")
		if err != nil {
			osRelease = nil
		}
		procVersion, err := os.ReadFile("/proc/version")
		if err != nil {
			procVersion = nil
		}
		return classifyLinux(string(osRelease), string(procVersion))
	default:
		return "", fmt.Errorf("unrecognized OS %q; set GENV_TARGET", runtime.GOOS)
	}
}

func isWSL() bool {
	data, err := os.ReadFile("/proc/version")
	if err != nil {
		return false
	}
	return isWSLProcVersion(string(data))
}

func isArch() bool {
	data, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return false
	}
	return osReleaseHasIDOrLike(string(data), "arch")
}

func classifyLinux(osRelease, procVersion string) (string, error) {
	wsl := isWSLProcVersion(procVersion)
	arch := osReleaseHasIDOrLike(osRelease, "arch")
	ubuntu := osReleaseHasIDOrLike(osRelease, "ubuntu")

	switch {
	case wsl && arch:
		return "wsl-arch", nil
	case wsl && ubuntu:
		return "ubuntu", nil
	case wsl:
		return "", fmt.Errorf("unrecognized WSL Linux distribution; set GENV_TARGET")
	case arch:
		return "arch", nil
	case ubuntu:
		return "ubuntu", nil
	default:
		return "", fmt.Errorf("unrecognized Linux distribution; set GENV_TARGET")
	}
}

func isWSLProcVersion(procVersion string) bool {
	s := strings.ToLower(procVersion)
	return strings.Contains(s, "microsoft") || strings.Contains(s, "wsl")
}

func osReleaseHasIDOrLike(osRelease, target string) bool {
	fields := parseOSRelease(osRelease)
	target = strings.ToLower(target)
	id := strings.ToLower(fields["ID"])
	if id == target {
		return true
	}
	for _, part := range strings.Fields(strings.ToLower(fields["ID_LIKE"])) {
		if strings.Trim(part, `"'`) == target {
			return true
		}
	}
	return false
}

func parseOSRelease(data string) map[string]string {
	fields := make(map[string]string)
	for _, line := range strings.Split(data, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.ToUpper(strings.TrimSpace(key))
		value = strings.TrimSpace(value)
		value = strings.Trim(value, `"'`)
		fields[key] = value
	}
	return fields
}

// Match reports whether predicate matches host. An empty predicate matches
// every host. A non-empty predicate matches only when host is non-empty and
// appears in predicate.
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
	}
	return false
}
