package export

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ks1686/genv/internal/schema"
	"github.com/ks1686/genv/internal/verify"
)

func TestBuildWithVerify_SuccessLeavesReportClean(t *testing.T) {
	f := &schema.GenvFile{
		SchemaVersion: schema.Version8,
		Targets: map[string]*schema.TargetBundle{
			"arch": {Packages: []schema.Package{{ID: "git", Prefer: "pacman"}}},
		},
	}
	report, err := BuildWithOptions(f, "arch", t.TempDir(), Options{
		Verify: func(packages []schema.Package) []verify.Result {
			if len(packages) != 1 || packages[0].ID != "git" {
				t.Fatalf("Verify packages = %+v", packages)
			}
			return []verify.Result{{
				PackageID: "git", Manager: "pacman", PkgName: "git", Code: verify.CodeOK,
			}}
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.HasErrors() {
		t.Fatalf("verified export should not have errors: %+v", report)
	}
}

func TestBuildWithVerify_NotInstalledIsError(t *testing.T) {
	f := &schema.GenvFile{
		SchemaVersion: schema.Version8,
		Targets: map[string]*schema.TargetBundle{
			"arch": {Packages: []schema.Package{{ID: "ripgrep", Prefer: "pacman"}}},
		},
	}
	outDir := t.TempDir()
	report, err := BuildWithOptions(f, "arch", outDir, Options{
		Verify: func(packages []schema.Package) []verify.Result {
			return []verify.Result{{
				PackageID: "ripgrep", Manager: "pacman", PkgName: "ripgrep",
				Code:    verify.CodeNotInstalled,
				Message: `package "ripgrep" is not installed via pacman`,
			}}
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	item := findReportItem(report, verify.CodeNotInstalled, "ripgrep")
	if item == nil || item.Class != ClassError {
		t.Fatalf("expected package-not-installed error, got %+v", report)
	}
	if _, err := os.Stat(filepath.Join(outDir, "genv.json")); err != nil {
		t.Fatalf("snapshot should still be written: %v", err)
	}
}

func TestBuildWithoutVerify_DoesNotQuery(t *testing.T) {
	f := &schema.GenvFile{
		SchemaVersion: schema.Version8,
		Targets: map[string]*schema.TargetBundle{
			"arch": {Packages: []schema.Package{{ID: "git", Prefer: "pacman"}}},
		},
	}
	report, err := Build(f, "arch", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if item := findReportItem(report, verify.CodeNotInstalled, "git"); item != nil {
		t.Fatalf("default export must not verify installs: %+v", report)
	}
}
