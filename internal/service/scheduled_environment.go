package service

import (
	"fmt"
	"path"
	"sort"
	"strings"
)

var scheduledPathDefaults = map[string][]string{
	"darwin": {
		"/opt/homebrew/bin",
		"/opt/homebrew/sbin",
		"/usr/local/bin",
		"/usr/local/sbin",
		"/usr/bin",
		"/bin",
		"/usr/sbin",
		"/sbin",
	},
	"linux": {
		"/home/linuxbrew/.linuxbrew/bin",
		"/home/linuxbrew/.linuxbrew/sbin",
		"/usr/local/bin",
		"/usr/local/sbin",
		"/usr/bin",
		"/bin",
		"/usr/sbin",
		"/sbin",
	},
}

// ScheduledPath returns a sanitized PATH augmented with defaults for goos.
func ScheduledPath(pathValue, goos string) string {
	entries := make([]string, 0)
	seen := make(map[string]struct{})
	appendEntry := func(entry string) {
		if entry == "" || !path.IsAbs(entry) {
			return
		}
		if _, ok := seen[entry]; ok {
			return
		}
		seen[entry] = struct{}{}
		entries = append(entries, entry)
	}

	for _, entry := range strings.Split(pathValue, ":") {
		if isHomebrewShimDir(entry) {
			continue
		}
		appendEntry(entry)
	}
	for _, entry := range scheduledPathDefaults[goos] {
		appendEntry(entry)
	}

	return strings.Join(entries, ":")
}

func isHomebrewShimDir(entry string) bool {
	// Homebrew injects these during cask/formula hooks; they must not be baked
	// into launchd/systemd PATH (shims expect an interactive brew session).
	return strings.Contains(entry, "/Homebrew/shims/") || strings.Contains(entry, "/linuxbrew/.linuxbrew/Homebrew/shims/")
}

func renderSystemdEnvironment(environment map[string]string) string {
	var b strings.Builder
	for _, name := range sortedEnvironmentNames(environment) {
		fmt.Fprintf(&b, "Environment=%s\n", systemdQuoteArg(stripLineBreaks(name+"="+environment[name])))
	}
	return b.String()
}

func renderLaunchdEnvironment(environment map[string]string) string {
	if len(environment) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("    <key>EnvironmentVariables</key>\n    <dict>\n")
	for _, name := range sortedEnvironmentNames(environment) {
		fmt.Fprintf(&b, "        <key>%s</key>\n", xmlEscape(stripLineBreaks(name)))
		fmt.Fprintf(&b, "        <string>%s</string>\n", xmlEscape(stripLineBreaks(environment[name])))
	}
	b.WriteString("    </dict>\n")
	return b.String()
}

func sortedEnvironmentNames(environment map[string]string) []string {
	names := make([]string, 0, len(environment))
	for name := range environment {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
