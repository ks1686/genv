package export

import (
	"fmt"
	"sort"
	"strings"

	"github.com/ks1686/genv/internal/schema"
)

// Suggest returns read-only mapping suggestions for packages that do not have a
// usable manager entry on destTarget.
func Suggest(f *schema.GenvFile, destTarget string) []ReportItem {
	destTarget = strings.TrimSpace(destTarget)
	var report Report
	if destTarget == "" {
		return Report{{
			Class:   ClassError,
			Code:    "target-required",
			Message: "destination target is required",
		}}
	}
	if f == nil {
		return Report{{
			Class:   ClassError,
			Code:    "file-empty",
			Message: "genv file is nil",
		}}
	}

	allowed := managerAllowlist(destTarget)
	if f.Targets != nil && f.Targets[destTarget] == nil {
		report = append(report, ReportItem{
			Class:   ClassSuggestion,
			Code:    "target-missing",
			Message: fmt.Sprintf("targets.%s does not exist; create it before applying on %s", destTarget, destTarget),
		})
	}

	destPackages := packageMap(destinationPackages(f, destTarget))
	sourcePackages := sourcePackageMap(f, destTarget)
	ids := sortedPackageIDs(sourcePackages)
	for _, id := range ids {
		pkg := sourcePackages[id]
		if destPkg, ok := destPackages[id]; ok {
			if packageUsableOnTarget(destPkg, allowed) {
				continue
			}
			pkg = destPkg
		}
		constraints := managerConstraints(pkg)
		if len(constraints) == 0 {
			continue
		}
		candidates := usableManagers(constraints, allowed)
		if len(candidates) == 0 {
			candidates = preferredManagersForTarget(destTarget, allowed)
		}
		if len(candidates) == 0 {
			continue
		}
		report = append(report, ReportItem{
			Class:     ClassSuggestion,
			Code:      "manager-mapping-suggested",
			Message:   fmt.Sprintf("On %s, consider adding a %s manager mapping for package %q (current managers: %s).", destTarget, joinChoices(candidates), pkg.ID, strings.Join(sortedKeys(constraints), ", ")),
			PackageID: pkg.ID,
		})
	}

	return report.sorted()
}

func destinationPackages(f *schema.GenvFile, destTarget string) []schema.Package {
	if f == nil {
		return nil
	}
	if f.SchemaVersion == schema.Version8 && f.Targets != nil {
		if _, ok := f.Targets[destTarget]; !ok {
			return nil
		}
		effective, err := schema.MergeTarget(f, destTarget)
		if err != nil {
			return nil
		}
		return effective.Packages
	}
	return f.Packages
}

func sourcePackageMap(f *schema.GenvFile, destTarget string) map[string]schema.Package {
	out := make(map[string]schema.Package)
	for _, pkg := range f.Packages {
		addMapSourcePackage(out, pkg)
	}
	if f.Defaults != nil {
		for _, pkg := range f.Defaults.Packages {
			addMapSourcePackage(out, pkg)
		}
	}
	targetIDs := make([]string, 0, len(f.Targets))
	for targetID := range f.Targets {
		if targetID != destTarget {
			targetIDs = append(targetIDs, targetID)
		}
	}
	sort.Strings(targetIDs)
	for _, targetID := range targetIDs {
		target := f.Targets[targetID]
		if target == nil {
			continue
		}
		for _, pkg := range target.Packages {
			addMapSourcePackage(out, pkg)
		}
	}
	return out
}

func addMapSourcePackage(out map[string]schema.Package, pkg schema.Package) {
	if pkg.ID == "" {
		return
	}
	existing, ok := out[pkg.ID]
	if !ok || (len(managerConstraints(existing)) == 0 && len(managerConstraints(pkg)) > 0) {
		out[pkg.ID] = pkg
	}
}

func packageMap(packages []schema.Package) map[string]schema.Package {
	out := make(map[string]schema.Package, len(packages))
	for _, pkg := range packages {
		if pkg.ID != "" {
			out[pkg.ID] = pkg
		}
	}
	return out
}

func sortedPackageIDs(packages map[string]schema.Package) []string {
	ids := make([]string, 0, len(packages))
	for id := range packages {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func packageUsableOnTarget(pkg schema.Package, allowed map[string]bool) bool {
	constraints := managerConstraints(pkg)
	return len(constraints) == 0 || intersects(constraints, allowed)
}

func usableManagers(constraints, allowed map[string]bool) []string {
	var out []string
	for manager := range constraints {
		if allowed[manager] {
			out = append(out, manager)
		}
	}
	sort.Strings(out)
	return out
}

func preferredManagersForTarget(targetID string, allowed map[string]bool) []string {
	var preferred []string
	switch targetID {
	case "macos":
		preferred = []string{"brew", "mas", "linuxbrew"}
	case "arch", "wsl-arch":
		preferred = []string{"pacman", "paru", "yay", "snap", "linuxbrew"}
	case "ubuntu":
		preferred = []string{"snap", "linuxbrew"}
	case "windows":
		preferred = []string{"winget", "scoop", "choco"}
	case "linux":
		preferred = []string{"pacman", "paru", "yay", "snap", "linuxbrew"}
	}
	var out []string
	for _, manager := range preferred {
		if allowed[manager] {
			out = append(out, manager)
		}
	}
	return out
}

func sortedKeys(values map[string]bool) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func joinChoices(values []string) string {
	switch len(values) {
	case 0:
		return ""
	case 1:
		return values[0]
	case 2:
		return values[0] + " or " + values[1]
	default:
		return strings.Join(values[:len(values)-1], ", ") + ", or " + values[len(values)-1]
	}
}
