package adapter

import "testing"

// TestEopkg_ListInstalled_ParsesOutput verifies that eopkg list-installed output
// is parsed correctly, extracting package names from "pkgname - version, release R" lines.
func TestEopkg_ListInstalled_ParsesOutput(t *testing.T) {
	installFakeBinary(t, "eopkg",
		`if [ "$1" = "list-installed" ]; then
  echo "git - 2.43.0, release 1"
  echo "bash - 5.2.15, release 7"
fi`)
	pkgs, err := Eopkg{}.ListInstalled()
	if err != nil {
		t.Fatalf("Eopkg.ListInstalled: %v", err)
	}
	if len(pkgs) != 2 {
		t.Fatalf("expected 2 packages, got %d: %v", len(pkgs), pkgs)
	}
	if pkgs[0] != "git" || pkgs[1] != "bash" {
		t.Errorf("expected [git bash], got %v", pkgs)
	}
}

// TestEopkg_Search_ParsesOutput verifies that eopkg search output is parsed
// correctly, extracting names from "pkgname - description" lines.
func TestEopkg_Search_ParsesOutput(t *testing.T) {
	installFakeBinary(t, "eopkg",
		`if [ "$1" = "search" ]; then
  echo "git - Fast, distributed version control system"
  echo "gitg - GNOME client to view git repositories"
fi`)
	pkgs, err := Eopkg{}.Search("git")
	if err != nil {
		t.Fatalf("Eopkg.Search: %v", err)
	}
	if len(pkgs) != 2 {
		t.Fatalf("expected 2 packages, got %d: %v", len(pkgs), pkgs)
	}
	if pkgs[0] != "git" || pkgs[1] != "gitg" {
		t.Errorf("expected [git gitg], got %v", pkgs)
	}
}

// TestEopkg_Search_FiltersNonMatches verifies that packages not matching the
// query are excluded from results.
func TestEopkg_Search_FiltersNonMatches(t *testing.T) {
	installFakeBinary(t, "eopkg",
		`if [ "$1" = "search" ]; then
  echo "git - Fast, distributed version control system"
  echo "vim - Vi IMproved text editor"
fi`)
	pkgs, err := Eopkg{}.Search("git")
	if err != nil {
		t.Fatalf("Eopkg.Search: %v", err)
	}
	if len(pkgs) != 1 {
		t.Fatalf("expected 1 package, got %d: %v", len(pkgs), pkgs)
	}
	if pkgs[0] != "git" {
		t.Errorf("expected [git], got %v", pkgs)
	}
}

// TestEopkg_QueryVersion_ParsesVersion verifies that the version is correctly
// extracted from eopkg list-installed output.
func TestEopkg_QueryVersion_ParsesVersion(t *testing.T) {
	installFakeBinary(t, "eopkg",
		`if [ "$1" = "list-installed" ]; then
  echo "git - 2.43.0, release 1"
  echo "bash - 5.2.15, release 7"
fi`)
	ver, err := Eopkg{}.QueryVersion("git")
	if err != nil {
		t.Fatalf("Eopkg.QueryVersion: %v", err)
	}
	if ver != "2.43.0" {
		t.Errorf("version: got %q, want %q", ver, "2.43.0")
	}
}

// TestEopkg_QueryVersion_NoMatch verifies that QueryVersion returns "" when
// the package is not in the installed list.
func TestEopkg_QueryVersion_NoMatch(t *testing.T) {
	installFakeBinary(t, "eopkg",
		`if [ "$1" = "list-installed" ]; then
  echo "git - 2.43.0, release 1"
fi`)
	ver, err := Eopkg{}.QueryVersion("vim")
	if err != nil {
		t.Fatalf("Eopkg.QueryVersion: %v", err)
	}
	if ver != "" {
		t.Errorf("expected empty version for absent package, got %q", ver)
	}
}
