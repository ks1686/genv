package verify

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/ks1686/genv/internal/adapter"
	"github.com/ks1686/genv/internal/genvfile"
	"github.com/ks1686/genv/internal/schema"
	"github.com/ks1686/genv/internal/testutil"
)

func TestPackages_InstalledViaResolvedManager(t *testing.T) {
	pkgs := []schema.Package{{ID: "git", Prefer: "pacman"}}
	results := Packages(pkgs, nil, Options{
		Available: map[string]bool{"pacman": true},
		Query: func(manager, pkgName string) (bool, string, error) {
			if manager != "pacman" || pkgName != "git" {
				t.Fatalf("Query(%q, %q), want pacman/git", manager, pkgName)
			}
			return true, "2.43.0", nil
		},
	})
	if len(results) != 1 {
		t.Fatalf("len = %d, want 1", len(results))
	}
	r := results[0]
	if !r.OK() || r.Code != CodeOK {
		t.Fatalf("result = %+v, want OK", r)
	}
	if r.PackageID != "git" || r.Manager != "pacman" || r.PkgName != "git" || r.Version != "2.43.0" {
		t.Fatalf("result = %+v", r)
	}
}

func TestPackages_NotInstalledIsDriftSignal(t *testing.T) {
	pkgs := []schema.Package{{ID: "ripgrep", Prefer: "pacman"}}
	results := Packages(pkgs, nil, Options{
		Available: map[string]bool{"pacman": true},
		Query: func(manager, pkgName string) (bool, string, error) {
			return false, "", nil
		},
	})
	if len(results) != 1 || results[0].OK() || results[0].Code != CodeNotInstalled {
		t.Fatalf("result = %+v, want not-installed", results)
	}
	if results[0].PackageID != "ripgrep" || results[0].Manager != "pacman" {
		t.Fatalf("result = %+v", results[0])
	}
}

func TestPackages_HonorsLockManagerOverPrefer(t *testing.T) {
	pkgs := []schema.Package{{ID: "git", Prefer: "brew"}}
	lock := []genvfile.LockedPackage{{ID: "git", Manager: "pacman", PkgName: "git"}}
	var gotManager, gotName string
	results := Packages(pkgs, lock, Options{
		Available: map[string]bool{"pacman": true, "brew": true},
		Query: func(manager, pkgName string) (bool, string, error) {
			gotManager, gotName = manager, pkgName
			return true, "1.0", nil
		},
	})
	if gotManager != "pacman" || gotName != "git" {
		t.Fatalf("Query(%q, %q), want locked pacman/git", gotManager, gotName)
	}
	if len(results) != 1 || !results[0].OK() || results[0].Manager != "pacman" {
		t.Fatalf("result = %+v", results)
	}
}

func TestPackages_LockManagerUnavailable(t *testing.T) {
	pkgs := []schema.Package{{ID: "git", Prefer: "pacman"}}
	lock := []genvfile.LockedPackage{{ID: "git", Manager: "brew", PkgName: "git"}}
	queried := false
	results := Packages(pkgs, lock, Options{
		Available: map[string]bool{"pacman": true},
		Query: func(manager, pkgName string) (bool, string, error) {
			queried = true
			return true, "", nil
		},
	})
	if queried {
		t.Fatal("Query must not run when the locked manager is unavailable")
	}
	if len(results) != 1 || results[0].OK() || results[0].Code != CodeManagerUnavailable {
		t.Fatalf("result = %+v, want manager-unavailable", results)
	}
}

func TestPackages_Unresolved(t *testing.T) {
	pkgs := []schema.Package{{ID: "git", Prefer: "winget"}}
	results := Packages(pkgs, nil, Options{
		Available: map[string]bool{},
		Query: func(manager, pkgName string) (bool, string, error) {
			t.Fatalf("Query(%q, %q) should not run for unresolved packages", manager, pkgName)
			return false, "", nil
		},
	})
	if len(results) != 1 || results[0].OK() || results[0].Code != CodeUnresolved {
		t.Fatalf("result = %+v, want unresolved", results)
	}
}

func TestPackages_QueryError(t *testing.T) {
	pkgs := []schema.Package{{ID: "git", Prefer: "pacman"}}
	results := Packages(pkgs, nil, Options{
		Available: map[string]bool{"pacman": true},
		Query: func(manager, pkgName string) (bool, string, error) {
			return false, "", errors.New("query timed out")
		},
	})
	if len(results) != 1 || results[0].OK() || results[0].Code != CodeQueryFailed {
		t.Fatalf("result = %+v, want query-failed", results)
	}
}

func TestPackages_UsesManagersMapName(t *testing.T) {
	pkgs := []schema.Package{{
		ID:       "cursor",
		Prefer:   "winget",
		Managers: map[string]string{"winget": "Anysphere.Cursor"},
	}}
	var gotName string
	results := Packages(pkgs, nil, Options{
		Available: map[string]bool{"winget": true},
		Query: func(manager, pkgName string) (bool, string, error) {
			gotName = pkgName
			return true, "", nil
		},
	})
	if gotName != "Anysphere.Cursor" {
		t.Fatalf("Query name = %q, want Anysphere.Cursor", gotName)
	}
	if len(results) != 1 || !results[0].OK() || results[0].PkgName != "Anysphere.Cursor" {
		t.Fatalf("result = %+v", results)
	}
}

func TestPackages_DefaultQueryUsesAdapter(t *testing.T) {
	stub := defaultQueryAdapter{name: "pacman", installed: map[string]bool{"git": true}, version: "2.50.0"}
	orig := adapter.All
	adapter.All = []adapter.Adapter{stub}
	t.Cleanup(func() { adapter.All = orig })

	results := Packages([]schema.Package{{ID: "git", Prefer: "pacman"}}, nil, Options{
		Available: map[string]bool{"pacman": true},
	})
	if len(results) != 1 || !results[0].OK() || results[0].Version != "2.50.0" {
		t.Fatalf("result = %+v", results)
	}
}

type defaultQueryAdapter struct {
	name      string
	installed map[string]bool
	version   string
}

func (a defaultQueryAdapter) Name() string    { return a.name }
func (a defaultQueryAdapter) Available() bool { return true }
func (a defaultQueryAdapter) NormalizeID(id string, _ map[string]string) (string, bool) {
	return id, false
}
func (a defaultQueryAdapter) PlanInstall(string) []string         { return []string{"true"} }
func (a defaultQueryAdapter) PlanUninstall(string) []string       { return []string{"true"} }
func (a defaultQueryAdapter) PlanUpgrade(string) []string         { return []string{"true"} }
func (a defaultQueryAdapter) PlanClean() [][]string               { return nil }
func (a defaultQueryAdapter) Query(pkgName string) (bool, error)  { return a.installed[pkgName], nil }
func (a defaultQueryAdapter) ListInstalled() ([]string, error)    { return nil, nil }
func (a defaultQueryAdapter) QueryVersion(string) (string, error) { return a.version, nil }

func TestPackages_Empty(t *testing.T) {
	results := Packages(nil, nil, Options{Available: map[string]bool{}})
	if len(results) != 0 {
		t.Fatalf("empty spec: got %+v", results)
	}
}

func TestPackages_ExternalPresent(t *testing.T) {
	tool := testutil.WriteStdoutTool(t, filepath.Join(t.TempDir(), "tool"), "tool-1.2.3")
	pkgs := []schema.Package{{
		ID: "tool",
		External: &schema.ExternalRecipe{
			Detect: schema.ExternalDetect{Command: []string{tool}, VersionRegex: `tool-([0-9.]+)`},
		},
	}}
	results := Packages(pkgs, nil, Options{
		Available: map[string]bool{"external": true},
		Query: func(manager, pkgName string) (bool, string, error) {
			t.Fatalf("Query(%q, %q) should not run for external recipes", manager, pkgName)
			return false, "", nil
		},
	})
	if len(results) != 1 || !results[0].OK() || results[0].Manager != "external" || results[0].Version != "1.2.3" {
		t.Fatalf("result = %+v", results)
	}
}

func TestPackages_ExternalMissing(t *testing.T) {
	pkgs := []schema.Package{{
		ID: "missing-tool",
		External: &schema.ExternalRecipe{
			Detect: schema.ExternalDetect{Command: []string{"__genv_missing_external_tool__"}},
		},
	}}
	results := Packages(pkgs, nil, Options{Available: map[string]bool{"external": true}})
	if len(results) != 1 || results[0].OK() || results[0].Code != CodeNotInstalled {
		t.Fatalf("result = %+v, want not-installed", results)
	}
}
