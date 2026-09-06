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
	assertGoldenFile(t, filepath.Join(outDir, "report.md"), filepath.Join(fixtureDir, "golden", "arch", "report.md"))
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

func TestIsAbsolutePathRecognizesTargetPlatformPaths(t *testing.T) {
	tests := []struct {
		name string
		path string
		want bool
	}{
		{name: "posix", path: "/etc/genv/config", want: true},
		{name: "windows drive backslash", path: `C:\Users\me\.gitconfig`, want: true},
		{name: "windows drive slash", path: "D:/Users/me/.gitconfig", want: true},
		{name: "windows drive root", path: `z:\`, want: true},
		{name: "unc backslash", path: `\\server\share\dir\file`, want: true},
		{name: "unc mixed separators", path: `\\server/share/dir/file`, want: true},
		{name: "drive relative", path: `C:Users\me\.gitconfig`, want: false},
		{name: "non drive letter", path: `1:\Users\me`, want: false},
		{name: "incomplete unc", path: `\\server`, want: false},
		{name: "relative", path: "assets/gitconfig", want: false},
		{name: "parent relative", path: "../assets/gitconfig", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isAbsolutePath(tt.path); got != tt.want {
				t.Fatalf("isAbsolutePath(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestBuildCopiesSupervisorTemplates(t *testing.T) {
	base := t.TempDir()
	if err := os.MkdirAll(filepath.Join(base, "agents"), 0o755); err != nil {
		t.Fatal(err)
	}
	plist := filepath.Join(base, "agents", "com.example.agent.plist")
	if err := os.WriteFile(plist, []byte("<plist/>\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	f := &schema.GenvFile{
		SchemaVersion: schema.Version8,
		Targets: map[string]*schema.TargetBundle{
			"macos": {
				Services: map[string]*schema.Service{
					"agent": {Launchd: &schema.LaunchdSpec{Plist: "agents/com.example.agent.plist"}},
				},
			},
		},
	}
	outDir := t.TempDir()
	if _, err := BuildWithOptions(f, "macos", outDir, Options{BaseDir: base}); err != nil {
		t.Fatal(err)
	}
	copied := filepath.Join(outDir, "files", "agents", "com.example.agent.plist")
	if _, err := os.Stat(copied); err != nil {
		t.Fatalf("expected bundled plist at %s: %v", copied, err)
	}
	data, err := os.ReadFile(filepath.Join(outDir, "genv.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "files/agents/com.example.agent.plist") {
		t.Fatalf("snapshot should rewrite launchd.plist, got:\n%s", data)
	}
}

func TestBuildDoesNotDeferNativeLinuxManagers(t *testing.T) {
	f := &schema.GenvFile{
		SchemaVersion: schema.Version8,
		Targets: map[string]*schema.TargetBundle{
			"ubuntu": {
				Packages: []schema.Package{{ID: "htop", Prefer: "apt"}},
			},
		},
	}

	report, err := Build(f, "ubuntu", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range report {
		if item.Code == "apt-dnf-deferred" || item.Code == "manager-not-supported" {
			t.Fatalf("unexpected report item: %+v", item)
		}
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
	s = strings.ReplaceAll(s, "\r\n", "\n")
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
