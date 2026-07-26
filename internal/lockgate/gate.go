package lockgate

import (
	"fmt"

	"github.com/ks1686/genv/internal/genvfile"
)

// Decision describes whether a lock file was written for a different host.
type Decision struct {
	Foreign bool
	Reason  string
}

// Check decides whether lf belongs to a different target, OS, or manager set.
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
	for _, pkg := range lf.Packages {
		if pkg.Manager == "" {
			continue
		}
		if !available[pkg.Manager] {
			return Decision{
				Foreign: true,
				Reason:  fmt.Sprintf("lock package %q uses unavailable manager %q", pkg.ID, pkg.Manager),
			}
		}
	}
	return Decision{}
}
