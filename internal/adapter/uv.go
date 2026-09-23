package adapter

import (
	"errors"
	"os/exec"
	"strings"
)

// Uv is the adapter for uv global Python tool installs.
// It only operates on uv's tool environment ("uv tool install/uninstall"),
// never on project-level Python packages.
type Uv struct{}

func (Uv) Name() string { return "uv" }

func (Uv) Available() bool {
	_, err := lookPath("uv")
	return err == nil
}

func (Uv) NormalizeID(id string, managers map[string]string) (string, bool) {
	return normalizeID("uv", id, managers)
}

func (Uv) PlanInstall(pkgName string) []string {
	return []string{"uv", "tool", "install", pkgName}
}

func (Uv) PlanUninstall(pkgName string) []string {
	return []string{"uv", "tool", "uninstall", uvResolveToolName(pkgName)}
}

// PlanUpgrade reuses "uv tool install --upgrade", which upgrades an installed
// tool to the latest version matching the requested specifier.
func (Uv) PlanUpgrade(pkgName string) []string {
	return []string{"uv", "tool", "install", "--upgrade", pkgName}
}

// PlanClean runs uv's global cache clean. There is no tool-only variant;
// this clears uv's shared wheel/build cache, which can grow unbounded.
func (Uv) PlanClean() [][]string {
	return [][]string{{"uv", "cache", "clean"}}
}

func (Uv) Query(pkgName string) (bool, error) {
	entries, err := Uv{}.listEntries()
	if err != nil {
		return false, err
	}
	_, ok := uvMatchEntry(pkgName, entries)
	return ok, nil
}

// ListInstalled parses "uv tool list" output. Header lines name the tool
// (e.g. "black v24.2.0"); `- <entrypoint>` bullets and indented names are skipped.
func (Uv) ListInstalled() ([]string, error) {
	entries, err := Uv{}.listEntries()
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.name)
	}
	return names, nil
}

// QueryVersion parses the installed version from "uv tool list" output.
// Returns "", nil when the tool is not installed or has no reported version.
func (Uv) QueryVersion(pkgName string) (string, error) {
	entries, err := Uv{}.listEntries()
	if err != nil {
		return "", err
	}
	if entry, ok := uvMatchEntry(pkgName, entries); ok {
		return entry.version, nil
	}
	return "", nil
}

func (Uv) ListInstalledVersions() (map[string]string, error) {
	entries, err := Uv{}.listEntries()
	if err != nil {
		return nil, err
	}
	versions := make(map[string]string, len(entries))
	for _, entry := range entries {
		versions[entry.name] = entry.version
	}
	return versions, nil
}

// ListOutdated reports installed uv tools with a newer version on PyPI, keyed
// by bare tool name -> latest version, restricted to pkgNames when provided.
func (Uv) ListOutdated(pkgNames []string) (map[string]string, error) {
	entries, err := Uv{}.listEntries()
	if err != nil {
		return nil, err
	}
	return listRegistryOutdated(versionMapOf(entries, func(e uvEntry) (string, string) {
		return e.name, e.version
	}), pkgNames, func(raw string) string {
		if entry, ok := uvMatchEntry(raw, entries); ok {
			return entry.name
		}
		return uvToolName(raw)
	}, pypiLatestVersion)
}

type uvEntry struct {
	name     string
	version  string
	required string
}

func (Uv) listEntries() ([]uvEntry, error) {
	lines, err := runUvToolList()
	if err != nil {
		return nil, err
	}
	return parseUvToolList(lines), nil
}

// parseUvToolList reads "uv tool list" text. Only header lines matching
// `^\S+ v\d` are tools; `- <entrypoint>` bullets and indented names are not.
func parseUvToolList(lines []string) []uvEntry {
	entries := make([]uvEntry, 0, len(lines))
	for _, line := range lines {
		if entry, ok := parseUvToolHeader(line); ok {
			entries = append(entries, entry)
		}
	}
	return entries
}

func parseUvToolHeader(line string) (uvEntry, bool) {
	if line == "" || isIndented(line) || strings.HasPrefix(line, "-") {
		return uvEntry{}, false
	}
	fields := strings.Fields(line)
	if len(fields) < 2 || fields[0] == "-" {
		return uvEntry{}, false
	}
	ver := fields[1]
	if len(ver) < 2 || ver[0] != 'v' || ver[1] < '0' || ver[1] > '9' {
		return uvEntry{}, false
	}
	return uvEntry{name: fields[0], version: ver[1:], required: parseUvRequired(line)}, true
}

func parseUvRequired(line string) string {
	const prefix = "[required:"
	i := strings.Index(line, prefix)
	if i < 0 {
		return ""
	}
	rest, _, _ := strings.Cut(line[i+len(prefix):], "]")
	return strings.TrimSpace(rest)
}

// runUvToolList runs "uv tool list --show-version-specifiers" and returns
// stdout split into lines, preserving leading whitespace so indented
// entrypoint lines can be detected. The extra suffix lets git URL specs match
// the listed [required: ...] value. If that flag is unknown, fall back to
// plain "uv tool list". A non-zero exit from the fallback is "no tools".
func runUvToolList() ([]string, error) {
	out, err := runProbe("uv", "tool", "list", "--show-version-specifiers")
	if err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			return nil, err
		}
		out, err = runProbe("uv", "tool", "list")
		if err != nil {
			if errors.As(err, &exitErr) {
				return nil, nil
			}
			return nil, err
		}
	}
	return nonEmptyLines(string(out)), nil
}

// uvToolName maps a uv install spec to the tool name uv lists. Bare names and
// name@version stay on atVersionBaseName. Git URL specs use the repo basename
// (strip .git, @ref, #fragment) instead of cutting at the git@host separator.
// name@url keeps the explicit package name so the documented workaround still
// works when the repo basename is not the package name.
func uvToolName(s string) string {
	s = strings.TrimSpace(s)
	schemeEnd := strings.Index(s, "://")
	if schemeEnd < 0 {
		return atVersionBaseName(s)
	}
	if at := strings.Index(s[:schemeEnd], "@"); at > 0 {
		return s[:at]
	}
	rest := s[schemeEnd+len("://"):]
	rest, _, _ = strings.Cut(rest, "#")
	rest = strings.TrimRight(rest, "/")
	seg := rest[strings.LastIndex(rest, "/")+1:]
	seg, _, _ = strings.Cut(seg, "@")
	return strings.TrimSuffix(seg, ".git")
}

func uvCanonicalSpec(s string) string {
	s = strings.TrimSpace(s)
	if schemeEnd := strings.Index(s, "://"); schemeEnd >= 0 {
		if at := strings.Index(s[:schemeEnd], "@"); at > 0 {
			s = strings.TrimSpace(s[at+1:])
		}
	}
	s, _, _ = strings.Cut(s, "#")
	s = strings.TrimRight(s, "/")
	if slash := strings.LastIndex(s, "/"); slash >= 0 {
		base := s[slash+1:]
		if at := strings.Index(base, "@"); at >= 0 {
			s = s[:slash+1] + base[:at]
		}
	}
	return s
}

func uvMatchEntry(pkgName string, entries []uvEntry) (uvEntry, bool) {
	name := uvToolName(pkgName)
	want := uvCanonicalSpec(pkgName)
	for _, entry := range entries {
		if entry.required != "" && want != "" && uvCanonicalSpec(entry.required) == want {
			return entry, true
		}
	}
	urlSpec := strings.Contains(pkgName, "://")
	for _, entry := range entries {
		if entry.name != name {
			continue
		}
		if entry.required == "" || !urlSpec {
			return entry, true
		}
	}
	return uvEntry{}, false
}

// uvResolveToolName returns the name uv tool uninstall expects. When the spec
// is a git URL whose repo basename is not the package name, this prefers the
// listed tool that advertises the same required specifier.
func uvResolveToolName(pkgName string) string {
	entries, err := (Uv{}).listEntries()
	if err == nil {
		if entry, ok := uvMatchEntry(pkgName, entries); ok {
			return entry.name
		}
	}
	return uvToolName(pkgName)
}

// isIndented reports whether line starts with horizontal whitespace.
func isIndented(line string) bool {
	return strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t")
}
