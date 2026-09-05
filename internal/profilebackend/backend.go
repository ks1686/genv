package profilebackend

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/ks1686/genv/internal/genvfile"
	"github.com/ks1686/genv/internal/schema"
)

// Backend applies env and shell fragments for one profile family.
type Backend interface {
	Name() string
	ApplyEnv(vars map[string]schema.EnvVar) error
	ApplyShell(cfg *schema.ShellConfig) error
}

// SelectBackends returns the profile backends that should run for goos.
//
// Non-Windows: POSIX only (never writes .ps1).
// Windows with a PowerShell engine: PowerShell always; POSIX only when a POSIX
// shell/rc is already relevant.
// Windows without an engine: POSIX only when relevant; callers should warn
// about the missing engine separately.
// SelectBackendsIn is SelectBackends with fragments rooted at stateDir.
// An empty stateDir keeps the default config directory.
func SelectBackendsIn(goos, stateDir string) []Backend {
	backends := SelectBackends(goos)
	if stateDir == "" {
		return backends
	}
	out := make([]Backend, len(backends))
	for i, b := range backends {
		switch t := b.(type) {
		case POSIXBackend:
			t.Dir = stateDir
			out[i] = t
		case PowerShellBackend:
			t.Dir = stateDir
			out[i] = t
		default:
			out[i] = b
		}
	}
	return out
}

func shouldInjectRC(stateDir string) bool {
	if stateDir == "" {
		return true
	}
	def, err := genvfile.DefaultDir()
	if err != nil {
		return false
	}
	return genvfile.WithinDir(def, stateDir) && genvfile.WithinDir(stateDir, def)
}

func SelectBackends(goos string) []Backend {
	if goos != "windows" {
		return []Backend{POSIXBackend{}}
	}
	_, hasEngine := DetectEngine()
	var out []Backend
	if hasEngine {
		out = append(out, PowerShellBackend{})
	}
	if PosixRelevant() {
		out = append(out, POSIXBackend{})
	}
	return out
}

// MissingEngineWarning returns a user-facing warning when Windows apply cannot
// use a PowerShell backend, or "" when no warning is needed.
func MissingEngineWarning(goos string) string {
	if goos != "windows" {
		return ""
	}
	if _, ok := DetectEngine(); ok {
		return ""
	}
	return "no PowerShell engine (pwsh or powershell) found on PATH; skipping PowerShell env/shell profiles"
}

// PosixRelevant reports whether a POSIX shell/rc is already in play on this host
// (so Windows apply should also maintain env.sh / shell.sh).
func PosixRelevant() bool {
	shell := strings.ToLower(filepath.Base(os.Getenv("SHELL")))
	switch shell {
	case "bash", "zsh", "fish", "sh":
		return true
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return false
	}
	for _, name := range []string{".bashrc", ".zshrc", ".profile"} {
		if _, err := os.Stat(filepath.Join(home, name)); err == nil {
			return true
		}
	}
	return false
}
