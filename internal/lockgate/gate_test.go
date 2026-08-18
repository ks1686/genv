package lockgate

import (
	"strings"
	"testing"

	"github.com/ks1686/genv/internal/genvfile"
)

func TestCheck_EmptyLockIsNotForeign(t *testing.T) {
	lf := &genvfile.LockFile{}

	d := Check(lf, "arch", "linux", map[string]bool{})

	if d.Foreign {
		t.Fatalf("expected empty lock to be local, got foreign with reason %q", d.Reason)
	}
}

func TestCheck_MacOSLockOnArchIsForeign(t *testing.T) {
	lf := &genvfile.LockFile{
		Target: "macos",
		GOOS:   "darwin",
		Packages: []genvfile.LockedPackage{
			{ID: "foo", Manager: "mas", PkgName: "1"},
			{ID: "bar", Manager: "brew", PkgName: "bar"},
		},
	}

	d := Check(lf, "arch", "linux", map[string]bool{"pacman": true})

	if !d.Foreign {
		t.Fatal("expected foreign")
	}
	if !strings.Contains(d.Reason, "target") {
		t.Fatalf("expected target mismatch reason, got %q", d.Reason)
	}
}

func TestCheck_GOOSMismatchIsForeign(t *testing.T) {
	lf := &genvfile.LockFile{GOOS: "darwin"}

	d := Check(lf, "macos", "linux", map[string]bool{})

	if !d.Foreign {
		t.Fatal("expected foreign")
	}
	if !strings.Contains(d.Reason, "goos") {
		t.Fatalf("expected goos mismatch reason, got %q", d.Reason)
	}
}

func TestCheck_UnavailableManagerIsNotForeignWhenMetadataMatches(t *testing.T) {
	lf := &genvfile.LockFile{
		Target: "arch",
		GOOS:   "linux",
		Packages: []genvfile.LockedPackage{
			{ID: "git", Manager: "pacman", PkgName: "git"},
			{ID: "nvim", Manager: "pacman", PkgName: "neovim"},
		},
	}

	d := Check(lf, "arch", "linux", map[string]bool{"brew": true})

	if d.Foreign {
		t.Fatalf("expected local lock with skip list, got foreign %q", d.Reason)
	}
	if len(d.Unavailable) != 1 || d.Unavailable[0] != "pacman" {
		t.Fatalf("unavailable = %v, want [pacman]", d.Unavailable)
	}
}

func TestCheckStrict_MissingMetadataIsForeign(t *testing.T) {
	lf := &genvfile.LockFile{
		Packages: []genvfile.LockedPackage{
			{ID: "git", Manager: "pacman", PkgName: "git"},
		},
	}

	d := CheckStrict(lf, "arch", "linux", map[string]bool{"pacman": true})

	if !d.Foreign {
		t.Fatal("expected foreign")
	}
	if !strings.Contains(d.Reason, "target/goos") {
		t.Fatalf("expected missing metadata reason, got %q", d.Reason)
	}
}

func TestCheck_EmptyPackageManagerIsIgnored(t *testing.T) {
	lf := &genvfile.LockFile{
		Target: "arch",
		GOOS:   "linux",
		Packages: []genvfile.LockedPackage{
			{ID: "legacy", PkgName: "legacy"},
		},
	}

	d := Check(lf, "arch", "linux", map[string]bool{})

	if d.Foreign {
		t.Fatalf("expected package without manager to be ignored, got %q", d.Reason)
	}
}

func TestCheck_MatchingTargetOK(t *testing.T) {
	lf := &genvfile.LockFile{
		Target: "arch",
		GOOS:   "linux",
		Packages: []genvfile.LockedPackage{
			{ID: "git", Manager: "pacman", PkgName: "git"},
		},
	}

	d := Check(lf, "arch", "linux", map[string]bool{"pacman": true})

	if d.Foreign {
		t.Fatalf("expected OK, got foreign with reason %q", d.Reason)
	}
	if len(d.Unavailable) != 0 {
		t.Fatalf("unavailable = %v, want empty", d.Unavailable)
	}
}
