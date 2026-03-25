package commands

import (
	"errors"
	"fmt"
	"io"
	"sort"
	"text/tabwriter"

	"github.com/ks1686/genv/internal/schema"
)

// ErrEnvNotFound is returned when the named variable is not in the spec.
var ErrEnvNotFound = errors.New("env var not found in spec")

// EnvSet adds or updates the variable name in f's env block.
// It upgrades f.SchemaVersion to schema.Version2 if needed.
// Returns an error if name is not a valid POSIX variable name.
func EnvSet(f *schema.GenvFile, name, value string, sensitive bool) error {
	if !schema.ValidEnvName(name) {
		return fmt.Errorf("invalid variable name %q: must match [A-Za-z_][A-Za-z0-9_]*\nTip: use letters, digits, and underscores only; the name must not start with a digit", name)
	}
	if f.Env == nil {
		f.Env = make(map[string]schema.EnvVar)
	}
	f.Env[name] = schema.EnvVar{Value: value, Sensitive: sensitive}
	// Upgrade schema to v2 now that an env block is present.
	f.SchemaVersion = schema.Version2
	return nil
}

// EnvUnset removes the variable name from f's env block.
// Returns ErrEnvNotFound when name is absent.
func EnvUnset(f *schema.GenvFile, name string) error {
	if _, ok := f.Env[name]; !ok {
		return fmt.Errorf("%w: %q\nTip: run 'genv env list' to see declared variables", ErrEnvNotFound, name)
	}
	delete(f.Env, name)
	// If env block is now empty and we were at v2, stay at v2 for forward compat.
	return nil
}

// EnvList writes a tabular listing of all declared env variables to w.
// Sensitive values are shown as [redacted].
func EnvList(f *schema.GenvFile, w io.Writer) {
	if len(f.Env) == 0 {
		_, _ = fmt.Fprintln(w, "no env variables declared.")
		return
	}

	names := make([]string, 0, len(f.Env))
	for name := range f.Env {
		names = append(names, name)
	}
	sort.Strings(names)

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "NAME\tVALUE\tSENSITIVE")
	for _, name := range names {
		ev := f.Env[name]
		sensitive := ""
		if ev.Sensitive {
			sensitive = "yes"
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\n", name, RedactValue(ev.Value, ev.Sensitive), sensitive)
	}
	_ = tw.Flush()
}
