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
	// Raise schema to at least v3 without downgrading a newer file.
	f.SchemaVersion = schema.AtLeastVersion(f.SchemaVersion, schema.Version3)
}

// ShellAliasSet adds or updates the alias name in f's shell block.
// Shell target may be "bash", "zsh", "fish", "powershell", or "" (POSIX).
func ShellAliasSet(f *schema.GenvFile, name, value, shell, targetID string) error {
	if name == "" {
		return fmt.Errorf("alias name must not be empty\nTip: provide a valid shell identifier as NAME")
	}
	if shell != "" && !schema.KnownShellTargets[shell] {
		return fmt.Errorf("unknown shell %q; expected %s", shell, schema.ValidShellTargetsMsg)
	}
	if f.SchemaVersion == schema.Version8 {
		bundle, err := ActiveBundle(f, targetID)
		if err != nil {
			return err
		}
		if bundle.Shell == nil {
			bundle.Shell = &schema.TargetShellConfig{}
		}
		if bundle.Shell.Aliases == nil {
			bundle.Shell.Aliases = make(map[string]*schema.ShellAlias)
		}
		alias := schema.ShellAlias{Value: value, Shell: shell}
		bundle.Shell.Aliases[name] = &alias
		return nil
	}
	ensureShell(f)
	if shell == "powershell" {
		f.SchemaVersion = schema.AtLeastVersion(f.SchemaVersion, schema.Version7)
	}
	if f.Shell.Aliases == nil {
		f.Shell.Aliases = make(map[string]schema.ShellAlias)
	}
	f.Shell.Aliases[name] = schema.ShellAlias{Value: value, Shell: shell}
	return nil
}

// ShellAliasUnset removes the alias name from f's shell block.
// Returns ErrShellAliasNotFound when name is absent.
func ShellAliasUnset(f *schema.GenvFile, name, targetID string) error {
	if f.SchemaVersion == schema.Version8 {
		bundle, err := ActiveBundle(f, targetID)
		if err != nil {
			return err
		}
		if bundle.Shell != nil && bundle.Shell.Aliases != nil {
			if _, ok := bundle.Shell.Aliases[name]; ok {
				if defaultAliasExists(f.Defaults, name) {
					bundle.Shell.Aliases[name] = nil
				} else {
					delete(bundle.Shell.Aliases, name)
				}
				return nil
			}
		}
		if defaultAliasExists(f.Defaults, name) {
			if bundle.Shell == nil {
				bundle.Shell = &schema.TargetShellConfig{}
			}
			if bundle.Shell.Aliases == nil {
				bundle.Shell.Aliases = make(map[string]*schema.ShellAlias)
			}
			bundle.Shell.Aliases[name] = nil
			return nil
		}
		return fmt.Errorf("%w: %q\nTip: run 'genv shell status' to see declared aliases", ErrShellAliasNotFound, name)
	}
	if f.Shell == nil {
		return fmt.Errorf("%w: %q\nTip: run 'genv shell alias set' to declare aliases", ErrShellAliasNotFound, name)
	}
	if _, ok := f.Shell.Aliases[name]; !ok {
		return fmt.Errorf("%w: %q\nTip: run 'genv shell status' to see declared aliases", ErrShellAliasNotFound, name)
	}
	delete(f.Shell.Aliases, name)
	return nil
}
