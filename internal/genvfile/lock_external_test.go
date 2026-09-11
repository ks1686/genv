package genvfile

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExternalReceiptRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "genv.lock.json")
	want := &LockFile{
		SchemaVersion: "9",
		Packages: []LockedPackage{{
			ID:               "tool",
			Manager:          "external",
			PkgName:          "tool",
			InstalledVersion: "1.2.3",
			External: &ExternalReceipt{
				SourceType:     "githubRelease",
				ReleaseID:      "42",
				ReleaseTag:     "v1.2.3",
				ArtifactURL:    "https://example.test/tool.tar.gz",
				ArtifactSHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
				Verification:   "minisign:key-id",
				RecipeSHA256:   "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
				InstallType:    "archive",
				Owned:          true,
				Paths:          []ExternalPathReceipt{{Path: "/home/test/.local/bin/tool", SHA256: "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"}},
			},
		}},
	}

	if err := WriteLock(path, want); err != nil {
		t.Fatalf("WriteLock() error: %v", err)
	}
	got, err := ReadLock(path)
	if err != nil {
		t.Fatalf("ReadLock() error: %v", err)
	}
	if len(got.Packages) != 1 || got.Packages[0].External == nil {
		t.Fatalf("receipt missing after round trip: %+v", got)
	}
	receipt := got.Packages[0].External
	if receipt.ReleaseID != "42" || !receipt.Owned || len(receipt.Paths) != 1 || receipt.Paths[0].Path != "/home/test/.local/bin/tool" {
		t.Fatalf("receipt = %+v", receipt)
	}
}

func TestLegacyLockWithoutExternalReceiptStillParses(t *testing.T) {
	path := filepath.Join(t.TempDir(), "genv.lock.json")
	if err := os.WriteFile(path, []byte(`{"schemaVersion":"8","packages":[{"id":"tool","manager":"external","pkgName":"tool"}]}`), 0o600); err != nil {
		t.Fatalf("write legacy lock: %v", err)
	}
	got, err := ReadLock(path)
	if err != nil {
		t.Fatalf("ReadLock() error: %v", err)
	}
	if len(got.Packages) != 1 || got.Packages[0].External != nil {
		t.Fatalf("legacy package changed: %+v", got.Packages)
	}
}
