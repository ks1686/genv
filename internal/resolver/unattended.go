package resolver

import (
	"strings"

	"github.com/ks1686/genv/internal/genvfile"
)

const unattendedElevationReason = "requires elevation (unattended)"

// canElevateWithoutPrompt reports whether this process can already perform
// privileged package actions without a GUI/UAC/sudo password prompt.
// Tests replace it.
var canElevateWithoutPrompt = defaultCanElevateWithoutPrompt

func defaultCanElevateWithoutPrompt() bool {
	if processElevated() {
		return true
	}
	return sudoNoninteractiveOK()
}

// withNoninteractiveSudo inserts `sudo -n` so a scheduled run cannot wait on a password.
func withNoninteractiveSudo(argv []string) []string {
	if len(argv) == 0 || argv[0] != "sudo" {
		return argv
	}
	for _, arg := range argv[1:] {
		if arg == "-n" || arg == "--non-interactive" {
			return argv
		}
		if arg == "--" {
			break
		}
	}
	out := make([]string, 0, len(argv)+1)
	out = append(out, "sudo", "-n")
	out = append(out, argv[1:]...)
	return out
}

func commandNeedsElevation(argv []string) bool {
	if len(argv) == 0 {
		return false
	}
	switch argv[0] {
	case "sudo":
		return true
	case "choco":
		return chocoMutating(argv)
	case "winget":
		return wingetMutating(argv)
	case "paru", "yay":
		return true
	default:
		return false
	}
}

func wingetMutating(argv []string) bool {
	for _, arg := range argv[1:] {
		switch arg {
		case "upgrade", "install", "uninstall":
			return true
		case "source", "list", "search", "show", "export":
			return false
		}
	}
	return false
}

func chocoMutating(argv []string) bool {
	for _, arg := range argv[1:] {
		switch arg {
		case "upgrade", "install", "uninstall", "update":
			return true
		}
	}
	return false
}

func skipUnattendedElevation(unattended bool, argv []string) bool {
	if !unattended {
		return false
	}
	argv = withNoninteractiveSudo(argv)
	return commandNeedsElevation(argv) && !canElevateWithoutPrompt()
}

func skippedElevation(lps []genvfile.LockedPackage, argv []string) []SkippedPackage {
	reason := unattendedElevationReason
	if len(argv) > 0 {
		reason = unattendedElevationReason + ": " + argv[0]
	}
	out := make([]SkippedPackage, 0, len(lps))
	for _, lp := range lps {
		manager := lp.Manager
		if manager == "" && len(argv) > 0 {
			manager = argv[0]
		}
		out = append(out, SkippedPackage{ID: lp.ID, Manager: manager, Reason: reason})
	}
	if len(out) == 0 && len(argv) > 0 {
		out = append(out, SkippedPackage{Manager: argv[0], Reason: reason})
	}
	return out
}

func refreshSkipWarning(managers []string) string {
	return "skipping refresh for " + strings.Join(managers, ",") + ": " + unattendedElevationReason
}
