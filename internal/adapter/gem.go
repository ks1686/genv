package adapter

import (
	"errors"
	"os"
	"os/exec"
	"strings"
)

// Gem manages globally installed Ruby gems via `gem install`.
// When multiple versions of a gem are installed, `gem uninstall -a` removes
// all of them; genv tracks a gem by name, so uninstall targets that gem's
// installed versions.
type Gem struct{}

func (Gem) Name() string { return "gem" }

func (Gem) Available() bool {
	_, err := lookPath("gem")
	return err == nil
}

func (Gem) NormalizeID(id string, managers map[string]string) (string, bool) {
	return normalizeID("gem", id, managers)
}

func (Gem) PlanInstall(pkgName string) []string {
	return []string{"gem", "install", pkgName}
}

func (Gem) PlanUninstall(pkgName string) []string {
	return []string{"gem", "uninstall", "-x", "-a", atVersionBaseName(pkgName)}
}

func (Gem) PlanUpgrade(pkgName string) []string {
	return []string{"gem", "install", pkgName}
}

func (Gem) PlanClean() [][]string {
	return [][]string{{"gem", "cleanup"}}
}

func (g Gem) Query(pkgName string) (bool, error) {
	entries, err := g.listEntries()
	if err != nil {
		return false, err
	}
	_, ok := findGemEntry(entries, pkgName)
	return ok, nil
}

func (g Gem) ListInstalled() ([]string, error) {
	entries, err := g.listEntries()
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.name)
	}
	return names, nil
}

func (g Gem) QueryVersion(pkgName string) (string, error) {
	entries, err := g.listEntries()
	if err != nil {
		return "", err
	}
	entry, ok := findGemEntry(entries, pkgName)
	if !ok {
		return "", nil
	}
	return entry.version, nil
}

func (g Gem) ListInstalledVersions() (map[string]string, error) {
	entries, err := g.listEntries()
	if err != nil {
		return nil, err
	}
	versions := make(map[string]string, len(entries))
	for _, entry := range entries {
		versions[entry.name] = entry.version
	}
	return versions, nil
}

type gemEntry struct {
	name    string
	version string
}

func (Gem) listEntries() ([]gemEntry, error) {
	// Skip gems entirely when their installation directory is not writable —
	// e.g. macOS system Ruby at /Library/Ruby/Gems, whose gems are root-owned.
	// genv cannot install, upgrade, or uninstall there, so reporting them only
	// pollutes `scan` with unmanageable packages that fail every upgrade with
	// Gem::FilePermissionError.
	if !gemManageable() {
		return nil, nil
	}
	out, err := runProbe("gem", "list", "--local")
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return nil, nil
		}
		return nil, err
	}
	return parseGemListEntries(string(out)), nil
}

// gemManageable reports whether genv can manage gems in the active gem
// installation directory. It is a package var so tests can override the probe.
// When the directory cannot be determined it returns true, preserving the
// default "list everything" behavior rather than hiding a writable install.
var gemManageable = func() bool {
	out, err := runProbe("gem", "environment", "gemdir")
	if err != nil {
		return true
	}
	dir := strings.TrimSpace(string(out))
	if dir == "" {
		return true
	}
	return dirWritable(dir)
}

// dirWritable reports whether dir exists and the current user can create files
// in it, probed by creating and removing a temporary file. This captures POSIX
// permissions and ACLs that a mode-bit check would miss.
func dirWritable(dir string) bool {
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return false
	}
	f, err := os.CreateTemp(dir, ".genv-writecheck-*")
	if err != nil {
		return false
	}
	name := f.Name()
	f.Close()
	os.Remove(name)
	return true
}

func findGemEntry(entries []gemEntry, pkgName string) (gemEntry, bool) {
	base := atVersionBaseName(pkgName)
	for _, entry := range entries {
		if entry.name == base {
			return entry, true
		}
	}
	return gemEntry{}, false
}

// parseGemListEntries parses `gem list --local` output. Each line looks like
// "rake (13.0.6)" or "json (default: 2.3.0, 2.6.1)". The first version listed
// is reported; a leading "default: " marker is stripped.
func parseGemListEntries(out string) []gemEntry {
	var entries []gemEntry
	for line := range strings.SplitSeq(out, "\n") {
		if entry, ok := parseGemListLine(line); ok {
			entries = append(entries, entry)
		}
	}
	return entries
}

func parseGemListLine(line string) (gemEntry, bool) {
	line = strings.TrimSpace(line)
	open := strings.Index(line, "(")
	if open <= 0 || !strings.HasSuffix(line, ")") {
		return gemEntry{}, false
	}
	name := strings.TrimSpace(line[:open])
	if name == "" {
		return gemEntry{}, false
	}
	versions := line[open+1 : len(line)-1]
	versions = strings.TrimPrefix(versions, "default: ")
	first, _, _ := strings.Cut(versions, ",")
	return gemEntry{name: name, version: strings.TrimSpace(first)}, true
}
