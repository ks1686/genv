// Package env manages the shell environment variable fragment written by genv.
//
// The fragment is a POSIX shell script at ~/.config/genv/env.sh (XDG-aware)
// that exports every variable declared in the genv.json env block. genv apply
// rewrites this file atomically and injects a source line into the user's shell
// rc files (bash and zsh) exactly once.
package env

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ks1686/genv/internal/genvfile"
	"github.com/ks1686/genv/internal/schema"
)

// FragmentPath returns the path to the managed shell fragment.
// Respects $XDG_CONFIG_HOME; falls back to ~/.config/genv/env.sh.
func FragmentPath() (string, error) {
	dir, err := genvfile.DefaultDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "env.sh"), nil
}

// WriteFragment atomically writes a POSIX shell fragment that exports every
// variable in vars. Sensitive values are written as their actual value (the
// fragment must contain real values for shells to export them); redaction only
// applies to log/JSON output. If vars is empty the fragment is removed.
func WriteFragment(path string, vars map[string]schema.EnvVar) error {
	if len(vars) == 0 {
		// Nothing to export — remove the fragment if it exists.
		err := os.Remove(path)
		if err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("removing empty fragment %s: %w", path, err)
		}
		return nil
	}

	var sb strings.Builder
	sb.WriteString("# genv managed env — do not edit between these markers\n")
	sb.WriteString("# BEGIN genv env\n")

	// Sort for deterministic output.
	names := make([]string, 0, len(vars))
	for name := range vars {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		ev := vars[name]
		sb.WriteString("export ")
		sb.WriteString(name)
		sb.WriteString("=")
		sb.WriteString(shellQuote(ev.Value))
		sb.WriteString("\n")
	}

	sb.WriteString("# END genv env\n")

	if err := genvfile.WritePrivate(path, []byte(sb.String())); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	return nil
}

// envBlockMarker introduces the genv-managed source line in an rc file. The
// wording is load-bearing: InjectSourceLine recognises it to find a block to
// replace, and blocks written by older genv versions use this same marker, so
// they are upgraded in place rather than duplicated.
const envBlockMarker = "# genv env"

// sourceLine renders the guarded dot-source line for fragmentPath.
//
// Two properties matter, both from #217:
//
//   - Guarded. The fragment is rendered output, not a tracked file, so it is
//     legitimately absent on a fresh clone and on any host that has not
//     applied since the last render. Sourcing it unguarded printed
//     "No such file or directory" from every non-interactive shell start until
//     someone ran `genv apply`. The guard is an `if`, not `[ -r x ] && . x`:
//     the `&&` form leaves a false exit status when the fragment is missing,
//     which is itself a problem for the non-interactive shells this fixes,
//     since it breaks any later `&&` chain and can trip `set -e`.
//   - $HOME-relative. A single committed rc template has to be correct on
//     every host. An absolute path baked in from one machine is wrong on all
//     the others.
//
// A fragment outside the home directory (a custom --state-dir) cannot be
// expressed as $HOME-relative, so it keeps its absolute path but is still
// guarded.
func sourceLine(fragmentPath string) string {
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		if rel, ok := homeRelative(home, fragmentPath); ok {
			p := `"$HOME/` + rel + `"`
			return "if [ -r " + p + " ]; then . " + p + "; fi"
		}
	}
	q := shellQuote(fragmentPath)
	return "if [ -r " + q + " ]; then . " + q + "; fi"
}

// homeRelative returns fragmentPath relative to home, in slash form, when it
// lies underneath it. Both sides are resolved first so a symlinked or
// non-clean path still compares equal.
func homeRelative(home, fragmentPath string) (string, bool) {
	cleanHome := filepath.Clean(home)
	cleanFrag := filepath.Clean(fragmentPath)
	rel, err := filepath.Rel(cleanHome, cleanFrag)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}
	return filepath.ToSlash(rel), true
}

// InjectSourceLine ensures that fragmentPath is sourced exactly once in rcPath.
//
// An existing genv block is replaced rather than appended to. Recognition is
// by marker, not by path: a committed rc template carrying one host's absolute
// path never matched on another host, so genv appended a second block on first
// apply and the file stayed permanently dirty (#217).
func InjectSourceLine(rcPath, fragmentPath string) error {
	line := sourceLine(fragmentPath)

	data, err := os.ReadFile(rcPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("reading %s: %w", rcPath, err)
	}

	dir := filepath.Dir(rcPath)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("creating directory %s: %w", dir, err)
	}

	updated := replaceOrAppendBlock(string(data), envBlockMarker, line, filepath.Base(fragmentPath))
	if updated == string(data) {
		return nil
	}
	return os.WriteFile(rcPath, []byte(updated), 0o644)
}

// managedFragmentNames returns the fragment basenames that share one "# genv env"
// block. ApplyEnv and ApplyShell each call InjectSourceLine with only their
// own path, so the rewriter has to recognise both or the line it is not
// currently rewriting is left behind and duplicated on the next apply (#226).
func managedFragmentNames(fragmentBase string) []string {
	names := []string{"env.sh", "shell.sh"}
	if fragmentBase != "" && fragmentBase != "env.sh" && fragmentBase != "shell.sh" {
		names = append(names, fragmentBase)
	}
	return names
}

// replaceOrAppendBlock rewrites the block introduced by marker so it holds
// one source line per managed fragment, appending a fresh block when the
// file has none. fragmentBase is the fragment being injected now.
//
// The block is every consecutive source line under the marker that names a
// managed fragment (guarded `if [ -r ...]` or a legacy `. /path`). Anything
// after that run, including the user's own source lines, is left alone.
// Duplicate copies of the same fragment collapse, and the block is rewritten
// in a fixed order so a later apply is a no-op.
func replaceOrAppendBlock(content, marker, sourceLine, fragmentBase string) string {
	lines := strings.Split(content, "\n")
	start := -1
	for i, l := range lines {
		if strings.TrimSpace(l) == marker {
			start = i
			break
		}
	}
	if start < 0 {
		return content + "\n" + marker + "\n" + sourceLine + "\n"
	}

	names := managedFragmentNames(fragmentBase)
	kept := make(map[string]string)
	end := start + 1
	for end < len(lines) {
		name, ok := managedSourceLine(lines[end], names)
		if !ok {
			break
		}
		kept[name] = strings.TrimRight(lines[end], "\r")
		end++
	}
	kept[fragmentBase] = sourceLine

	out := make([]string, 0, len(lines)+len(kept)+1)
	out = append(out, lines[:start]...)
	out = append(out, marker)
	for _, name := range orderedFragmentNames(kept) {
		out = append(out, kept[name])
	}
	out = append(out, lines[end:]...)
	return strings.Join(out, "\n")
}

// managedSourceLine reports which managed fragment a rc line sources.
// Recognition is the line shape plus the basename, so a comment that merely
// mentions env.sh, or the user's own `if [ -r other.sh ]`, is not part of
// the block.
func managedSourceLine(line string, names []string) (string, bool) {
	trimmed := strings.TrimSpace(line)
	if !strings.HasPrefix(trimmed, "if [ -r ") && !strings.HasPrefix(trimmed, ". ") {
		return "", false
	}
	var hit string
	for _, name := range names {
		if name == "" {
			continue
		}
		if strings.Contains(line, "/"+name) || strings.Contains(line, `\`+name) {
			if len(name) > len(hit) {
				hit = name
			}
		}
	}
	if hit == "" {
		return "", false
	}
	return hit, true
}

// orderedFragmentNames emits env.sh, then shell.sh, then any other fragment
// basename. Apply order must not change the bytes of a block that is already
// correct.
func orderedFragmentNames(kept map[string]string) []string {
	preferred := []string{"env.sh", "shell.sh"}
	out := make([]string, 0, len(kept))
	seen := make(map[string]bool, len(kept))
	for _, name := range preferred {
		if _, ok := kept[name]; ok {
			out = append(out, name)
			seen[name] = true
		}
	}
	rest := make([]string, 0, len(kept))
	for name := range kept {
		if !seen[name] {
			rest = append(rest, name)
		}
	}
	sort.Strings(rest)
	return append(out, rest...)
}

// RcFiles returns the list of shell rc files to inject the source line into.
// It detects the user's shell from $SHELL and falls back to common defaults.
func RcFiles() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}

	shell := filepath.Base(os.Getenv("SHELL"))
	switch shell {
	case "zsh":
		return []string{filepath.Join(home, ".zshrc")}
	case "fish":
		// Fish uses its own config; we still inject a bash-compatible source
		// call via a fish wrapper. For now, skip fish and only target POSIX shells.
		return []string{filepath.Join(home, ".bashrc")}
	default:
		return []string{filepath.Join(home, ".bashrc")}
	}
}

// EnvStatusKind classifies an env variable's state vs the lock.
type EnvStatusKind string

const (
	// EnvStatusOK means the variable is in both spec and lock with the same value.
	EnvStatusOK EnvStatusKind = "ok"

	// EnvStatusModified means the variable is in both spec and lock but the values differ.
	EnvStatusModified EnvStatusKind = "modified"

	// EnvStatusMissing means the variable is in the spec but has no lock entry.
	EnvStatusMissing EnvStatusKind = "missing"

	// EnvStatusExtra means the variable is in the lock but not in the spec.
	EnvStatusExtra EnvStatusKind = "extra"
)

// EnvStatusEntry is one row in the env status report.
type EnvStatusEntry struct {
	Name      string
	Kind      EnvStatusKind
	SpecValue string // value from genv.json; empty for EnvStatusExtra
	LockValue string // value from lock; empty for EnvStatusMissing
	Sensitive bool
}

// EnvStatus computes the two-way diff between the spec env block and the lock
// env entries. Like package Status it compares spec vs lock — it does not
// query the live process environment.
func EnvStatus(specEnv map[string]schema.EnvVar, lockEnv []genvfile.LockedEnvVar) []EnvStatusEntry {
	lockByName := make(map[string]genvfile.LockedEnvVar, len(lockEnv))
	for _, le := range lockEnv {
		lockByName[le.Name] = le
	}
	specNames := make(map[string]bool, len(specEnv))
	for name := range specEnv {
		specNames[name] = true
	}

	// Collect names and sort for deterministic output.
	sortedSpec := make([]string, 0, len(specEnv))
	for name := range specEnv {
		sortedSpec = append(sortedSpec, name)
	}
	sort.Strings(sortedSpec)

	var entries []EnvStatusEntry

	// Spec-side pass: ok, modified, or missing.
	for _, name := range sortedSpec {
		ev := specEnv[name]
		le, inLock := lockByName[name]
		if !inLock {
			entries = append(entries, EnvStatusEntry{
				Name:      name,
				Kind:      EnvStatusMissing,
				SpecValue: ev.Value,
				Sensitive: ev.Sensitive,
			})
			continue
		}
		kind := EnvStatusOK
		if le.Value != ev.Value {
			kind = EnvStatusModified
		}
		entries = append(entries, EnvStatusEntry{
			Name:      name,
			Kind:      kind,
			SpecValue: ev.Value,
			LockValue: le.Value,
			Sensitive: ev.Sensitive || le.Sensitive,
		})
	}

	// Lock-side pass: extra entries not in spec.
	sortedLock := make([]string, 0, len(lockEnv))
	for _, le := range lockEnv {
		sortedLock = append(sortedLock, le.Name)
	}
	sort.Strings(sortedLock)
	for _, name := range sortedLock {
		if !specNames[name] {
			le := lockByName[name]
			entries = append(entries, EnvStatusEntry{
				Name:      name,
				Kind:      EnvStatusExtra,
				LockValue: le.Value,
				Sensitive: le.Sensitive,
			})
		}
	}

	return entries
}

// ApplyEnv writes the managed fragment and injects source lines.
// It returns the list of variable names that were actually written.
func ApplyEnv(fragmentPath string, vars map[string]schema.EnvVar, rcFiles []string) error {
	if err := WriteFragment(fragmentPath, vars); err != nil {
		return err
	}
	if len(vars) == 0 {
		// No variables — no need to inject source lines.
		return nil
	}
	for _, rc := range rcFiles {
		if err := InjectSourceLine(rc, fragmentPath); err != nil {
			// Non-fatal: rc injection failure should not block apply.
			_, _ = fmt.Fprintf(os.Stderr, "genv: warning: could not inject source line into %s: %v\n", rc, err)
		}
	}
	return nil
}

// ReadFragment parses the managed fragment at path and returns the exported
// variable names it sets. Used primarily in tests to verify fragment contents.
func ReadFragment(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]string{}, nil
		}
		return nil, err
	}

	result := make(map[string]string)
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "export ") {
			continue
		}
		rest := strings.TrimPrefix(line, "export ")
		eqIdx := strings.IndexByte(rest, '=')
		if eqIdx < 0 {
			continue
		}
		name := rest[:eqIdx]
		quotedVal := rest[eqIdx+1:]
		result[name] = shellUnquote(quotedVal)
	}
	return result, scanner.Err()
}

// shellQuote wraps v in single quotes, escaping any embedded single quotes
// using the 'x'\”y' idiom so the result is safe to embed in a POSIX shell script.
func shellQuote(v string) string {
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

// shellUnquote reverses shellQuote for testing purposes (single-quoted strings only).
func shellUnquote(s string) string {
	if len(s) >= 2 && s[0] == '\'' && s[len(s)-1] == '\'' {
		inner := s[1 : len(s)-1]
		return strings.ReplaceAll(inner, `'\''`, "'")
	}
	return s
}
