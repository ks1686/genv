package commands

import (
	"errors"
	"fmt"

	"github.com/ks1686/genv/internal/schema"
)

// ErrShellAliasNotFound is returned when the named alias is not in the spec.
var ErrShellAliasNotFound = errors.New("alias not found in spec")

// ensureShell ensures f has a non-nil Shell block and upgrades to schema v3.
func ensureShell(f *schema.GenvFile) {
	if f.Shell == nil {
		f.Shell = &schema.ShellConfig{}
	}
	f.SchemaVersion = schema.Version3
}

// ShellAliasSet adds or updates the alias name in f's shell block.
// Shell target may be "bash", "zsh", "fish", or "" (all).
func ShellAliasSet(f *schema.GenvFile, name, value, shell string) error {
	if name == "" {
		return fmt.Errorf("alias name must not be empty\nTip: provide a valid shell identifier as NAME")
	}
	if shell != "" && !schema.KnownShellTargets[shell] {
		return fmt.Errorf("unknown shell %q; expected %s", shell, schema.ValidShellTargetsMsg)
	}
	ensureShell(f)
	if f.Shell.Aliases == nil {
		f.Shell.Aliases = make(map[string]schema.ShellAlias)
	}
	f.Shell.Aliases[name] = schema.ShellAlias{Value: value, Shell: shell}
	return nil
}

// ShellAliasUnset removes the alias name from f's shell block.
// Returns ErrShellAliasNotFound when name is absent.
func ShellAliasUnset(f *schema.GenvFile, name string) error {
	if f.Shell == nil {
		return fmt.Errorf("%w: %q\nTip: run 'genv shell alias set' to declare aliases", ErrShellAliasNotFound, name)
	}
	if _, ok := f.Shell.Aliases[name]; !ok {
		return fmt.Errorf("%w: %q\nTip: run 'genv shell status' to see declared aliases", ErrShellAliasNotFound, name)
	}
	delete(f.Shell.Aliases, name)
	return nil
}
