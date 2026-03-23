package commands

import (
	"testing"

	"github.com/ks1686/gpm/internal/gpmfile"
	"github.com/ks1686/gpm/internal/schema"
)

func TestStatus_AllOK(t *testing.T) {
	f := &schema.GpmFile{
		Packages: []schema.Package{
			{ID: "git"},
			{ID: "vim", Version: "9.*"},
		},
	}
	lf := &gpmfile.LockFile{
		Packages: []gpmfile.LockedPackage{
			{ID: "git", Manager: "apt", PkgName: "git", InstalledVersion: "2.43.0"},
			{ID: "vim", Manager: "apt", PkgName: "vim", InstalledVersion: "9.1.0"},
		},
	}
	entries := Status(f, lf)
	if len(entries) != 2 {
		t.Fatalf("len = %d, want 2", len(entries))
	}
	for _, e := range entries {
		if e.Kind != StatusOK {
			t.Errorf("entry %q: kind = %q, want %q", e.ID, e.Kind, StatusOK)
		}
	}
}

func TestStatus_Missing(t *testing.T) {
	f := &schema.GpmFile{
		Packages: []schema.Package{{ID: "curl"}},
	}
	lf := &gpmfile.LockFile{}
	entries := Status(f, lf)
	if len(entries) != 1 {
		t.Fatalf("len = %d, want 1", len(entries))
	}
	if entries[0].Kind != StatusMissing {
		t.Errorf("kind = %q, want %q", entries[0].Kind, StatusMissing)
	}
	if entries[0].Manager != "" {
		t.Errorf("Manager = %q, want empty for missing", entries[0].Manager)
	}
}

func TestStatus_Extra(t *testing.T) {
	f := &schema.GpmFile{}
	lf := &gpmfile.LockFile{
		Packages: []gpmfile.LockedPackage{
			{ID: "htop", Manager: "pacman", PkgName: "htop", InstalledVersion: "3.3.0"},
		},
	}
	entries := Status(f, lf)
	if len(entries) != 1 {
		t.Fatalf("len = %d, want 1", len(entries))
	}
	if entries[0].Kind != StatusExtra {
		t.Errorf("kind = %q, want %q", entries[0].Kind, StatusExtra)
	}
}

func TestStatus_Drift(t *testing.T) {
	f := &schema.GpmFile{
		Packages: []schema.Package{{ID: "neovim", Version: "0.10.*"}},
	}
	lf := &gpmfile.LockFile{
		Packages: []gpmfile.LockedPackage{
			{ID: "neovim", Manager: "brew", PkgName: "neovim", InstalledVersion: "0.9.5"},
		},
	}
	entries := Status(f, lf)
	if len(entries) != 1 {
		t.Fatalf("len = %d, want 1", len(entries))
	}
	if entries[0].Kind != StatusDrift {
		t.Errorf("kind = %q, want %q", entries[0].Kind, StatusDrift)
	}
}

func TestStatus_NoDriftWhenNoInstalledVersion(t *testing.T) {
	// Old lock entries without InstalledVersion should not cause drift.
	f := &schema.GpmFile{
		Packages: []schema.Package{{ID: "git", Version: "2.40.*"}},
	}
	lf := &gpmfile.LockFile{
		Packages: []gpmfile.LockedPackage{
			{ID: "git", Manager: "apt", PkgName: "git"}, // InstalledVersion is ""
		},
	}
	entries := Status(f, lf)
	if len(entries) != 1 {
		t.Fatalf("len = %d, want 1", len(entries))
	}
	if entries[0].Kind != StatusOK {
		t.Errorf("kind = %q, want %q (old lock entries must not drift)", entries[0].Kind, StatusOK)
	}
}

func TestStatus_Empty(t *testing.T) {
	entries := Status(&schema.GpmFile{}, &gpmfile.LockFile{})
	if len(entries) != 0 {
		t.Errorf("expected empty entries for empty spec+lock, got %d", len(entries))
	}
}

func TestStatus_Mixed(t *testing.T) {
	f := &schema.GpmFile{
		Packages: []schema.Package{
			{ID: "git"},                    // in lock → ok
			{ID: "curl"},                  // not in lock → missing
			{ID: "vim", Version: "9.*"},   // in lock, version drifts → drift
		},
	}
	lf := &gpmfile.LockFile{
		Packages: []gpmfile.LockedPackage{
			{ID: "git", Manager: "apt", PkgName: "git", InstalledVersion: "2.43.0"},
			{ID: "vim", Manager: "apt", PkgName: "vim", InstalledVersion: "8.2.0"},
			{ID: "htop", Manager: "apt", PkgName: "htop"}, // extra
		},
	}
	entries := Status(f, lf)
	// expected: git=ok, curl=missing, vim=drift, htop=extra
	if len(entries) != 4 {
		t.Fatalf("len = %d, want 4", len(entries))
	}
	byID := make(map[string]StatusKind)
	for _, e := range entries {
		byID[e.ID] = e.Kind
	}
	cases := map[string]StatusKind{
		"git":  StatusOK,
		"curl": StatusMissing,
		"vim":  StatusDrift,
		"htop": StatusExtra,
	}
	for id, want := range cases {
		if got := byID[id]; got != want {
			t.Errorf("entry %q: kind = %q, want %q", id, got, want)
		}
	}
}
