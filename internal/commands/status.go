package commands

import (
	"github.com/ks1686/gpm/internal/gpmfile"
	"github.com/ks1686/gpm/internal/schema"
	"github.com/ks1686/gpm/internal/version"
)

// StatusKind classifies a package's current state relative to the spec and lock.
type StatusKind string

const (
	// StatusOK means the package is in both spec and lock, and the installed
	// version satisfies the spec version constraint (or no constraint is set).
	StatusOK StatusKind = "ok"

	// StatusDrift means the package is in both spec and lock, but the locked
	// InstalledVersion does not satisfy the spec version constraint.
	StatusDrift StatusKind = "drift"

	// StatusMissing means the package is in the spec but has no lock entry —
	// it has never been installed by gpm (run 'gpm apply').
	StatusMissing StatusKind = "missing"

	// StatusExtra means the package is in the lock but not in the spec —
	// it was removed from the spec without being uninstalled (run 'gpm apply').
	StatusExtra StatusKind = "extra"
)

// StatusEntry is one row in the status report.
type StatusEntry struct {
	ID               string
	Manager          string // empty for StatusMissing entries
	PkgName          string
	Kind             StatusKind
	SpecVersion      string // constraint from gpm.json, may be empty
	InstalledVersion string // recorded version from lock, may be empty
}

// Status computes the three-way diff between the spec (gpm.json) and the lock
// file (gpm.lock.json). It does not query the live system — the lock file is
// gpm's record of what it last installed.
//
// Categories:
//   - ok:      in spec and lock, version constraint satisfied (or unconstrained)
//   - drift:   in spec and lock, but InstalledVersion fails the spec constraint
//   - missing: in spec only (gpm apply needed)
//   - extra:   in lock only (removed from spec without gpm remove / gpm apply)
func Status(f *schema.GpmFile, lf *gpmfile.LockFile) []StatusEntry {
	lockByID := make(map[string]gpmfile.LockedPackage, len(lf.Packages))
	for _, lp := range lf.Packages {
		lockByID[lp.ID] = lp
	}
	specByID := make(map[string]bool, len(f.Packages))
	for _, pkg := range f.Packages {
		specByID[pkg.ID] = true
	}

	var entries []StatusEntry

	// Spec-side pass: ok, drift, or missing.
	for _, pkg := range f.Packages {
		lp, inLock := lockByID[pkg.ID]
		if !inLock {
			entries = append(entries, StatusEntry{
				ID:          pkg.ID,
				Kind:        StatusMissing,
				SpecVersion: pkg.Version,
			})
			continue
		}
		kind := StatusOK
		// Only report drift when an installed version is actually recorded.
		if lp.InstalledVersion != "" && !version.Satisfies(pkg.Version, lp.InstalledVersion) {
			kind = StatusDrift
		}
		entries = append(entries, StatusEntry{
			ID:               pkg.ID,
			Manager:          lp.Manager,
			PkgName:          lp.PkgName,
			Kind:             kind,
			SpecVersion:      pkg.Version,
			InstalledVersion: lp.InstalledVersion,
		})
	}

	// Lock-side pass: extra entries not in spec.
	for _, lp := range lf.Packages {
		if !specByID[lp.ID] {
			entries = append(entries, StatusEntry{
				ID:               lp.ID,
				Manager:          lp.Manager,
				PkgName:          lp.PkgName,
				Kind:             StatusExtra,
				InstalledVersion: lp.InstalledVersion,
			})
		}
	}

	return entries
}
