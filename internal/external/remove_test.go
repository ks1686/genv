package external

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ks1686/genv/internal/genvfile"
)

func TestRemoveRefusesModifiedOwnedPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tool")
	if err := os.WriteFile(path, []byte("changed"), 0o755); err != nil {
		t.Fatal(err)
	}
	receipt := &genvfile.ExternalReceipt{Owned: true, Paths: []genvfile.ExternalPathReceipt{{Path: path, SHA256: "239f59ed55e737c77147cf55ad0c1b030b6d7ee748a7426952f9b852d5a935e5"}}}
	if err := Remove(receipt); err == nil {
		t.Fatal("Remove() accepted modified path")
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("modified path was removed: %v", err)
	}
}

func TestRemoveRefusesUnownedReceipt(t *testing.T) {
	if err := Remove(&genvfile.ExternalReceipt{}); err == nil {
		t.Fatal("Remove() accepted unowned receipt")
	}
}

func TestRemoveDeletesUnchangedOwnedPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tool")
	if err := os.WriteFile(path, []byte("payload"), 0o755); err != nil {
		t.Fatal(err)
	}
	digest, err := fileSHA256(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := Remove(&genvfile.ExternalReceipt{Owned: true, Paths: []genvfile.ExternalPathReceipt{{Path: path, SHA256: digest}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("path still exists: %v", err)
	}
}
