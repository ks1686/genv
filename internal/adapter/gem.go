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

// ListForScan returns user-installed gems, skipping the gems that ship with
// Ruby rather than being chosen by the user. ListInstalled stays complete for
// apply, status and upgrade.
func (g Gem) ListForScan() ([]string, error) {
	entries, err := g.listEntries()
	if err != nil {
		return nil, err
	}
	defaults := gemDefaultGems()
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if defaults[entry.name] || rubyBundledGem[entry.name] {
			continue
		}
		names = append(names, entry.name)
	}
	return names, nil
}

// gemDefaultGems returns the gem names the running Ruby treats as default.
//
// This cannot be derived from `gem list --local` text. RubyGems prints the
// `default:` marker only when a gem has a second, non-default version also
// installed, so the output looks like this on RubyGems 4.0.20:
//
//	abbrev (0.1.2)                      default gem, no marker
//	bundler (default: 4.0.20, 4.0.18)   marked only because two versions exist
//
// Inferring "is a default gem" from that marker therefore drops every default
// gem that has a single version installed, which is exactly the abbrev,
// base64, benchmark, bigdecimal and csv that the default scan proposed.
// Asking RubyGems for the set is version-proof instead (#216).
//
// It is a var so tests can supply a known set.
//
// Known limitation, RubyGems 4: gems that Ruby used to ship as default gems
// (csv, base64, bigdecimal, benchmark, abbrev) are no longer reported by
// default_stubs there, and RubyGems 4 removed `bundled_gem?`. They land in
// the same gem home as a user install, so nothing in RubyGems distinguishes
// them — on a Ruby 4 host the default scan still proposes them. That is a
// Ruby limitation, not a filter that can be tightened without hardcoding a
// per-release list.
var gemDefaultGems = func() map[string]bool {
	out, err := runProbe("ruby", "-e", `require "rubygems"; puts Gem::Specification.default_stubs.map(&:name)`)
	if err != nil {
		return nil
	}
	set := make(map[string]bool)
	for line := range strings.SplitSeq(string(out), "\n") {
		if name := strings.TrimSpace(line); name != "" {
			set[name] = true
		}
	}
	return set
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
	name       string
	version    string
	defaultGem bool
}

// rubyBundledGem is the set of gems Ruby ships beside the interpreter
// (tool/bundled_gems). They look like ordinary `gem list` entries — no
// `default:` marker — but adopting them pins stdlib that `gem upgrade`
// cannot usefully manage.
var rubyBundledGem = map[string]bool{
	"debug":           true,
	"error_highlight": true,
	"irb":             true,
	"matrix":          true,
	"minitest":        true,
	"net-ftp":         true,
	"net-imap":        true,
	"net-pop":         true,
	"net-smtp":        true,
	"power_assert":    true,
	"prime":           true,
	"racc":            true,
	"rake":            true,
	"rbs":             true,
	"rdoc":            true,
	"reline":          true,
	"rexml":           true,
	"rss":             true,
	"syntax_suggest":  true,
	"test-unit":       true,
	"typeprof":        true,
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
	_ = f.Close()
	_ = os.Remove(name)
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
	defaultGem := strings.Contains(versions, "default:")
	versions = strings.TrimPrefix(versions, "default: ")
	first, _, _ := strings.Cut(versions, ",")
	return gemEntry{name: name, version: strings.TrimSpace(first), defaultGem: defaultGem}, true
}
