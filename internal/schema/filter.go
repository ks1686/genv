package schema

import "strings"

// DropInapplicable removes desired-state entries that cannot apply on goos.
// Homebrew env vars are macOS-only. On Windows, only powershell-targeted
// aliases/functions are kept; unscoped and POSIX shell entries are dropped
// so they do not show up as missing.
func DropInapplicable(f *GenvFile, goos string) *GenvFile {
	if f == nil {
		return nil
	}
	out := *f
	if goos != "darwin" && len(f.Env) > 0 {
		env := make(map[string]EnvVar, len(f.Env))
		for k, v := range f.Env {
			if strings.HasPrefix(k, "HOMEBREW_") {
				continue
			}
			env[k] = v
		}
		if len(env) == 0 {
			env = nil
		}
		out.Env = env
	}
	if goos == "windows" && f.Shell != nil {
		aliases := make(map[string]ShellAlias)
		for k, a := range f.Shell.Aliases {
			if a.Shell == "powershell" {
				aliases[k] = a
			}
		}
		funcs := make(map[string]ShellFunction)
		for k, fn := range f.Shell.Functions {
			if fn.Shell == "powershell" {
				funcs[k] = fn
			}
		}
		shell := *f.Shell
		shell.Aliases = aliases
		shell.Functions = funcs
		out.Shell = normalizeShell(&shell)
	}
	return &out
}
