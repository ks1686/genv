package adapter

import (
	"testing"
)

// TestXbps_ListInstalled_ParsesOutput verifies that xbps-query -l output is
// parsed correctly, with version suffixes stripped.
func TestXbps_ListInstalled_ParsesOutput(t *testing.T) {
	installFakeBinary(t, "xbps-query",
		`if [ "$1" = "-l" ]; then
  echo "ii gimp-2.10.32_2          GNU Image Manipulation Program"
  echo "ii git-2.39.2_1            Distributed version control system"
fi`)
	pkgs, err := Xbps{}.ListInstalled()
	if err != nil {
		t.Fatalf("Xbps.ListInstalled: %v", err)
	}
	if len(pkgs) != 2 {
		t.Fatalf("expected 2 packages, got %d: %v", len(pkgs), pkgs)
	}
	if pkgs[0] != "gimp" || pkgs[1] != "git" {
		t.Errorf("expected [gimp git], got %v", pkgs)
	}
}

// TestXbps_Search_ParsesOutput verifies that xbps-query -Rs output is
// parsed correctly.
func TestXbps_Search_ParsesOutput(t *testing.T) {
	installFakeBinary(t, "xbps-query",
		`if [ "$1" = "-Rs" ]; then
		  echo "[*] gimp-2.10.32_2          GNU Image Manipulation Program"
		  echo "[*] git-2.39.2_1            Distributed version control system"
fi`)
	pkgs, err := Xbps{}.Search("git")
	if err != nil {
		t.Fatalf("Xbps.Search: %v", err)
	}
	if len(pkgs) != 1 {
		t.Fatalf("expected 1 package, got %d: %v", len(pkgs), pkgs)
	}
	if pkgs[0] != "git-2.39.2_1" {
		t.Errorf("expected [git-2.39.2_1], got %v", pkgs)
	}
}

// TestXbps_QueryVersion_ParsesVersion verifies that xbps-query -p pkgver
// output is parsed correctly (strips pkgname prefix and _revision suffix).
func TestXbps_QueryVersion_ParsesVersion(t *testing.T) {
	installFakeBinary(t, "xbps-query",
		`if [ "$1" = "-p" ] && [ "$2" = "pkgver" ]; then
  echo "gimp-2.10.32_2"
fi`)
	ver, err := Xbps{}.QueryVersion("gimp")
	if err != nil {
		t.Fatalf("Xbps.QueryVersion: %v", err)
	}
	if ver != "2.10.32" {
		t.Errorf("version: got %q, want %q", ver, "2.10.32")
	}
}

// TestTrimVersionSuffix_Xbps tests the version trimming for xbps packages.
func TestTrimVersionSuffix_Xbps(t *testing.T) {
	if got := trimVersionSuffix("gimp-2.10.32_2"); got != "gimp" {
		t.Errorf("got %q, want %q", got, "gimp")
	}
	if got := trimVersionSuffix("libxkbcommon-1.5.0_1"); got != "libxkbcommon" {
		t.Errorf("got %q, want %q", got, "libxkbcommon")
	}
}
