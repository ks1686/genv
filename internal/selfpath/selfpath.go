// Package selfpath chooses an invocation path for the running binary that
// survives package-manager upgrades when a stable symlink is available.
package selfpath

import (
	"os"
	"os/exec"
	"path/filepath"
)

// PreferStable returns a path that refers to the same file as exe when possible,
// preferring a PATH hit for filepath.Base(exe) over a version-pinned install
// path (e.g. Homebrew Caskroom/.../<version>/genv used by cask post_install).
//
// lookPath defaults to exec.LookPath when nil. If no same-file PATH candidate
// exists, exe is returned unchanged.
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
