package commands

import (
	"fmt"
	"sort"
	"strings"

	"github.com/ks1686/genv/internal/schema"
)

// knownManagerList is computed once at init from the constant KnownManagers map.
var knownManagerList = func() string {
	names := make([]string, 0, len(schema.KnownManagers))
	for k := range schema.KnownManagers {
		names = append(names, k)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}()

// KnownManagerList returns a sorted, comma-separated string of all known manager names.
func KnownManagerList() string { return knownManagerList }

// KnownManagerListFor is KnownManagerList plus any spec-defined adapter names.
func KnownManagerListFor(f *schema.GenvFile) string {
	if f == nil || len(f.Adapters) == 0 {
		return knownManagerList
	}
	names := make([]string, 0, len(schema.KnownManagers)+len(f.Adapters))
	for k := range schema.KnownManagers {
		names = append(names, k)
	}
	for k := range f.Adapters {
		names = append(names, k)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

// RedactValue returns value unchanged unless sensitive is true, in which case
// it returns "[redacted]". Use for any user-facing output that may expose secrets.
func RedactValue(value string, sensitive bool) string {
	if sensitive && value != "" {
		return "[redacted]"
	}
	return value
}

// ActiveBundle returns the existing v8 target bundle selected for a mutation.
// The target key must already be present so commands cannot silently create a
// profile for the wrong OS due to a typo or unsupported host classification.
func ActiveBundle(f *schema.GenvFile, targetID string) (*schema.TargetBundle, error) {
	if f == nil {
		return nil, fmt.Errorf("genv file is nil")
	}
	if f.SchemaVersion != schema.Version8 {
		return nil, fmt.Errorf("active target is only valid for schemaVersion %q", schema.Version8)
	}
	if targetID == "" {
		return nil, fmt.Errorf("schemaVersion %q requires an active target", schema.Version8)
	}
	if !schema.KnownTargets[targetID] {
		return nil, fmt.Errorf("unknown target %q", targetID)
	}
	if f.Targets == nil {
		return nil, fmt.Errorf("no matching targets.%s", targetID)
	}
	bundle, ok := f.Targets[targetID]
	if !ok {
		return nil, fmt.Errorf("no matching targets.%s", targetID)
	}
	if bundle == nil {
		bundle = &schema.TargetBundle{}
		f.Targets[targetID] = bundle
	}
	return bundle, nil
}

func activePackageSlice(f *schema.GenvFile, targetID string) (*[]schema.Package, error) {
	bundle, err := ActiveBundle(f, targetID)
	if err != nil {
		return nil, err
	}
	seedTargetPackagesFromDefaults(f, bundle)
	return &bundle.Packages, nil
}

func seedTargetPackagesFromDefaults(f *schema.GenvFile, bundle *schema.TargetBundle) {
	if bundle == nil || bundle.Packages != nil || f == nil || f.Defaults == nil || len(f.Defaults.Packages) == 0 {
		return
	}
	bundle.Packages = copyPackages(f.Defaults.Packages)
}

func copyPackages(in []schema.Package) []schema.Package {
	if in == nil {
		return nil
	}
	out := make([]schema.Package, len(in))
	for i, pkg := range in {
		out[i] = pkg
		out[i].Managers = copyStringMap(pkg.Managers)
		out[i].Host = copyHostPredicate(pkg.Host)
	}
	return out
}

func copyStringMap(in map[string]string) map[string]string {
	if in == nil {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func copyHostPredicate(in schema.HostPredicate) schema.HostPredicate {
	if in == nil {
		return nil
	}
	out := make(schema.HostPredicate, len(in))
	copy(out, in)
	return out
}

func defaultEnvExists(defaults *schema.TargetBundle, name string) bool {
	return defaults != nil && defaults.Env[name] != nil
}

func defaultAliasExists(defaults *schema.TargetBundle, name string) bool {
	return defaults != nil && defaults.Shell != nil && defaults.Shell.Aliases[name] != nil
}

func defaultServiceExists(defaults *schema.TargetBundle, name string) bool {
	return defaults != nil && defaults.Services[name] != nil
}
