package profilebackend

import (
	"path/filepath"

	genvenv "github.com/ks1686/genv/internal/env"
	"github.com/ks1686/genv/internal/schema"
	"github.com/ks1686/genv/internal/shellcfg"
)

// POSIXBackend writes env.sh / shell.sh and injects into bash/zsh rc files.
type POSIXBackend struct {
	// Dir is the state directory for fragments. Empty uses the default config dir.
	Dir string
}

func (POSIXBackend) Name() string { return "posix" }

func (b POSIXBackend) ApplyEnv(vars map[string]schema.EnvVar) error {
	fragPath, err := b.envFragmentPath()
	if err != nil {
		return err
	}
	return genvenv.ApplyEnv(fragPath, vars, b.rcFiles())
}

func (b POSIXBackend) ApplyShell(cfg *schema.ShellConfig) error {
	fragPath, err := b.shellFragmentPath()
	if err != nil {
		return err
	}
	return shellcfg.ApplyShell(fragPath, cfg, b.rcFiles())
}

func (b POSIXBackend) envFragmentPath() (string, error) {
	if b.Dir != "" {
		return filepath.Join(b.Dir, "env.sh"), nil
	}
	return genvenv.FragmentPath()
}

func (b POSIXBackend) shellFragmentPath() (string, error) {
	if b.Dir != "" {
		return filepath.Join(b.Dir, "shell.sh"), nil
	}
	return shellcfg.FragmentPath()
}

func (b POSIXBackend) rcFiles() []string {
	if !shouldInjectRC(b.Dir) {
		return nil
	}
	return genvenv.RcFiles()
}
