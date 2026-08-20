package commands

import (
	"strings"

	"github.com/ks1686/genv/internal/adapter"
	"github.com/ks1686/genv/internal/genvfile"
	"github.com/ks1686/genv/internal/schema"
	"github.com/ks1686/genv/internal/version"
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

	// StatusMissing means the package is in the spec but has no lock entry and
	// is not installed on the live system (run 'genv apply').
	StatusMissing StatusKind = "missing"

	// StatusPresent means the package is in the spec, not in the lock, but
	// already installed via its manager. Apply will adopt it without installing.
	StatusPresent StatusKind = "present"

	// StatusExtra means the package is in the lock but not in the spec —
	// it was removed from the spec without being uninstalled (run 'genv apply').
	StatusExtra StatusKind = "extra"
)

// StatusEntry is one row in the status report.
type StatusEntry struct {
	ID               string
	Manager          string // empty for StatusMissing entries
	PkgName          string
	Kind             StatusKind
	SpecVersion      string // constraint from genv.json, may be empty
	InstalledVersion string // recorded version from lock, may be empty
}

// Status computes the three-way diff between the spec (genv.json) and the lock
// file (genv.lock.json). It does not query the live system — the lock file is
// the record of what genv last installed.
func Status(f *schema.GenvFile, lf *genvfile.LockFile) []StatusEntry {
	return StatusWithLive(f, lf, nil)
}

// StatusWithLive is Status plus a live inventory (manager → native names).
// Unlocked spec packages that are live-installed are StatusPresent.
func StatusWithLive(f *schema.GenvFile, lf *genvfile.LockFile, live map[string]map[string]bool) []StatusEntry {
	lockByID := make(map[string]genvfile.LockedPackage, len(lf.Packages))
	for _, lp := range lf.Packages {
		lockByID[lp.ID] = lp
	}
	specByID := make(map[string]bool, len(f.Packages))
	for _, pkg := range f.Packages {
		specByID[pkg.ID] = true
	}

	var entries []StatusEntry

	for _, pkg := range f.Packages {
		lp, inLock := lockByID[pkg.ID]
		if !inLock {
			if mgr, name, ok := liveMatch(pkg, live); ok {
				entries = append(entries, StatusEntry{
					ID:          pkg.ID,
					Manager:     mgr,
					PkgName:     name,
					Kind:        StatusPresent,
					SpecVersion: pkg.Version,
				})
				continue
			}
			entries = append(entries, StatusEntry{
				ID:          pkg.ID,
				Kind:        StatusMissing,
				SpecVersion: pkg.Version,
			})
			continue
		}
		kind := StatusOK
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

func liveMatch(pkg schema.Package, live map[string]map[string]bool) (manager, pkgName string, ok bool) {
	if live == nil {
		return "", "", false
	}
	try := func(mgr string) (string, string, bool) {
		if mgr == "" {
			return "", "", false
		}
		name := pkg.ID
		if a := adapter.ByName(mgr); a != nil {
			name, _ = a.NormalizeID(pkg.ID, pkg.Managers)
		} else if n, exists := pkg.Managers[mgr]; exists {
			name = n
		}
		if liveHas(live, mgr, name) {
			return mgr, name, true
		}
		return "", "", false
	}
	if mgr, name, ok := try(pkg.Prefer); ok {
		return mgr, name, true
	}
	for mgr := range pkg.Managers {
		if m, name, ok := try(mgr); ok {
			return m, name, true
		}
	}
	return "", "", false
}

func liveHas(live map[string]map[string]bool, manager, pkgName string) bool {
	names := live[manager]
	if names[pkgName] {
		return true
	}
	for n := range names {
		if strings.EqualFold(n, pkgName) {
			return true
		}
	}
	return false
}
