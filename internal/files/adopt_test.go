package files

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ks1686/genv/internal/schema"
)

func TestAdopt_seedsSourceBacksUpAndLinks(t *testing.T) {
	home := t.TempDir()
	setTestHome(t, home)

	source := filepath.Join(home, "repo", "foo")
	target := filepath.Join(home, ".foo")
	live := []byte("live content\n")
	if err := os.WriteFile(target, live, 0o644); err != nil {
		t.Fatalf("write live target: %v", err)
	}

	res, err := Adopt(source, target, AdoptOptions{})
	if err != nil {
		t.Fatalf("Adopt error = %v, want nil", err)
	}
	if !res.Seeded {
		t.Fatal("Seeded = false, want true")
	}
	if res.Source != source || res.Target != target {
		t.Fatalf("AdoptResult paths = %+v", res)
	}

	gotSource, err := os.ReadFile(source)
	if err != nil {
		t.Fatalf("read seeded source: %v", err)
	}
	if !bytes.Equal(gotSource, live) {
		t.Fatalf("source = %q, want %q", gotSource, live)
	}

	gotLink, err := os.Readlink(target)
	if err != nil {
		t.Fatalf("Readlink: %v", err)
	}
	if gotLink != source {
		t.Fatalf("symlink points to %q, want %q", gotLink, source)
	}

	matches, err := filepath.Glob(target + ".backup.*")
	if err != nil {
		t.Fatalf("glob backups: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected one backup, got %v", matches)
	}
	backupData, err := os.ReadFile(matches[0])
	if err != nil {
		t.Fatalf("read backup: %v", err)
	}
	if !bytes.Equal(backupData, live) {
		t.Fatalf("backup = %q, want %q", backupData, live)
	}
	if res.BackupPath != matches[0] {
		t.Fatalf("BackupPath = %q, want %q", res.BackupPath, matches[0])
	}
}

func TestFindLinkByTarget_matchesExpandedAndRaw(t *testing.T) {
	home := t.TempDir()
	setTestHome(t, home)

	cfg := &schema.FilesConfig{
		Links: []schema.FileLink{{
			Source: "foo",
			Target: "~/.foo",
			Mode:   "managed-link",
		}},
	}
	got, err := FindLinkByTarget(cfg, "~/.foo")
	if err != nil {
		t.Fatalf("raw target: %v", err)
	}
	if got.Source != "foo" || got.Target != "~/.foo" {
		t.Fatalf("got %+v", got)
	}
	got, err = FindLinkByTarget(cfg, filepath.Join(home, ".foo"))
	if err != nil {
		t.Fatalf("expanded target: %v", err)
	}
	if got.Source != "foo" {
		t.Fatalf("expanded match = %+v", got)
	}
	if _, err := FindLinkByTarget(cfg, "~/.missing"); err == nil {
		t.Fatal("missing target: error = nil")
	}
	if _, err := FindLinkByTarget(nil, "~/.foo"); err == nil {
		t.Fatal("nil config: error = nil")
	}
}

func TestAdopt_alreadyLinkedIsNoOp(t *testing.T) {
	home := t.TempDir()
	setTestHome(t, home)
	source := filepath.Join(home, "repo", "foo")
	target := filepath.Join(home, ".foo")
	if err := os.MkdirAll(filepath.Dir(source), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(source, []byte("src\n"), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}
	if err := os.Symlink(source, target); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	res, err := Adopt(source, target, AdoptOptions{})
	if err != nil {
		t.Fatalf("Adopt: %v", err)
	}
	if res.Seeded || len(res.Steps) != 0 {
		t.Fatalf("already-linked should be a no-op: %+v", res)
	}
}

func TestAdopt_dryRunPrintsThreeStepsWritesNothing(t *testing.T) {
	home := t.TempDir()
	setTestHome(t, home)

	source := filepath.Join(home, "repo", "foo")
	target := filepath.Join(home, ".foo")
	live := []byte("live content\n")
	if err := os.WriteFile(target, live, 0o644); err != nil {
		t.Fatalf("write live target: %v", err)
	}

	res, err := Adopt(source, target, AdoptOptions{DryRun: true})
	if err != nil {
		t.Fatalf("Adopt error = %v, want nil", err)
	}
	if !res.Seeded {
		t.Fatal("dry-run Seeded = false, want true")
	}
	if len(res.Steps) != 3 {
		t.Fatalf("Steps = %v, want 3", res.Steps)
	}
	joined := strings.Join(res.Steps, "\n")
	if !strings.Contains(joined, "copy") || !strings.Contains(joined, source) || !strings.Contains(joined, target) {
		t.Fatalf("steps missing copy: %v", res.Steps)
	}
	if !strings.Contains(joined, "backup") {
		t.Fatalf("steps missing backup: %v", res.Steps)
	}
	if !strings.Contains(joined, "link") {
		t.Fatalf("steps missing link: %v", res.Steps)
	}

	if _, err := os.Stat(source); !os.IsNotExist(err) {
		t.Fatalf("dry-run must not seed source; stat = %v", err)
	}
	fi, err := os.Lstat(target)
	if err != nil {
		t.Fatalf("lstat target: %v", err)
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		t.Fatal("dry-run must not replace target with a symlink")
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read target: %v", err)
	}
	if !bytes.Equal(got, live) {
		t.Fatalf("target was modified: got %q, want %q", got, live)
	}
	matches, err := filepath.Glob(target + ".backup.*")
	if err != nil {
		t.Fatalf("glob backups: %v", err)
	}
	if len(matches) != 0 {
		t.Fatalf("dry-run must not write backups, got %v", matches)
	}
}
