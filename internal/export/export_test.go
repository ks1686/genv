package export

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ks1686/genv/internal/genvfile"
	"github.com/ks1686/genv/internal/schema"
)

func TestBuildGoldenSnapshotAndReport(t *testing.T) {
	root := repoRoot(t)
	fixtureDir := filepath.Join(root, "testdata", "export", "multi-target")
	specPath := filepath.Join(fixtureDir, "genv.json")
	outDir := t.TempDir()

	f, err := genvfile.Read(specPath)
	if err != nil {
		t.Fatal(err)
	}
	report, err := BuildWithOptions(f, "arch", outDir, Options{BaseDir: fixtureDir})
	if err != nil {
		t.Fatal(err)
	}
	if !report.HasErrors() {
		t.Fatal("expected report errors for incompatible manager and absolute source")
	}

	assertGoldenFile(t, filepath.Join(outDir, "genv.json"), filepath.Join(fixtureDir, "golden", "arch", "genv.json"))
	assertGoldenFile(t, filepath.Join(outDir, "report.json"), filepath.Join(fixtureDir, "golden", "arch", "report.json"))
	assertGoldenFile(t, filepath.Join(outDir, "files", "assets", "gitconfig"), filepath.Join(fixtureDir, "assets", "gitconfig"))
	assertGoldenFile(t, filepath.Join(outDir, "files", "assets", "config.tmpl"), filepath.Join(fixtureDir, "assets", "config.tmpl"))

	data, err := os.ReadFile(filepath.Join(outDir, "genv.json"))
	if err != nil {
		t.Fatal(err)
	}
	parsed, errs, err := schema.ParseAndValidate(data)
	if err != nil || len(errs) > 0 {
		t.Fatalf("exported snapshot failed validation: err=%v errs=%v", err, errs)
	}
	if parsed.Defaults != nil || parsed.Targets["macos"] != nil {
		t.Fatalf("snapshot should contain no defaults or sibling targets: %+v", parsed)
	}
	if strings.Contains(string(data), "SECRET") || strings.Contains(string(data), "do-not-export") {
		t.Fatalf("snapshot leaked sensitive env data:\n%s", data)
	}
}

func TestBuildReportsAptDNFDeferredSuggestion(t *testing.T) {
	f := &schema.GenvFile{
		SchemaVersion: schema.Version8,
		Targets: map[string]*schema.TargetBundle{
			"ubuntu": {
				Packages: []schema.Package{{ID: "htop"}},
			},
		},
	}

	report, err := Build(f, "ubuntu", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if len(report) != 1 {
		t.Fatalf("report length = %d, want 1: %+v", len(report), report)
	}
	if report[0].Class != ClassSuggestion || report[0].Code != "apt-dnf-deferred" || report[0].PackageID != "htop" {
		t.Fatalf("unexpected report: %+v", report)
	}
}

func assertGoldenFile(t *testing.T, gotPath, wantPath string) {
	t.Helper()
	got, err := os.ReadFile(gotPath)
	if err != nil {
		t.Fatalf("read got %s: %v", gotPath, err)
	}
	want, err := os.ReadFile(wantPath)
	if err != nil {
		t.Fatalf("read want %s: %v", wantPath, err)
	}
	if normalizePathSeparators(string(got)) != normalizePathSeparators(string(want)) {
		t.Fatalf("%s mismatch (-got +want)\n--- got ---\n%s\n--- want ---\n%s", filepath.Base(gotPath), got, want)
	}
}

func normalizePathSeparators(s string) string {
	return strings.ReplaceAll(s, "\\", "/")
}

func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not locate repo root")
		}
		dir = parent
	}
}
