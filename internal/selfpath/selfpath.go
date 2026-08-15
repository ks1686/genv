// Package selfpath chooses an invocation path for the running binary that
// survives package-manager upgrades when a stable symlink is available.
package selfpath

import (
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
)

// PreferStable returns a path that refers to the same file as exe when possible,
// preferring a PATH hit for filepath.Base(exe) over a version-pinned install
// path (e.g. Homebrew Caskroom/.../<version>/genv used by cask post_install).
//
// When exe lives under Homebrew Caskroom/Cellar versioned layout, the brew
// prefix bin/<name> path is preferred even if that symlink is missing or
// briefly dangling mid-upgrade (SameFile would fail in that window).
//
// lookPath defaults to exec.LookPath when nil. If no stable candidate exists,
// exe is returned unchanged.
func PreferStable(exe, arg0 string, lookPath func(string) (string, error)) string {
	if exe == "" {
		return exe
	}
	if lookPath == nil {
		lookPath = exec.LookPath
	}
	base := filepath.Base(exe)
	if base != "" && base != "." {
		if p, err := lookPath(base); err == nil && sameFile(p, exe) {
			return p
		}
	}
	if stable := brewStableBin(exe); stable != "" {
		return stable
	}
	if arg0 != "" && arg0 != exe {
		cand := arg0
		if !filepath.IsAbs(cand) {
			p, err := lookPath(cand)
			if err != nil {
				return exe
			}
			cand = p
		}
		if sameFile(cand, exe) {
			return cand
		}
	}
	return exe
}

// brewStableBin maps Homebrew versioned install paths to <prefix>/bin/<name>.
// Returns "" when exe is not under Caskroom or Cellar.
func brewStableBin(exe string) string {
	// Homebrew layouts are POSIX. Normalize both separators so unit tests on
	// Windows and a Windows-style string on Unix still match Caskroom/Cellar.
	slash := path.Clean(strings.ReplaceAll(exe, "\\", "/"))
	base := path.Base(slash)
	if base == "" || base == "." {
		return ""
	}
	for _, marker := range []string{"/Caskroom/", "/Cellar/"} {
		idx := strings.Index(slash, marker)
		if idx < 0 {
			continue
		}
		prefix := slash[:idx]
		if prefix == "" {
			continue
		}
		out := prefix + "/bin/" + base
		if strings.Contains(exe, "\\") && !strings.Contains(exe, "/") {
			return filepath.FromSlash(out)
		}
		return out
	}
	return ""
}

func sameFile(a, b string) bool {
	fa, err := os.Stat(a)
	if err != nil {
		return false
	}
	fb, err := os.Stat(b)
	if err != nil {
		return false
	}
	return os.SameFile(fa, fb)
}
