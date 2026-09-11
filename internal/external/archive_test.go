package external

import (
	"archive/tar"
	"archive/zip"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/klauspost/compress/zstd"
	"github.com/ks1686/genv/internal/schema"
	"github.com/ulikunitz/xz"
)

func TestInstallArchiveCopiesDeclaredFile(t *testing.T) {
	archivePath := filepath.Join(t.TempDir(), "tool.zip")
	writeZipFixture(t, archivePath, map[string]string{"tool-1.0/bin/tool": "binary"})
	destination := filepath.Join(t.TempDir(), "bin", "tool")
	receipts, restore, finish, err := installArchive(archivePath, "tool.zip", schema.ExternalInstall{
		Type: "archive", StripComponents: 1,
		Files: []schema.ExternalInstallFile{{From: "bin/tool", To: destination, Mode: "0755"}},
	})
	if err != nil {
		t.Fatalf("installArchive() error: %v", err)
	}
	defer restore()
	if err := finish(); err != nil {
		t.Fatalf("finish() error: %v", err)
	}
	got, err := os.ReadFile(destination)
	if err != nil || string(got) != "binary" || len(receipts) != 1 {
		t.Fatalf("content=%q receipts=%+v error=%v", got, receipts, err)
	}
}

func TestInstallCompressedTarFormats(t *testing.T) {
	for _, suffix := range []string{".tar.xz", ".tar.zst"} {
		t.Run(suffix, func(t *testing.T) {
			archivePath := filepath.Join(t.TempDir(), "tool"+suffix)
			writeCompressedTarFixture(t, archivePath, suffix)
			destination := filepath.Join(t.TempDir(), "tool")
			_, restore, finish, err := installArchive(archivePath, filepath.Base(archivePath), schema.ExternalInstall{
				Type: "archive", Files: []schema.ExternalInstallFile{{From: "tool", To: destination}},
			})
			if err != nil {
				t.Fatalf("installArchive() error: %v", err)
			}
			defer restore()
			if err := finish(); err != nil {
				t.Fatal(err)
			}
			if data, err := os.ReadFile(destination); err != nil || string(data) != "binary" {
				t.Fatalf("content=%q error=%v", data, err)
			}
		})
	}
}

func TestInstallArchiveRejectsTraversalEvenWhenNotSelected(t *testing.T) {
	archivePath := filepath.Join(t.TempDir(), "tool.zip")
	writeZipFixture(t, archivePath, map[string]string{"../escape": "bad", "tool": "good"})
	_, _, _, err := installArchive(archivePath, "tool.zip", schema.ExternalInstall{
		Type: "archive", Files: []schema.ExternalInstallFile{{From: "tool", To: filepath.Join(t.TempDir(), "tool")}},
	})
	if err == nil {
		t.Fatal("installArchive() accepted traversal entry")
	}
}

func writeZipFixture(t *testing.T, path string, files map[string]string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	for name, content := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}

func writeCompressedTarFixture(t *testing.T, path, suffix string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	var writer io.WriteCloser
	switch suffix {
	case ".tar.xz":
		writer, err = xz.NewWriter(f)
	case ".tar.zst":
		writer, err = zstd.NewWriter(f)
	}
	if err != nil {
		t.Fatal(err)
	}
	tw := tar.NewWriter(writer)
	content := []byte("binary")
	if err := tw.WriteHeader(&tar.Header{Name: "tool", Mode: 0o755, Size: int64(len(content))}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}
