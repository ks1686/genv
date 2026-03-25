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

// InjectSourceLine ensures that fragmentPath is sourced exactly once in rcPath.
// The source line is appended only if no line in rcPath already contains
// fragmentPath as a substring. If rcPath does not exist it is created.
func InjectSourceLine(rcPath, fragmentPath string) error {
	sourceLine := ". " + fragmentPath

	// Read existing contents to check for duplicate.
	data, err := os.ReadFile(rcPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("reading %s: %w", rcPath, err)
	}
	if strings.Contains(string(data), fragmentPath) {
		// Already sourced.
		return nil
	}

	dir := filepath.Dir(rcPath)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("creating directory %s: %w", dir, err)
	}

	f, err := os.OpenFile(rcPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("opening %s: %w", rcPath, err)
	}
	defer f.Close()

	_, err = fmt.Fprintf(f, "\n# genv env\n%s\n", sourceLine)
	return err
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
	return "'" + strings.ReplaceAll(v, "'", `'\''`) + "'"
}

// shellUnquote reverses shellQuote for testing purposes (single-quoted strings only).
func shellUnquote(s string) string {
	if len(s) >= 2 && s[0] == '\'' && s[len(s)-1] == '\'' {
		inner := s[1 : len(s)-1]
		return strings.ReplaceAll(inner, `'\''`, "'")
	}
	return s
}
