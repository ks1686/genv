package files

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ks1686/genv/internal/schema"
)

func TestStatus_reportsOK_whenTargetsMatchSpec(t *testing.T) {
	// Given
	home := t.TempDir()
	setTestHome(t, home)
	source := setupSource(t, home, "simple.txt")
	target := filepath.Join(home, ".genv-test", "simple.txt")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("mkdir target parent: %v", err)
	}
	if err := os.Symlink(source, target); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	cfg := &schema.FilesConfig{Links: []schema.FileLink{{Source: source, Target: "~/.genv-test/simple.txt"}}}

	// When
	res, err := Status(cfg, "any")

	// Then
	if err != nil {
		t.Fatalf("Status error = %v", err)
	}
	if !res.OK {
		t.Fatalf("Status OK = false, want true: %+v", res.Entries)
	}
	if len(res.Entries) != 1 || res.Entries[0].Kind != "ok" {
		t.Fatalf("entries = %+v, want one ok entry", res.Entries)
	}
}

func TestStatus_reportsProblemKinds_whenTargetsDiffer(t *testing.T) {
	// Given
	home := t.TempDir()
	setTestHome(t, home)
	repo := filepath.Join(home, "repo")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatalf("mkdir repo: %v", err)
	}
	linkSource := setupSource(t, home, "simple.txt")
	templateSource := filepath.Join(repo, "config.toml")
	if err := os.WriteFile(templateSource, []byte("home = __HOME__\n"), 0o644); err != nil {
		t.Fatalf("write template source: %v", err)
	}
	wrongLink := filepath.Join(home, ".genv-test", "wrong-link")
	wrongTemplate := filepath.Join(home, ".genv-test", "config.toml")
	wrongDir := filepath.Join(home, ".genv-test", "config-dir")
	if err := os.MkdirAll(filepath.Dir(wrongLink), 0o755); err != nil {
		t.Fatalf("mkdir target parent: %v", err)
	}
	if err := os.WriteFile(wrongLink, []byte("not a symlink"), 0o644); err != nil {
		t.Fatalf("write wrong link target: %v", err)
	}
	if err := os.WriteFile(wrongTemplate, []byte("home = /old\n"), 0o644); err != nil {
		t.Fatalf("write wrong template target: %v", err)
	}
	if err := os.WriteFile(wrongDir, []byte("not a dir"), 0o644); err != nil {
		t.Fatalf("write wrong dir target: %v", err)
	}
	cfg := &schema.FilesConfig{
		Links: []schema.FileLink{
			{Source: linkSource, Target: "~/.genv-test/missing-link"},
			{Source: linkSource, Target: "~/.genv-test/wrong-link"},
		},
		Templates: []schema.FileTemplate{{Source: templateSource, Target: "~/.genv-test/config.toml"}},
		Dirs:      []schema.FileDir{{Target: "~/.genv-test/config-dir"}},
	}

	// When
	res, err := Status(cfg, "any")

	// Then
	if err != nil {
		t.Fatalf("Status error = %v", err)
	}
	if res.OK {
		t.Fatalf("Status OK = true, want false")
	}
	got := map[string]string{}
	for _, entry := range res.Entries {
		got[filepath.Base(entry.Target)] = entry.Kind
	}
	want := map[string]string{
		"missing-link": "missing",
		"wrong-link":   "wrong-type",
		"config.toml":  "mismatch",
		"config-dir":   "wrong-type",
	}
	for target, kind := range want {
		if got[target] != kind {
			t.Fatalf("kind for %s = %q, want %q (all entries: %+v)", target, got[target], kind, res.Entries)
		}
	}
}
