package files

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/ks1686/genv/internal/schema"
)

func skipIfPermUnsupported(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("Unix permission bits are not portable on Windows")
	}
}

func mustFilePerm(t *testing.T, path string) fs.FileMode {
	t.Helper()
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	return fi.Mode().Perm()
}

func TestApply_setsDirPerm_secondApplyNoop(t *testing.T) {
	skipIfPermUnsupported(t)
	home := t.TempDir()
	setTestHome(t, home)

	target := filepath.Join(home, ".gnupg")
	cfg := &schema.FilesConfig{
		Dirs: []schema.FileDir{{Target: "~/.gnupg", Perm: "0700"}},
	}

	res, err := Apply(context.Background(), cfg, "any", ApplyOptions{})
	if err != nil {
		t.Fatalf("first Apply: %v", err)
	}
	if len(res.Created) != 1 || res.Created[0] != target {
		t.Fatalf("first Apply Created = %v, want [%s]", res.Created, target)
	}
	if got := mustFilePerm(t, target); got != 0o700 {
		t.Fatalf("dir perm = %04o, want 0700", got)
	}

	res2, err := Apply(context.Background(), cfg, "any", ApplyOptions{})
	if err != nil {
		t.Fatalf("second Apply: %v", err)
	}
	if len(res2.Skipped) != 1 || res2.Skipped[0] != target {
		t.Fatalf("second Apply Skipped = %v, want [%s]", res2.Skipped, target)
	}
	if len(res2.Created) != 0 || len(res2.Updated) != 0 {
		t.Fatalf("second Apply should be a no-op, got %+v", res2)
	}
	if got := mustFilePerm(t, target); got != 0o700 {
		t.Fatalf("dir perm after second apply = %04o, want 0700", got)
	}
}

func TestApply_chmodsExistingDirPerm(t *testing.T) {
	skipIfPermUnsupported(t)
	home := t.TempDir()
	setTestHome(t, home)

	target := filepath.Join(home, ".gnupg")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if got := mustFilePerm(t, target); got == 0o700 {
		t.Fatal("precondition: dir already 0700")
	}

	cfg := &schema.FilesConfig{
		Dirs: []schema.FileDir{{Target: "~/.gnupg", Perm: "0700"}},
	}
	res, err := Apply(context.Background(), cfg, "any", ApplyOptions{})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if len(res.Updated) != 1 || res.Updated[0] != target {
		t.Fatalf("Updated = %v, want [%s]", res.Updated, target)
	}
	if got := mustFilePerm(t, target); got != 0o700 {
		t.Fatalf("dir perm = %04o, want 0700", got)
	}
}

func TestApply_setsManagedLinkSourcePerm_secondApplyNoop(t *testing.T) {
	skipIfPermUnsupported(t)
	home := t.TempDir()
	setTestHome(t, home)

	source := setupSource(t, home, "config.toml")
	if err := os.Chmod(source, 0o644); err != nil {
		t.Fatalf("chmod source: %v", err)
	}
	target := filepath.Join(home, ".snowflake", "config.toml")
	cfg := &schema.FilesConfig{
		Links: []schema.FileLink{{
			Source: source,
			Target: "~/.snowflake/config.toml",
			Mode:   "managed-link",
			Perm:   "0600",
		}},
	}

	res, err := Apply(context.Background(), cfg, "any", ApplyOptions{})
	if err != nil {
		t.Fatalf("first Apply: %v", err)
	}
	if len(res.Created) != 1 || res.Created[0] != target {
		t.Fatalf("first Apply Created = %v, want [%s]", res.Created, target)
	}
	if got := mustFilePerm(t, source); got != 0o600 {
		t.Fatalf("source perm = %04o, want 0600", got)
	}

	res2, err := Apply(context.Background(), cfg, "any", ApplyOptions{})
	if err != nil {
		t.Fatalf("second Apply: %v", err)
	}
	if len(res2.Skipped) != 1 || res2.Skipped[0] != target {
		t.Fatalf("second Apply Skipped = %v, want [%s]", res2.Skipped, target)
	}
	if len(res2.Created) != 0 || len(res2.Updated) != 0 {
		t.Fatalf("second Apply should be a no-op, got %+v", res2)
	}
}

func TestApply_setsTemplatePerm_secondApplyNoop(t *testing.T) {
	skipIfPermUnsupported(t)
	home := t.TempDir()
	setTestHome(t, home)

	repo := filepath.Join(home, "repo")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatalf("mkdir repo: %v", err)
	}
	source := filepath.Join(repo, "config.toml")
	if err := os.WriteFile(source, []byte("home = __HOME__\n"), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}

	target := filepath.Join(home, ".snowflake", "config.toml")
	cfg := &schema.FilesConfig{
		Templates: []schema.FileTemplate{{
			Source: "config.toml",
			Target: "~/.snowflake/config.toml",
			Perm:   "0600",
		}},
	}

	res, err := Apply(context.Background(), cfg, "any", ApplyOptions{SourceRoot: repo})
	if err != nil {
		t.Fatalf("first Apply: %v", err)
	}
	if len(res.Created) != 1 || res.Created[0] != target {
		t.Fatalf("first Apply Created = %v, want [%s]", res.Created, target)
	}
	if got := mustFilePerm(t, target); got != 0o600 {
		t.Fatalf("template perm = %04o, want 0600", got)
	}

	res2, err := Apply(context.Background(), cfg, "any", ApplyOptions{SourceRoot: repo})
	if err != nil {
		t.Fatalf("second Apply: %v", err)
	}
	if len(res2.Skipped) != 1 || res2.Skipped[0] != target {
		t.Fatalf("second Apply Skipped = %v, want [%s]", res2.Skipped, target)
	}
	if len(res2.Created) != 0 || len(res2.Updated) != 0 {
		t.Fatalf("second Apply should be a no-op, got %+v", res2)
	}
}

func TestStatus_reportsPermMismatch(t *testing.T) {
	skipIfPermUnsupported(t)
	home := t.TempDir()
	setTestHome(t, home)

	source := setupSource(t, home, "config.toml")
	if err := os.Chmod(source, 0o644); err != nil {
		t.Fatalf("chmod source: %v", err)
	}
	linkTarget := filepath.Join(home, ".snowflake", "config.toml")
	if err := os.MkdirAll(filepath.Dir(linkTarget), 0o755); err != nil {
		t.Fatalf("mkdir link parent: %v", err)
	}
	if err := os.Symlink(source, linkTarget); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	repo := filepath.Join(home, "repo")
	templateSource := filepath.Join(repo, "tmpl.toml")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatalf("mkdir repo: %v", err)
	}
	if err := os.WriteFile(templateSource, []byte("ok\n"), 0o644); err != nil {
		t.Fatalf("write template source: %v", err)
	}
	templateTarget := filepath.Join(home, ".config", "app.toml")
	if err := os.MkdirAll(filepath.Dir(templateTarget), 0o755); err != nil {
		t.Fatalf("mkdir template parent: %v", err)
	}
	if err := os.WriteFile(templateTarget, []byte("ok\n"), 0o644); err != nil {
		t.Fatalf("write template target: %v", err)
	}

	dirTarget := filepath.Join(home, ".gnupg")
	if err := os.MkdirAll(dirTarget, 0o755); err != nil {
		t.Fatalf("mkdir dir: %v", err)
	}

	cfg := &schema.FilesConfig{
		Links: []schema.FileLink{{
			Source: source,
			Target: "~/.snowflake/config.toml",
			Mode:   "managed-link",
			Perm:   "0600",
		}},
		Templates: []schema.FileTemplate{{
			Source: templateSource,
			Target: "~/.config/app.toml",
			Perm:   "0600",
		}},
		Dirs: []schema.FileDir{{Target: "~/.gnupg", Perm: "0700"}},
	}

	res, err := Status(cfg, "any")
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if res.OK {
		t.Fatal("Status OK = true, want false")
	}
	got := map[string]string{}
	for _, entry := range res.Entries {
		got[filepath.Base(entry.Target)] = entry.Kind
	}
	want := map[string]string{
		"config.toml": "perm-mismatch",
		"app.toml":    "perm-mismatch",
		".gnupg":      "perm-mismatch",
	}
	for target, kind := range want {
		if got[target] != kind {
			t.Fatalf("kind for %s = %q, want %q (all entries: %+v)", target, got[target], kind, res.Entries)
		}
	}
}

func TestStatus_okWhenPermMatches(t *testing.T) {
	skipIfPermUnsupported(t)
	home := t.TempDir()
	setTestHome(t, home)

	target := filepath.Join(home, ".gnupg")
	if err := os.MkdirAll(target, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.Chmod(target, 0o700); err != nil {
		t.Fatalf("chmod: %v", err)
	}

	cfg := &schema.FilesConfig{
		Dirs: []schema.FileDir{{Target: "~/.gnupg", Perm: "0700"}},
	}
	res, err := Status(cfg, "any")
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if !res.OK || len(res.Entries) != 1 || res.Entries[0].Kind != "ok" {
		t.Fatalf("Status = %+v, want one ok entry", res)
	}
}
