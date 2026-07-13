package upgrade

import (
	"context"
	"fmt"
	"io"

	"github.com/ks1686/genv/internal/genvfile"
	"github.com/ks1686/genv/internal/output"
	"github.com/ks1686/genv/internal/resolver"
	"github.com/ks1686/genv/internal/schema"
)

type UpgradeOptions struct {
	Spec    *schema.GenvFile
	Lock    *genvfile.LockFile
	Filters output.UpgradeFilters
}

type UpgradePlan struct {
	Actions  []resolver.UpgradeAction
	Skipped  []resolver.SkippedPackage
	Warnings []string
}

type Options = UpgradeOptions
type Plan = UpgradePlan

type UpgradeRunOptions struct {
	Plan     UpgradePlan
	Lock     *genvfile.LockFile
	LockPath string
	Stdin    io.Reader
	Stdout   io.Writer
	Stderr   io.Writer
}

type UpgradeRunResult struct {
	Plan           UpgradePlan
	Upgraded       []genvfile.LockedPackage
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
	var skippedByConstraint []resolver.SkippedPackage
	for _, lp := range filteredPackages {
		if packagesByID[lp.ID].Version != "" {
			skippedByConstraint = append(skippedByConstraint, resolver.SkippedPackage{
				ID:      lp.ID,
				Manager: lp.Manager,
				Reason:  "version-constrained package requires an explicit compatible target",
			})
			continue
		}
		upgradeablePackages = append(upgradeablePackages, lp)
	}

	actions, skipped := resolver.PlanUpgrade(upgradeablePackages)
	plan.Actions = actions
	plan.Skipped = append(skipped, skippedByFilter...)
	plan.Skipped = append(plan.Skipped, skippedByConstraint...)

	return plan, nil
}

func RunUpgrade(ctx context.Context, opts UpgradeRunOptions) UpgradeRunResult {
	execResult := resolver.ExecuteUpgrade(ctx, opts.Plan.Actions, opts.Stdin, opts.Stdout, opts.Stderr)
	applyUpgradedVersions(opts.Lock, execResult.Upgraded)

	result := UpgradeRunResult{
		Plan:     opts.Plan,
		Upgraded: execResult.Upgraded,
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
		}
	}
}
