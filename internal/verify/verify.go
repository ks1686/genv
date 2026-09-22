package verify

import (
	"context"
	"fmt"

	"github.com/ks1686/genv/internal/adapter"
	externalpkg "github.com/ks1686/genv/internal/external"
	"github.com/ks1686/genv/internal/genvfile"
	"github.com/ks1686/genv/internal/resolver"
	"github.com/ks1686/genv/internal/schema"
)

const (
	CodeOK                 = "ok"
	CodeNotInstalled       = "package-not-installed"
	CodeUnresolved         = "package-unresolved"
	CodeManagerUnavailable = "manager-unavailable"
	CodeQueryFailed        = "query-failed"
)

// Result is the outcome of proving one tracked package is installed.
type Result struct {
	PackageID string
	Manager   string
	PkgName   string
	Version   string
	Code      string
	Message   string
	Drift     bool
}

// OK reports whether the manager confirmed the package is installed.
func (r Result) OK() bool { return r.Code == CodeOK }

// QueryFunc looks up whether pkgName is installed via manager.
type QueryFunc func(manager, pkgName string) (installed bool, version string, err error)

// Options controls Packages. Tests inject Available and Query.
type Options struct {
	Available map[string]bool
	Query     QueryFunc
}

// Packages proves each tracked package is installed via its locked or resolved
// manager. A nil lock means resolve against available managers only.
func Packages(pkgs []schema.Package, lock []genvfile.LockedPackage, opts Options) []Result {
	available := opts.Available
	if available == nil {
		available = resolver.Detect()
	}
	query := opts.Query
	if query == nil {
		query = defaultQuery
	}
	lockByID := make(map[string]genvfile.LockedPackage, len(lock))
	for _, lp := range lock {
		lockByID[lp.ID] = lp
	}
	out := make([]Result, 0, len(pkgs))
	for _, pkg := range pkgs {
		var locked *genvfile.LockedPackage
		if lp, ok := lockByID[pkg.ID]; ok {
			locked = &lp
		}
		out = append(out, one(pkg, locked, available, query))
	}
	return out
}

func one(pkg schema.Package, lp *genvfile.LockedPackage, available map[string]bool, query QueryFunc) Result {
	r := Result{PackageID: pkg.ID, PkgName: pkg.ID}
	if pkg.External != nil {
		state := externalpkg.InspectLocal(context.Background(), pkg, lp)
		r.Manager = "external"
		r.Version = state.Version
		if !state.Present {
			r.Code = CodeNotInstalled
			r.Message = fmt.Sprintf("package %q is not installed via external", pkg.ID)
			return r
		}
		r.Code = CodeOK
		r.Drift = state.Drift
		return r
	}

	manager, pkgName := "", ""
	if lp != nil && lp.Manager != "" {
		manager = lp.Manager
		pkgName = lp.PkgName
		if pkgName == "" {
			pkgName = nativeName(manager, pkg)
		}
	} else {
		action := resolver.ResolveOne(pkg, available)
		if !action.Resolved() {
			r.Code = CodeUnresolved
			r.Message = fmt.Sprintf("package %q has no usable manager on this host", pkg.ID)
			return r
		}
		manager, pkgName = action.Manager, action.PkgName
	}
	r.Manager, r.PkgName = manager, pkgName
	if !available[manager] {
		r.Code = CodeManagerUnavailable
		if lp != nil && lp.Manager != "" {
			r.Message = fmt.Sprintf("package %q is locked to %s, which is not available", pkg.ID, manager)
		} else {
			r.Message = fmt.Sprintf("package %q resolved to %s, which is not available", pkg.ID, manager)
		}
		return r
	}

	installed, version, err := query(manager, pkgName)
	if err != nil {
		r.Code = CodeQueryFailed
		r.Message = fmt.Sprintf("package %q: querying %s: %v", pkg.ID, manager, err)
		return r
	}
	if !installed {
		r.Code = CodeNotInstalled
		r.Message = fmt.Sprintf("package %q is not installed via %s", pkg.ID, manager)
		return r
	}
	r.Code = CodeOK
	r.Version = version
	return r
}

func nativeName(manager string, pkg schema.Package) string {
	if a := adapter.ByName(manager); a != nil {
		name, _ := a.NormalizeID(pkg.ID, pkg.Managers)
		return name
	}
	if name, ok := pkg.Managers[manager]; ok && name != "" {
		return name
	}
	return pkg.ID
}

func defaultQuery(manager, pkgName string) (bool, string, error) {
	a := adapter.ByName(manager)
	if a == nil {
		return false, "", fmt.Errorf("unknown manager %s", manager)
	}
	installed, err := resolver.CallTimed(func() (bool, error) {
		return a.Query(pkgName)
	}, resolver.DefaultLiveListTimeout)
	if err != nil {
		return false, "", err
	}
	if !installed {
		return false, "", nil
	}
	version, _ := resolver.CallTimed(func() (string, error) {
		return a.QueryVersion(pkgName)
	}, resolver.DefaultLiveListTimeout)
	return true, version, nil
}
