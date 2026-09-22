package commands

import (
	"testing"

	"github.com/ks1686/genv/internal/genvfile"
	"github.com/ks1686/genv/internal/schema"
	"github.com/ks1686/genv/internal/verify"
)

func TestStatusWithVerify_OKWhenQueryProvesInstall(t *testing.T) {
	f := &schema.GenvFile{Packages: []schema.Package{{ID: "git", Prefer: "pacman"}}}
	lf := &genvfile.LockFile{Packages: []genvfile.LockedPackage{
		{ID: "git", Manager: "pacman", PkgName: "git", InstalledVersion: "2.43.0"},
	}}
	entries := StatusWithVerify(f, lf, []verify.Result{{
		PackageID: "git", Manager: "pacman", PkgName: "git", Version: "2.43.0", Code: verify.CodeOK,
	}})
	if len(entries) != 1 || entries[0].Kind != StatusOK {
		t.Fatalf("got %+v, want ok", entries)
	}
	if entries[0].Manager != "pacman" || entries[0].InstalledVersion != "2.43.0" {
		t.Fatalf("got %+v", entries[0])
	}
}

func TestStatusWithVerify_LockedNotInstalledIsDrift(t *testing.T) {
	f := &schema.GenvFile{Packages: []schema.Package{{ID: "git", Prefer: "pacman"}}}
	lf := &genvfile.LockFile{Packages: []genvfile.LockedPackage{
		{ID: "git", Manager: "pacman", PkgName: "git", InstalledVersion: "2.43.0"},
	}}
	entries := StatusWithVerify(f, lf, []verify.Result{{
		PackageID: "git", Manager: "pacman", PkgName: "git", Code: verify.CodeNotInstalled,
		Message: `package "git" is not installed via pacman`,
	}})
	if len(entries) != 1 || entries[0].Kind != StatusDrift {
		t.Fatalf("got %+v, want drift", entries)
	}
	if entries[0].InstalledVersion != "" {
		t.Fatalf("absent install must clear InstalledVersion, got %+v", entries[0])
	}
}

func TestStatusWithVerify_UnlockedInstalledIsPresent(t *testing.T) {
	f := &schema.GenvFile{Packages: []schema.Package{{ID: "git", Prefer: "pacman"}}}
	entries := StatusWithVerify(f, &genvfile.LockFile{}, []verify.Result{{
		PackageID: "git", Manager: "pacman", PkgName: "git", Code: verify.CodeOK, Version: "2.43.0",
	}})
	if len(entries) != 1 || entries[0].Kind != StatusPresent {
		t.Fatalf("got %+v, want present", entries)
	}
}

func TestStatusWithVerify_UnlockedMissingStaysMissing(t *testing.T) {
	f := &schema.GenvFile{Packages: []schema.Package{{ID: "curl", Prefer: "pacman"}}}
	entries := StatusWithVerify(f, &genvfile.LockFile{}, []verify.Result{{
		PackageID: "curl", Manager: "pacman", PkgName: "curl", Code: verify.CodeNotInstalled,
	}})
	if len(entries) != 1 || entries[0].Kind != StatusMissing {
		t.Fatalf("got %+v, want missing", entries)
	}
}

func TestStatusWithVerify_VersionConstraintDrift(t *testing.T) {
	f := &schema.GenvFile{Packages: []schema.Package{{ID: "vim", Version: "9.*", Prefer: "pacman"}}}
	lf := &genvfile.LockFile{Packages: []genvfile.LockedPackage{
		{ID: "vim", Manager: "pacman", PkgName: "vim", InstalledVersion: "8.2.0"},
	}}
	entries := StatusWithVerify(f, lf, []verify.Result{{
		PackageID: "vim", Manager: "pacman", PkgName: "vim", Version: "8.2.0", Code: verify.CodeOK,
	}})
	if len(entries) != 1 || entries[0].Kind != StatusDrift {
		t.Fatalf("got %+v, want drift for unsatisfied constraint", entries)
	}
}

func TestStatusWithVerify_KeepsExtraLockEntries(t *testing.T) {
	f := &schema.GenvFile{Packages: []schema.Package{{ID: "git"}}}
	lf := &genvfile.LockFile{Packages: []genvfile.LockedPackage{
		{ID: "git", Manager: "pacman", PkgName: "git"},
		{ID: "htop", Manager: "pacman", PkgName: "htop"},
	}}
	entries := StatusWithVerify(f, lf, []verify.Result{{
		PackageID: "git", Manager: "pacman", PkgName: "git", Code: verify.CodeOK,
	}})
	byID := map[string]StatusKind{}
	for _, e := range entries {
		byID[e.ID] = e.Kind
	}
	if byID["git"] != StatusOK || byID["htop"] != StatusExtra {
		t.Fatalf("got %+v", entries)
	}
}
