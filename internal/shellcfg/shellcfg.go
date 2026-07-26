// Package shellcfg manages the shell configuration fragment written by genv.
//
// The fragment is a POSIX-compatible shell script at ~/.config/genv/shell.sh
// (XDG-aware) that defines aliases and functions declared in the genv.json
// shell block. genv apply rewrites this file atomically and injects a source
// line into the user's shell rc files exactly once.
//
// Per-shell targeting is implemented via inline guards:
//
//	if [ -n "$BASH_VERSION" ]; then alias foo='bar'; fi
//	if [ -n "$ZSH_VERSION"  ]; then alias foo='bar'; fi
package shellcfg

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	genvenv "github.com/ks1686/genv/internal/env"
	"github.com/ks1686/genv/internal/genvfile"
	"github.com/ks1686/genv/internal/schema"
)

// FragmentPath returns the path to the managed shell fragment.
// Respects $XDG_CONFIG_HOME; falls back to ~/.config/genv/shell.sh.
func FragmentPath() (string, error) {
	dir, err := genvfile.DefaultDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "shell.sh"), nil
}

// WriteFragment atomically writes a POSIX-compatible shell fragment that
// defines every alias, function, and source entry in cfg. Entries with
// shell:"powershell" are omitted (owned by the PowerShell profile backend).
// If cfg is nil or has no POSIX-relevant content the fragment is removed.
func WriteFragment(path string, cfg *schema.ShellConfig) error {
	if !hasPOSIXFragmentContent(cfg) {
		err := os.Remove(path)
		if err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("removing empty shell fragment %s: %w", path, err)
		}
		return nil
	}

	var sb strings.Builder
	sb.WriteString("# genv managed shell config — do not edit between these markers\n")
	sb.WriteString("# BEGIN genv shell\n")

	// --- Aliases ---
	posixAliases := make([]string, 0)
	for name, a := range cfg.Aliases {
		if a.Shell == "powershell" {
			continue
		}
		posixAliases = append(posixAliases, name)
	}
	sort.Strings(posixAliases)
	if len(posixAliases) > 0 {
		sb.WriteString("\n# aliases\n")
		for _, name := range posixAliases {
			a := cfg.Aliases[name]
			line := fmt.Sprintf("alias %s=%s", name, singleQuote(a.Value))
			sb.WriteString(shellGuard(line, a.Shell))
			sb.WriteString("\n")
		}
	}

	// --- Functions ---
	posixFuncs := make([]string, 0)
	for name, fn := range cfg.Functions {
		if fn.Shell == "powershell" {
			continue
		}
		posixFuncs = append(posixFuncs, name)
	}
	sort.Strings(posixFuncs)
	if len(posixFuncs) > 0 {
		sb.WriteString("\n# functions\n")
		for _, name := range posixFuncs {
			fn := cfg.Functions[name]
			body := fmt.Sprintf("%s() {\n%s\n}", name, indent(fn.Body))
			sb.WriteString(shellGuard(body, fn.Shell))
			sb.WriteString("\n")
		}
	}

	// --- Source entries ---
	if len(cfg.Source) > 0 {
		sb.WriteString("\n# source\n")
		for _, s := range cfg.Source {
			_, _ = fmt.Fprintf(&sb, ". %s\n", singleQuote(s))
		}
	}

	sb.WriteString("\n# END genv shell\n")

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("creating directory %s: %w", dir, err)
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(sb.String()), 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("saving %s: %w", path, err)
	}
	return nil
}

func hasPOSIXFragmentContent(cfg *schema.ShellConfig) bool {
	if cfg == nil {
		return false
	}
	if len(cfg.Source) > 0 {
		return true
	}
	for _, a := range cfg.Aliases {
		if a.Shell != "powershell" {
			return true
		}
	}
	for _, fn := range cfg.Functions {
		if fn.Shell != "powershell" {
			return true
		}
	}
	return false
}

// ApplyShell writes the managed shell fragment and injects source lines into
// rc files. It reuses env.InjectSourceLine so both fragments share one injector.
func ApplyShell(fragmentPath string, cfg *schema.ShellConfig, rcFiles []string) error {
	if err := WriteFragment(fragmentPath, cfg); err != nil {
		return err
	}
	if !hasPOSIXFragmentContent(cfg) {
		return nil
	}
	for _, rc := range rcFiles {
		if err := genvenv.InjectSourceLine(rc, fragmentPath); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "genv: warning: could not inject source line into %s: %v\n", rc, err)
		}
	}
	return nil
}

// ShellStatusKind classifies a shell config entry's state vs the lock.
type ShellStatusKind string

const (
	ShellStatusOK       ShellStatusKind = "ok"
	ShellStatusModified ShellStatusKind = "modified"
	ShellStatusMissing  ShellStatusKind = "missing"
	ShellStatusExtra    ShellStatusKind = "extra"
)

// ShellStatusEntry is one row in the shell config status report.
type ShellStatusEntry struct {
	Kind      ShellStatusKind
	EntryType string // "alias", "function", "source"
	Name      string // alias/function name; source path for source entries
	SpecValue string // value/body/path from spec
	LockValue string // value/body/path from lock
}

// ShellStatus computes the two-way diff between the spec shell block and the
// lock shell config. Like env status it compares spec vs lock.
func ShellStatus(spec *schema.ShellConfig, lock *genvfile.LockedShellConfig) []ShellStatusEntry {
	var entries []ShellStatusEntry

	// Build lock indexes.
	lockAliases := make(map[string]genvfile.LockedShellAlias)
	lockFunctions := make(map[string]genvfile.LockedShellFunction)
	lockSource := make(map[string]bool)
	if lock != nil {
		for _, a := range lock.Aliases {
			lockAliases[a.Name] = a
		}
		for _, fn := range lock.Functions {
			lockFunctions[fn.Name] = fn
		}
		for _, s := range lock.Source {
			lockSource[s] = true
		}
	}

	// Build spec indexes.
	specAliases := make(map[string]bool)
	specFunctions := make(map[string]bool)
	specSource := make(map[string]bool)

	if spec != nil {
		// --- Aliases ---
		for _, name := range sortedKeys(spec.Aliases) {
			specAliases[name] = true
			a := spec.Aliases[name]
			if la, inLock := lockAliases[name]; inLock {
				kind := ShellStatusOK
				if la.Value != a.Value || la.Shell != a.Shell {
					kind = ShellStatusModified
				}
				entries = append(entries, ShellStatusEntry{Kind: kind, EntryType: "alias", Name: name, SpecValue: a.Value, LockValue: la.Value})
			} else {
				entries = append(entries, ShellStatusEntry{Kind: ShellStatusMissing, EntryType: "alias", Name: name, SpecValue: a.Value})
			}
		}
		// --- Functions ---
		for _, name := range sortedFuncKeys(spec.Functions) {
			specFunctions[name] = true
			fn := spec.Functions[name]
			if lf, inLock := lockFunctions[name]; inLock {
				kind := ShellStatusOK
				if lf.Body != fn.Body || lf.Shell != fn.Shell {
					kind = ShellStatusModified
				}
				entries = append(entries, ShellStatusEntry{Kind: kind, EntryType: "function", Name: name, SpecValue: fn.Body, LockValue: lf.Body})
			} else {
				entries = append(entries, ShellStatusEntry{Kind: ShellStatusMissing, EntryType: "function", Name: name, SpecValue: fn.Body})
			}
		}
		// --- Source ---
		for _, s := range spec.Source {
			specSource[s] = true
			if lockSource[s] {
				entries = append(entries, ShellStatusEntry{Kind: ShellStatusOK, EntryType: "source", Name: s, SpecValue: s, LockValue: s})
			} else {
				entries = append(entries, ShellStatusEntry{Kind: ShellStatusMissing, EntryType: "source", Name: s, SpecValue: s})
			}
		}
	}

	// Lock-side pass: extra entries not in spec.
	if lock != nil {
		for _, la := range lock.Aliases {
			if !specAliases[la.Name] {
				entries = append(entries, ShellStatusEntry{Kind: ShellStatusExtra, EntryType: "alias", Name: la.Name, LockValue: la.Value})
			}
		}
		for _, lf := range lock.Functions {
			if !specFunctions[lf.Name] {
				entries = append(entries, ShellStatusEntry{Kind: ShellStatusExtra, EntryType: "function", Name: lf.Name, LockValue: lf.Body})
			}
		}
		for _, s := range lock.Source {
			if !specSource[s] {
				entries = append(entries, ShellStatusEntry{Kind: ShellStatusExtra, EntryType: "source", Name: s, LockValue: s})
			}
		}
	}

	return entries
}

// SpecToLock converts a spec ShellConfig into a LockedShellConfig.
func SpecToLock(cfg *schema.ShellConfig) *genvfile.LockedShellConfig {
	if cfg == nil {
		return nil
	}
	lsc := &genvfile.LockedShellConfig{}
	for _, name := range sortedKeys(cfg.Aliases) {
		a := cfg.Aliases[name]
		lsc.Aliases = append(lsc.Aliases, genvfile.LockedShellAlias{Name: name, Value: a.Value, Shell: a.Shell})
	}
	for _, name := range sortedFuncKeys(cfg.Functions) {
		fn := cfg.Functions[name]
		lsc.Functions = append(lsc.Functions, genvfile.LockedShellFunction{Name: name, Body: fn.Body, Shell: fn.Shell})
	}
	lsc.Source = append(lsc.Source, cfg.Source...)
	return lsc
}

// shellGuard wraps line in a per-shell if guard when target is non-empty.
// target may be "bash", "zsh", or "fish". Empty target = no guard (all POSIX).
// "powershell" is never emitted here (filtered before shellGuard is called).
func shellGuard(line, target string) string {
	switch target {
	case "bash":
		return fmt.Sprintf("if [ -n \"$BASH_VERSION\" ]; then\n  %s\nfi", line)
	case "zsh":
		return fmt.Sprintf("if [ -n \"$ZSH_VERSION\" ]; then\n  %s\nfi", line)
	case "fish":
		// Fish uses a different syntax; emit a comment noting it is fish-only.
		return fmt.Sprintf("# fish-only (source manually in config.fish):\n# %s", line)
	case "powershell":
		return ""
	default:
		return line
	}
}

// singleQuote wraps v in POSIX single quotes, escaping embedded single quotes.
func singleQuote(v string) string {
	if v == "" {
		return "''"
	}
	var b strings.Builder
	b.Grow(len(v) + 2)
	b.WriteByte('\'')
	for i := 0; i < len(v); i++ {
		if v[i] == '\'' {
			b.WriteString(`'\''`)
			continue
		}
		b.WriteByte(v[i])
	}
	b.WriteByte('\'')
	return b.String()
}

// indent prefixes every line of s with two spaces.
func indent(s string) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = "  " + l
	}
	return strings.Join(lines, "\n")
}

func sortedKeys(m map[string]schema.ShellAlias) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func sortedFuncKeys(m map[string]schema.ShellFunction) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
