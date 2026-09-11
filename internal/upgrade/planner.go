package upgrade

import (
	"context"
	"fmt"
	"io"

	externalpkg "github.com/ks1686/genv/internal/external"
	"github.com/ks1686/genv/internal/genvfile"
	"github.com/ks1686/genv/internal/output"
	"github.com/ks1686/genv/internal/resolver"
	"github.com/ks1686/genv/internal/schema"
	"github.com/ks1686/genv/internal/version"
)

type UpgradeOptions struct {
	Spec *schema.GenvFile
	Lock *genvfile.LockFile
	// Filters control which packages are planned. When Filters.All is false
	// (the default), the plan is narrowed to packages with an update available
	// via each manager's OutdatedLister. Filters.All true restores brute-force
	// planning of every unconstrained tracked package (`genv upgrade --all`).
	Filters output.UpgradeFilters
	// Context bounds index refresh (and is reserved for later planner work).
	// Nil means context.Background. The hourly worker passes its job budget.
	Context context.Context
	// Stdin is forwarded to refresh commands so interactive sudo can prompt.
	// The hourly worker passes an empty reader.
	Stdin io.Reader
	// Unattended is set only by updates __run-once. Refresh and apply never
	// prompt for elevation; interactive genv upgrade/apply leave this false.
	Unattended bool
}

type UpgradePlan struct {
	Refresh  []resolver.RefreshAction
	Actions  []resolver.UpgradeAction
	Skipped  []resolver.SkippedPackage
	Warnings []string
}

type Options = UpgradeOptions
type Plan = UpgradePlan

type UpgradeRunOptions struct {
	Plan                UpgradePlan
	Lock                *genvfile.LockFile
	LockPath            string
	Stdin               io.Reader
	Stdout              io.Writer
	Stderr              io.Writer
	ExternalMode        externalpkg.ExecutionMode
	AcknowledgeExternal func(message string) bool
	Unattended          bool
}

type UpgradeRunResult struct {
	Plan           UpgradePlan
	Upgraded       []genvfile.LockedPackage
	Skipped        []resolver.SkippedPackage
	Errors         []error
	Failures       []resolver.UpgradeFailure
	LockWriteError error
}

func BuildPlan(opts Options) (Plan, error) {
	return BuildUpgradePlan(opts)
}

func BuildUpgradePlan(opts UpgradeOptions) (UpgradePlan, error) {
	var plan UpgradePlan

	for _, m := range opts.Filters.OnlyManager {
		if !schema.KnownManagers[m] {
			return plan, fmt.Errorf("unknown manager %q in --only-manager", m)
		}
	}
	for _, m := range opts.Filters.SkipManager {
		if !schema.KnownManagers[m] {
			return plan, fmt.Errorf("unknown manager %q in --skip-manager", m)
		}
	}

	packagesByID := make(map[string]schema.Package, len(opts.Spec.Packages))
	for _, p := range opts.Spec.Packages {
		packagesByID[p.ID] = p
	}

	var allowedPackages []genvfile.LockedPackage
	for _, lp := range opts.Lock.Packages {
		if _, ok := packagesByID[lp.ID]; ok {
			allowedPackages = append(allowedPackages, lp)
		}
	}

	var filteredPackages []genvfile.LockedPackage
	var skippedByFilter []resolver.SkippedPackage

	onlySet := make(map[string]bool)
	for _, o := range opts.Filters.Only {
		onlySet[o] = true
	}
	skipSet := make(map[string]bool)
	for _, s := range opts.Filters.Skip {
		skipSet[s] = true
	}
	onlyMgrSet := make(map[string]bool)
	for _, m := range opts.Filters.OnlyManager {
		onlyMgrSet[m] = true
	}
	skipMgrSet := make(map[string]bool)
	for _, m := range opts.Filters.SkipManager {
		skipMgrSet[m] = true
	}

	matchedOnly := make(map[string]bool)
	matchedSkip := make(map[string]bool)

	for _, lp := range allowedPackages {
		if len(onlyMgrSet) > 0 && !onlyMgrSet[lp.Manager] {
			skippedByFilter = append(skippedByFilter, resolver.SkippedPackage{
				ID:      lp.ID,
				Manager: lp.Manager,
				Reason:  "excluded by --only-manager",
			})
			continue
		}
		if len(onlyMgrSet) == 0 && skipMgrSet[lp.Manager] {
			skippedByFilter = append(skippedByFilter, resolver.SkippedPackage{
				ID:      lp.ID,
				Manager: lp.Manager,
				Reason:  "excluded by --skip-manager",
			})
			continue
		}

		isOnly := onlySet[lp.ID] || onlySet[lp.PkgName]
		isSkip := skipSet[lp.ID] || skipSet[lp.PkgName]

		if isOnly {
			if onlySet[lp.ID] {
				matchedOnly[lp.ID] = true
			}
			if onlySet[lp.PkgName] {
				matchedOnly[lp.PkgName] = true
			}
		}
		if isSkip {
			if skipSet[lp.ID] {
				matchedSkip[lp.ID] = true
			}
			if skipSet[lp.PkgName] {
				matchedSkip[lp.PkgName] = true
			}
		}

		if len(onlySet) > 0 && !isOnly {
			skippedByFilter = append(skippedByFilter, resolver.SkippedPackage{
				ID:      lp.ID,
				Manager: lp.Manager,
				Reason:  "excluded by --only",
			})
			continue
		}
		if len(onlySet) == 0 && isSkip {
			skippedByFilter = append(skippedByFilter, resolver.SkippedPackage{
				ID:      lp.ID,
				Manager: lp.Manager,
				Reason:  "excluded by --skip",
			})
			continue
		}

		filteredPackages = append(filteredPackages, lp)
	}

	for _, o := range opts.Filters.Only {
		if !matchedOnly[o] {
			plan.Warnings = append(plan.Warnings, fmt.Sprintf("warning: --only filter %q matched no tracked packages", o))
		}
	}
	for _, s := range opts.Filters.Skip {
		if !matchedSkip[s] {
			plan.Warnings = append(plan.Warnings, fmt.Sprintf("warning: --skip filter %q matched no tracked packages", s))
		}
	}

	var upgradeablePackages []genvfile.LockedPackage
	var externalActions []resolver.UpgradeAction
	var skippedByConstraint []resolver.SkippedPackage
	for _, lp := range filteredPackages {
		pkg := packagesByID[lp.ID]
		if lp.Manager == "external" && pkg.External != nil {
			latest := ""
			if !opts.Filters.All {
				var err error
				latest, err = externalLatestVersion(context.Background(), pkg)
				if err != nil {
					plan.Warnings = append(plan.Warnings, fmt.Sprintf("could not determine external update for %s: %v", lp.ID, err))
					plan.Skipped = append(plan.Skipped, resolver.SkippedPackage{ID: lp.ID, Manager: lp.Manager, Reason: "external update check failed"})
					continue
				}
				state := externalpkg.InspectLocal(context.Background(), pkg, &lp)
				if state.Present && state.Version == latest {
					continue
				}
				if !version.Satisfies(pkg.Version, latest) {
					skippedByConstraint = append(skippedByConstraint, resolver.SkippedPackage{ID: lp.ID, Manager: lp.Manager, Reason: "latest external release does not satisfy version constraint"})
					continue
				}
			}
			pkgCopy := pkg
			externalActions = append(externalActions, resolver.UpgradeAction{LPs: []genvfile.LockedPackage{lp}, Cmd: []string{"external-release"}, External: &pkgCopy, RemoteVersion: latest})
			continue
		}
		if pkg.Version != "" {
			skippedByConstraint = append(skippedByConstraint, resolver.SkippedPackage{
				ID:      lp.ID,
				Manager: lp.Manager,
				Reason:  "version-constrained package requires an explicit compatible target",
			})
			continue
		}
		upgradeablePackages = append(upgradeablePackages, lp)
	}

	refresh, keepAll, refreshWarns := resolver.RefreshIndexes(upgradeablePackages, resolver.RefreshOptions{
		Context:    opts.Context,
		Stdin:      opts.Stdin,
		Unattended: opts.Unattended,
	})
	plan.Refresh = refresh
	plan.Warnings = append(plan.Warnings, refreshWarns...)

	if !opts.Filters.All {
		filterable := packagesForOutdated(upgradeablePackages, keepAll)
		filtered, warnings := resolver.FilterOutdated(filterable)
		upgradeablePackages = mergeAfterOutdated(upgradeablePackages, keepAll, filtered)
		plan.Warnings = append(plan.Warnings, warnings...)
	}

	actions, skipped := resolver.PlanUpgrade(upgradeablePackages)
	plan.Actions = append(externalActions, actions...)
	plan.Skipped = append(plan.Skipped, skipped...)
	plan.Skipped = append(plan.Skipped, skippedByFilter...)
	plan.Skipped = append(plan.Skipped, skippedByConstraint...)

	return plan, nil
}

func RunUpgrade(ctx context.Context, opts UpgradeRunOptions) UpgradeRunResult {
	execResult := resolver.ExecuteUpgrade(ctx, opts.Plan.Actions, opts.Stdin, opts.Stdout, opts.Stderr, resolver.ApplyExecutionOptions{
		ExternalMode: opts.ExternalMode, AcknowledgeExternal: opts.AcknowledgeExternal, Unattended: opts.Unattended,
	})
	applyUpgradedVersions(opts.Lock, execResult.Upgraded)

	result := UpgradeRunResult{
		Plan:     opts.Plan,
		Upgraded: execResult.Upgraded,
		Skipped:  append([]resolver.SkippedPackage(nil), execResult.Skipped...),
		Errors:   append([]error(nil), execResult.Errors...),
		Failures: append([]resolver.UpgradeFailure(nil), execResult.Failures...),
	}
	if opts.LockPath != "" {
		if err := genvfile.WriteLock(opts.LockPath, opts.Lock); err != nil {
			result.LockWriteError = fmt.Errorf("writing lock: %w", err)
		}
	}

	return result
}

func packagesForOutdated(packages []genvfile.LockedPackage, keepAll map[string]bool) []genvfile.LockedPackage {
	if len(keepAll) == 0 {
		return packages
	}
	var out []genvfile.LockedPackage
	for _, lp := range packages {
		if !keepAll[lp.Manager] {
			out = append(out, lp)
		}
	}
	return out
}

func mergeAfterOutdated(original []genvfile.LockedPackage, keepAll map[string]bool, filtered []genvfile.LockedPackage) []genvfile.LockedPackage {
	kept := make(map[string]bool, len(filtered))
	for _, lp := range filtered {
		kept[lp.ID] = true
	}
	var out []genvfile.LockedPackage
	for _, lp := range original {
		if keepAll[lp.Manager] || kept[lp.ID] {
			out = append(out, lp)
		}
	}
	return out
}

var externalLatestVersion = func(ctx context.Context, pkg schema.Package) (string, error) {
	return (externalpkg.Engine{Host: externalpkg.CurrentHost()}).LatestVersion(ctx, pkg)
}

func applyUpgradedVersions(lf *genvfile.LockFile, upgraded []genvfile.LockedPackage) {
	if lf == nil {
		return
	}
	lockIndex := make(map[string]int, len(lf.Packages))
	for i, lp := range lf.Packages {
		lockIndex[lp.ID] = i
	}
	for _, u := range upgraded {
		if idx, ok := lockIndex[u.ID]; ok {
			lf.Packages[idx].InstalledVersion = u.InstalledVersion
			if u.External != nil {
				lf.Packages[idx].External = u.External
			}
		}
	}
}
