package resolver

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/ks1686/genv/internal/adapter"
	"github.com/ks1686/genv/internal/genvfile"
)

// outdatedTestMgr implements Adapter + OutdatedLister with a configurable
// outdated set and optional error, for FilterOutdated tests.
type outdatedTestMgr struct {
	name     string
	outdated map[string]string
	err      error
}

func (m *outdatedTestMgr) Name() string    { return m.name }
func (m *outdatedTestMgr) Available() bool { return true }
func (m *outdatedTestMgr) NormalizeID(id string, _ map[string]string) (string, bool) {
	return id, false
}
func (m *outdatedTestMgr) PlanInstall(p string) []string       { return []string{"install", p} }
func (m *outdatedTestMgr) PlanUninstall(p string) []string     { return []string{"uninstall", p} }
func (m *outdatedTestMgr) PlanUpgrade(p string) []string       { return []string{"upgrade", p} }
func (m *outdatedTestMgr) PlanClean() [][]string               { return nil }
func (m *outdatedTestMgr) Query(string) (bool, error)          { return true, nil }
func (m *outdatedTestMgr) ListInstalled() ([]string, error)    { return nil, nil }
func (m *outdatedTestMgr) QueryVersion(string) (string, error) { return "", nil }
func (m *outdatedTestMgr) ListOutdated(pkgNames []string) (map[string]string, error) {
	return m.outdated, m.err
}

// plainTestMgr implements Adapter only (no OutdatedLister).
type plainTestMgr struct{ name string }

func (m *plainTestMgr) Name() string                                              { return m.name }
func (m *plainTestMgr) Available() bool                                           { return true }
func (m *plainTestMgr) NormalizeID(id string, _ map[string]string) (string, bool) { return id, false }
func (m *plainTestMgr) PlanInstall(p string) []string                             { return []string{"install", p} }
func (m *plainTestMgr) PlanUninstall(p string) []string                           { return []string{"uninstall", p} }
func (m *plainTestMgr) PlanUpgrade(p string) []string                             { return []string{"upgrade", p} }
func (m *plainTestMgr) PlanClean() [][]string                                     { return nil }
func (m *plainTestMgr) Query(string) (bool, error)                                { return true, nil }
func (m *plainTestMgr) ListInstalled() ([]string, error)                          { return nil, nil }
func (m *plainTestMgr) QueryVersion(string) (string, error)                       { return "", nil }

func swapLookupAdapter(t *testing.T, byName map[string]adapter.Adapter) {
	t.Helper()
	orig := lookupAdapter
	lookupAdapter = func(name string) adapter.Adapter { return byName[name] }
	t.Cleanup(func() { lookupAdapter = orig })
}

func keptIDs(pkgs []genvfile.LockedPackage) []string {
	ids := make([]string, len(pkgs))
	for i, p := range pkgs {
		ids[i] = p.ID
	}
	return ids
}

func TestFilterOutdated_KeepsOnlyOutdated(t *testing.T) {
	swapLookupAdapter(t, map[string]adapter.Adapter{
		"brew": &outdatedTestMgr{name: "brew", outdated: map[string]string{"wget": "1.21.4"}},
	})
	packages := []genvfile.LockedPackage{
		{ID: "wget", Manager: "brew", PkgName: "wget"},
		{ID: "jq", Manager: "brew", PkgName: "jq"},
	}
	kept, warnings := FilterOutdated(packages)
	if got := keptIDs(kept); !slices.Equal(got, []string{"wget"}) {
		t.Fatalf("kept = %v, want [wget]", got)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "outdated timing: brew") {
		t.Fatalf("warnings = %v, want one brew timing line", warnings)
	}
}

func TestFilterOutdated_QueryErrorKeepsAllWithWarning(t *testing.T) {
	swapLookupAdapter(t, map[string]adapter.Adapter{
		"brew": &outdatedTestMgr{name: "brew", err: errors.New("registry unreachable")},
	})
	packages := []genvfile.LockedPackage{
		{ID: "wget", Manager: "brew", PkgName: "wget"},
		{ID: "jq", Manager: "brew", PkgName: "jq"},
	}
	kept, warnings := FilterOutdated(packages)
	if got := keptIDs(kept); !slices.Equal(got, []string{"wget", "jq"}) {
		t.Fatalf("kept = %v, want all packages", got)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "brew") || !strings.Contains(warnings[0], "after") {
		t.Fatalf("warnings = %v, want one mentioning brew and duration", warnings)
	}
}

func TestFilterOutdated_NoCapabilityKeepsAll(t *testing.T) {
	swapLookupAdapter(t, map[string]adapter.Adapter{
		"noop": &plainTestMgr{name: "noop"},
	})
	packages := []genvfile.LockedPackage{
		{ID: "a", Manager: "noop", PkgName: "a"},
		{ID: "b", Manager: "noop", PkgName: "b"},
	}
	kept, warnings := FilterOutdated(packages)
	if got := keptIDs(kept); !slices.Equal(got, []string{"a", "b"}) {
		t.Fatalf("kept = %v, want all packages", got)
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings = %v, want none", warnings)
	}
}

func TestFilterOutdated_MixedManagersPreserveOrder(t *testing.T) {
	swapLookupAdapter(t, map[string]adapter.Adapter{
		"brew": &outdatedTestMgr{name: "brew", outdated: map[string]string{"wget": "2"}},
		"mas":  &outdatedTestMgr{name: "mas", outdated: nil}, // nothing outdated
		"noop": &plainTestMgr{name: "noop"},                  // no detection: keep
	})
	packages := []genvfile.LockedPackage{
		{ID: "wget", Manager: "brew", PkgName: "wget"},
		{ID: "xcode", Manager: "mas", PkgName: "497799835"},
		{ID: "jq", Manager: "brew", PkgName: "jq"},
		{ID: "tool", Manager: "noop", PkgName: "tool"},
	}
	kept, _ := FilterOutdated(packages)
	if got := keptIDs(kept); !slices.Equal(got, []string{"wget", "tool"}) {
		t.Fatalf("kept = %v, want [wget tool]", got)
	}
}
