package profilebackend

import (
	genvenv "github.com/ks1686/genv/internal/env"
	"github.com/ks1686/genv/internal/schema"
	"github.com/ks1686/genv/internal/shellcfg"
)

// POSIXBackend writes env.sh / shell.sh and injects into bash/zsh rc files.
type POSIXBackend struct{}

func (POSIXBackend) Name() string { return "posix" }

func (POSIXBackend) ApplyEnv(vars map[string]schema.EnvVar) error {
	fragPath, err := genvenv.FragmentPath()
	if err != nil {
		return err
	}
	return genvenv.ApplyEnv(fragPath, vars, genvenv.RcFiles())
}

func (POSIXBackend) ApplyShell(cfg *schema.ShellConfig) error {
	fragPath, err := shellcfg.FragmentPath()
	if err != nil {
		return err
	}
	return shellcfg.ApplyShell(fragPath, cfg, genvenv.RcFiles())
}
