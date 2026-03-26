package adapter

import "testing"

// TestEmerge_ListInstalled_ParsesOutput verifies that qlist -I output is
// parsed correctly, with category prefixes and version suffixes stripped.
func TestEmerge_ListInstalled_ParsesOutput(t *testing.T) {
	installFakeBinary(t, "qlist",
		`if [ "$1" = "-I" ]; then
  echo "dev-vcs/git-2.43.0"
  echo "dev-libs/openssl-3.1.4-r1"
fi`)
	pkgs, err := Emerge{}.ListInstalled()
	if err != nil {
		t.Fatalf("Emerge.ListInstalled: %v", err)
	}
	if len(pkgs) != 2 {
		t.Fatalf("expected 2 packages, got %d: %v", len(pkgs), pkgs)
	}
	if pkgs[0] != "git" || pkgs[1] != "openssl" {
		t.Errorf("expected [git openssl], got %v", pkgs)
	}
}

// TestEmerge_Search_ParsesOutput verifies that emerge --search output is
// parsed correctly, extracting package names from "* category/name" lines.
func TestEmerge_Search_ParsesOutput(t *testing.T) {
	installFakeBinary(t, "emerge",
		`if [ "$1" = "--search" ]; then
  echo "*  dev-vcs/git"
  echo "      Latest version available: 2.43.0"
  echo "*  dev-vcs/git-annex"
  echo "      Latest version available: 10.0"
fi`)
	pkgs, err := Emerge{}.Search("git")
	if err != nil {
		t.Fatalf("Emerge.Search: %v", err)
	}
	if len(pkgs) != 2 {
		t.Fatalf("expected 2 packages, got %d: %v", len(pkgs), pkgs)
	}
	if pkgs[0] != "git" || pkgs[1] != "git-annex" {
		t.Errorf("expected [git git-annex], got %v", pkgs)
	}
}

// TestEmerge_Search_FiltersNonMatches verifies that packages not containing
// the query string are excluded from results.
func TestEmerge_Search_FiltersNonMatches(t *testing.T) {
	installFakeBinary(t, "emerge",
		`if [ "$1" = "--search" ]; then
  echo "*  dev-vcs/git"
  echo "*  app-editors/vim"
fi`)
	pkgs, err := Emerge{}.Search("git")
	if err != nil {
		t.Fatalf("Emerge.Search: %v", err)
	}
	if len(pkgs) != 1 {
		t.Fatalf("expected 1 package, got %d: %v", len(pkgs), pkgs)
	}
	if pkgs[0] != "git" {
		t.Errorf("expected [git], got %v", pkgs)
	}
}

// TestEmerge_QueryVersion_ParsesVersion verifies that the correct version is
// extracted from qlist -I output for the named package.
func TestEmerge_QueryVersion_ParsesVersion(t *testing.T) {
	installFakeBinary(t, "qlist",
		`if [ "$1" = "-I" ]; then
  echo "dev-vcs/git-2.43.0"
  echo "dev-libs/openssl-3.1.4-r1"
fi`)
	ver, err := Emerge{}.QueryVersion("git")
	if err != nil {
		t.Fatalf("Emerge.QueryVersion: %v", err)
	}
	if ver != "2.43.0" {
		t.Errorf("version: got %q, want %q", ver, "2.43.0")
	}
}

// TestEmerge_QueryVersion_NoMatch verifies that QueryVersion returns "" when
// the package is not in the installed list.
func TestEmerge_QueryVersion_NoMatch(t *testing.T) {
	installFakeBinary(t, "qlist",
		`if [ "$1" = "-I" ]; then
  echo "dev-vcs/git-2.43.0"
fi`)
	ver, err := Emerge{}.QueryVersion("vim")
	if err != nil {
		t.Fatalf("Emerge.QueryVersion: %v", err)
	}
	if ver != "" {
		t.Errorf("expected empty version for absent package, got %q", ver)
	}
}
