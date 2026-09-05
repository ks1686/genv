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

func TestStatus_reportsDrifted_whenLinkedSourceHashDiffers(t *testing.T) {
	home := t.TempDir()
	setTestHome(t, home)
	source := setupSource(t, home, "npmrc")
	target := filepath.Join(home, ".npmrc")
	if err := os.Symlink(source, target); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	cfg := &schema.FilesConfig{Links: []schema.FileLink{{
		Source: source,
		Target: "~/.npmrc",
		Mode:   "managed-link",
	}}}

	applied, err := HashFile(source)
	if err != nil || applied == "" {
		t.Fatalf("HashFile: hash=%q err=%v", applied, err)
	}
	if err := os.WriteFile(source, []byte("registry=https://example.invalid\n"), 0o644); err != nil {
		t.Fatalf("rewrite source: %v", err)
	}

	res, err := StatusWithHashes(cfg, "any", map[string]string{target: applied})
	if err != nil {
		t.Fatalf("StatusWithHashes: %v", err)
	}
	if res.OK {
		t.Fatal("Status.OK = true, want false when source content drifted")
	}
	if len(res.Entries) != 1 || res.Entries[0].Kind != "drifted" {
		t.Fatalf("entries = %+v, want one drifted entry", res.Entries)
	}
	if res.Entries[0].Target != target {
		t.Fatalf("drifted target = %q, want %q", res.Entries[0].Target, target)
	}
}

func TestStatus_keepsMismatch_whenHashAlsoDiffers(t *testing.T) {
	home := t.TempDir()
	setTestHome(t, home)
	source := setupSource(t, home, "wanted.txt")
	other := setupSource(t, home, "other.txt")
	target := filepath.Join(home, ".genv-test", "link")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.Symlink(other, target); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	hash, err := HashFile(source)
	if err != nil {
		t.Fatalf("HashFile: %v", err)
	}
	cfg := &schema.FilesConfig{Links: []schema.FileLink{{Source: source, Target: "~/.genv-test/link"}}}
	res, err := StatusWithHashes(cfg, "any", map[string]string{target: hash})
	if err != nil {
		t.Fatalf("StatusWithHashes: %v", err)
	}
	if len(res.Entries) != 1 || res.Entries[0].Kind != "mismatch" {
		t.Fatalf("topology mismatch must win over content drift: %+v", res.Entries)
	}
}

func TestStatus_staysOK_whenContentHashMatches(t *testing.T) {
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
	hash, err := HashFile(source)
	if err != nil {
		t.Fatalf("HashFile: %v", err)
	}
	cfg := &schema.FilesConfig{Links: []schema.FileLink{{Source: source, Target: "~/.genv-test/simple.txt"}}}
	res, err := StatusWithHashes(cfg, "any", map[string]string{target: hash})
	if err != nil {
		t.Fatalf("StatusWithHashes: %v", err)
	}
	if !res.OK || len(res.Entries) != 1 || res.Entries[0].Kind != "ok" {
		t.Fatalf("matching hash should stay ok: %+v", res.Entries)
	}
}

func TestStatus_staysOK_whenLockHasNoContentHash(t *testing.T) {
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

	res, err := StatusWithHashes(cfg, "any", nil)
	if err != nil {
		t.Fatalf("StatusWithHashes: %v", err)
	}
	if !res.OK || len(res.Entries) != 1 || res.Entries[0].Kind != "ok" {
		t.Fatalf("old locks without hashes must stay topology-only: %+v", res.Entries)
	}
}

func TestStatus_reportsDrifted_whenTemplateSourceHashDiffersButTargetMatchesRender(t *testing.T) {
	home := t.TempDir()
	setTestHome(t, home)
	source := filepath.Join(home, "repo", "config.toml")
	if err := os.MkdirAll(filepath.Dir(source), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(source, []byte("home = __HOME__\n"), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}
	rendered, err := renderedTemplate(source, "any")
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	applied := HashBytes(rendered)
	target := filepath.Join(home, ".genv-test", "config.toml")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("mkdir target parent: %v", err)
	}
	if err := os.WriteFile(target, rendered, 0o644); err != nil {
		t.Fatalf("write target: %v", err)
	}

	// Source changes, then the target is rewritten to the new render outside
	// apply. Topology still matches; the lock hash is the previous render.
	if err := os.WriteFile(source, []byte("home = __HOME__\neditor = helix\n"), 0o644); err != nil {
		t.Fatalf("rewrite source: %v", err)
	}
	newRendered, err := renderedTemplate(source, "any")
	if err != nil {
		t.Fatalf("re-render: %v", err)
	}
	if err := os.WriteFile(target, newRendered, 0o644); err != nil {
		t.Fatalf("rewrite target: %v", err)
	}

	cfg := &schema.FilesConfig{Templates: []schema.FileTemplate{{Source: source, Target: "~/.genv-test/config.toml"}}}
	res, err := StatusWithHashes(cfg, "any", map[string]string{target: applied})
	if err != nil {
		t.Fatalf("StatusWithHashes: %v", err)
	}
	if res.OK || len(res.Entries) != 1 || res.Entries[0].Kind != "drifted" {
		t.Fatalf("entries = %+v, want one drifted template", res.Entries)
	}
}
