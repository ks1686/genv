//go:build !windows

package genvfile

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ks1686/genv/internal/schema"
)

// A spec symlinked into a dotfiles checkout must stay a symlink after Write;
// the new content must land in the link target.
func TestWrite_PreservesSymlinkedSpec(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "dotfiles", "genv.json")
	if err := Write(target, &schema.GenvFile{SchemaVersion: schema.Version}); err != nil {
		t.Fatalf("seed Write: %v", err)
	}
	link := filepath.Join(dir, "config", "genv.json")
	if err := os.MkdirAll(filepath.Dir(link), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}

	f := &schema.GenvFile{SchemaVersion: schema.Version}
	f.Packages = append(f.Packages, schema.Package{ID: "ripgrep", Version: "*"})
	if err := Write(link, f); err != nil {
		t.Fatalf("Write via symlink: %v", err)
	}

	fi, err := os.Lstat(link)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("Write replaced the symlink %s with a regular file", link)
	}
	got, err := Read(target)
	if err != nil {
		t.Fatalf("Read target: %v", err)
	}
	if len(got.Packages) != 1 || got.Packages[0].ID != "ripgrep" {
		t.Fatalf("target packages = %+v, want [ripgrep]", got.Packages)
	}
}
