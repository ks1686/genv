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
	"windows": {
		`C:\Windows\System32`,
		`C:\Windows`,
		`C:\Windows\System32\Wbem`,
		`C:\Windows\System32\WindowsPowerShell\v1.0`,
		`C:\Windows\System32\OpenSSH`,
	},
}

// ScheduledPath returns a sanitized PATH augmented with defaults for goos.
func ScheduledPath(pathValue, goos string) string {
	return scheduledPathFrom(pathValue, goos, scheduledPathDefaults[goos])
}

// ScheduledWindowsPath sanitizes a Windows PATH and appends system dirs plus
// per-user scoop/winget shim locations so a Task Scheduler job can find them.
func ScheduledWindowsPath(pathValue, home, localAppData, scoopRoot string) string {
	extras := append([]string{}, scheduledPathDefaults["windows"]...)
	extras = append(extras, WindowsUserShimDirs(home, localAppData, scoopRoot)...)
	return scheduledPathFrom(pathValue, "windows", extras)
}

// WindowsUserShimDirs returns per-user scoop and winget shim directories.
func WindowsUserShimDirs(home, localAppData, scoopRoot string) []string {
	var dirs []string
	if scoopRoot != "" {
		dirs = append(dirs, scoopRoot+`\shims`)
	}
	if home != "" {
		dirs = append(dirs, home+`\scoop\shims`)
	}
	if localAppData != "" {
		dirs = append(dirs, localAppData+`\Microsoft\WindowsApps`)
		dirs = append(dirs, localAppData+`\Microsoft\WinGet\Links`)
	}
	return dirs
}

func scheduledPathFrom(pathValue, goos string, extras []string) string {
	sep := scheduledPathListSeparator(goos)
	entries := make([]string, 0)
	seen := make(map[string]struct{})
	appendEntry := func(entry string) {
		if entry == "" || !scheduledPathIsAbs(entry, goos) {
			return
		}
		if _, ok := seen[entry]; ok {
			return
		}
		seen[entry] = struct{}{}
		entries = append(entries, entry)
	}

	for _, entry := range strings.Split(pathValue, sep) {
		if isHomebrewShimDir(entry) {
			continue
		}
		appendEntry(entry)
	}
	for _, entry := range extras {
		appendEntry(entry)
	}

	return strings.Join(entries, sep)
}

func scheduledPathListSeparator(goos string) string {
	if goos == "windows" {
		return ";"
	}
	return ":"
}

func scheduledPathIsAbs(entry, goos string) bool {
	if goos == "windows" {
		return windowsPathIsAbs(entry)
	}
	return path.IsAbs(entry)
}

func windowsPathIsAbs(entry string) bool {
	if strings.HasPrefix(entry, `\\`) {
		return true
	}
	if len(entry) < 3 {
		return false
	}
	drive := entry[0]
	if (drive < 'A' || drive > 'Z') && (drive < 'a' || drive > 'z') {
		return false
	}
	if entry[1] != ':' {
		return false
	}
	return entry[2] == '\\' || entry[2] == '/'
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
