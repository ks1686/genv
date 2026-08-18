package lockgate

import (
	"fmt"

	"github.com/ks1686/genv/internal/genvfile"
)

// Decision describes whether a lock file was written for a different host.
type Decision struct {
	Foreign     bool
	Reason      string
	Unavailable []string
}

// Check decides whether lf belongs to a different target or OS.
// Unavailable managers on a matching host are listed in Unavailable rather
// than treating the whole lock as foreign.
func Check(lf *genvfile.LockFile, activeTarget, goos string, available map[string]bool) Decision {
	if lf == nil || (len(lf.Packages) == 0 && lf.Target == "" && lf.GOOS == "") {
		return Decision{}
	}
	if lf.Target != "" && lf.Target != activeTarget {
		return Decision{
			Foreign: true,
			Reason:  fmt.Sprintf("lock target %q does not match active target %q", lf.Target, activeTarget),
		}
	}
	if lf.GOOS != "" && lf.GOOS != goos {
		return Decision{
			Foreign: true,
			Reason:  fmt.Sprintf("lock goos %q does not match active goos %q", lf.GOOS, goos),
		}
	}
	var unavailable []string
	seen := map[string]bool{}
	for _, pkg := range lf.Packages {
		if pkg.Manager == "" || available[pkg.Manager] || seen[pkg.Manager] {
			continue
		}
		seen[pkg.Manager] = true
		unavailable = append(unavailable, pkg.Manager)
	}
	return Decision{Unavailable: unavailable}
}

// CheckStrict is Check plus treating a non-empty lock that lacks target/GOOS
// metadata as foreign. Use this on schema v8 mutation paths.
func CheckStrict(lf *genvfile.LockFile, activeTarget, goos string, available map[string]bool) Decision {
	if lf != nil && len(lf.Packages) > 0 && (lf.Target == "" || lf.GOOS == "") {
		return Decision{
			Foreign: true,
			Reason:  "lock is missing target/goos metadata",
		}
	}
	return Check(lf, activeTarget, goos, available)
}
