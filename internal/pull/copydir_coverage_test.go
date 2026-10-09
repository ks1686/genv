package pull

import (
	"github.com/ks1686/genv/internal/schema"
	"os"
	"path/filepath"
	"testing"
)

func TestCopyBundleAssetsCopiesDirectoryTree(t *testing.T) {
	cache, dst := t.TempDir(), t.TempDir()
	writeAsset(t, cache, "assets/tree/a.txt", "a")
	writeAsset(t, cache, "assets/tree/nested/b.txt", "b")
	f := &schema.GenvFile{SchemaVersion: schema.Version7, Files: &schema.FilesConfig{Links: []schema.FileLink{{Source: "assets/tree", Target: "~/.tree"}}}}
	copied, err := CopyBundleAssets(cache, dst, f)
	if err != nil {
		t.Fatal(err)
	}
	if len(copied) != 1 || copied[0] != "assets/tree" {
		t.Fatalf("copied=%v", copied)
	}
	assertFileContent(t, filepath.Join(dst, "assets/tree/a.txt"), "a")
	assertFileContent(t, filepath.Join(dst, "assets/tree/nested/b.txt"), "b")
}

func TestCopyBundleDirRejectsNestedSymlink(t *testing.T) {
	src, dst, outside := t.TempDir(), t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(src, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(src, "assets", "link")); err != nil {
		t.Skip(err)
	}
	if err := copyBundleDir(filepath.Join(src, "assets"), dst, "assets"); err == nil {
		t.Fatal("directory copy followed nested symlink")
	}
}
