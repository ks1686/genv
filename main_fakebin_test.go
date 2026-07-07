package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// fakeManagerScript emulates just enough of each real package manager's
// exact command grammar (as constructed by internal/adapter) for the
// top-level CLI tests in main_test.go: mutating commands (install/uninstall/
// upgrade/clean) succeed instantly, and query/list commands report "not
// installed" / "nothing installed" by default. It is installed under
// multiple names and dispatches on argv[0]'s basename.
//
// Without this, main_test.go's tests shell out to the real system package
// manager (see internal/adapter's PlanInstall/PlanUninstall/PlanClean),
// which is slow and mutates the developer's actual machine — e.g.
// TestDisownCmd_Basic really invoked `brew install neovim`.
const fakeManagerScript = `#!/bin/sh
me=$(basename "$0")

case "$me" in
sudo)
	# Only pacman's mutating commands are sudo-prefixed; just succeed.
	exit 0
	;;
brew)
	case "$1" in
	install|uninstall|upgrade|cleanup|search)
		exit 0
		;;
	list)
		case "$*" in
		*--versions*) exit 0 ;;
		*-1*) exit 0 ;;
		*) exit 1 ;;
		esac
		;;
	esac
	exit 0
	;;
pacman|paru|yay)
	case "$1" in
	-S|-Rns|-Rcs|-Sc|-Ss)
		exit 0
		;;
	-Qi|-Q)
		exit 1
		;;
	-Qq|-Qqe)
		exit 0
		;;
	esac
	exit 0
	;;
snap)
	case "$1" in
	install|remove|refresh|list)
		exit 0
		;;
	esac
	exit 0
	;;
bun)
	case "$1" in
	add|remove|update|pm)
		exit 0
		;;
	esac
	exit 0
	;;
uv)
	case "$1 $2" in
	"tool install"|"tool uninstall"|"tool upgrade"|"tool list")
		exit 0
		;;
	esac
	exit 0
	;;
esac
exit 0
`

// installFakeManagers writes fakeManagerScript under every package-manager
// name that main.go's adapters know how to invoke, but only for names
// already resolvable on the real PATH — so which managers report as
// "available" during tests is unchanged from before; only their execution
// is redirected away from the real system. It prepends the fake directory
// to PATH so the fakes shadow the real binaries.
func installFakeManagers() error {
	names := []string{"brew", "pacman", "paru", "yay", "snap", "bun", "uv", "sudo"}

	dir, err := os.MkdirTemp("", "genv-test-fakebin-*")
	if err != nil {
		return fmt.Errorf("installFakeManagers: %w", err)
	}

	var shadowed int
	for _, name := range names {
		if _, err := exec.LookPath(name); err != nil {
			continue // not really available on this host; don't fabricate it
		}
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(fakeManagerScript), 0o755); err != nil {
			return fmt.Errorf("installFakeManagers: writing %s: %w", name, err)
		}
		shadowed++
	}
	if shadowed == 0 {
		return nil
	}

	return os.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}
