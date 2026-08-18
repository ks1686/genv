package pull

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/ks1686/genv/internal/schema"
)

func TestCopyBundleAssetsV7CopiesRelativeFileAssets(t *testing.T) {
	cacheDir := t.TempDir()
	destDir := t.TempDir()
	writeAsset(t, cacheDir, "assets/profile", "profile")
	writeAsset(t, cacheDir, "templates/app.tmpl", "template")
	writeAsset(t, cacheDir, "secrets/token", "secret")
	writeAsset(t, cacheDir, "genv.lock.json", "lock")

	f := &schema.GenvFile{
		SchemaVersion: schema.Version7,
		Files: &schema.FilesConfig{
			Links: []schema.FileLink{
				{Source: "assets/profile", Target: "~/.profile"},
				{Source: "/absolute/skip", Target: "~/.skip"},
				{Source: "secrets/token", Target: "~/.token"},
				{Source: "genv.lock.json", Target: "~/.lock"},
			},
			Templates: []schema.FileTemplate{
				{Source: "templates/app.tmpl", Target: "~/.config/app/config"},
			},
		},
	}

	got, err := CopyBundleAssets(cacheDir, destDir, f)
	if err != nil {
		t.Fatalf("CopyBundleAssets: %v", err)
	}
	want := []string{"assets/profile", "templates/app.tmpl"}
	if !slices.Equal(got, want) {
		t.Fatalf("copied assets = %v, want %v", got, want)
	}
	assertFileContent(t, filepath.Join(destDir, "assets", "profile"), "profile")
	assertFileContent(t, filepath.Join(destDir, "templates", "app.tmpl"), "template")
	assertNotExists(t, filepath.Join(destDir, "secrets", "token"))
	assertNotExists(t, filepath.Join(destDir, "genv.lock.json"))
}

func TestCopyBundleAssets_RejectsSymlink(t *testing.T) {
	cacheDir := t.TempDir()
	destDir := t.TempDir()
	target := filepath.Join(cacheDir, "real")
	if err := os.WriteFile(target, []byte("secret"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	link := filepath.Join(cacheDir, "assets", "profile")
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	f := &schema.GenvFile{
		SchemaVersion: schema.Version7,
		Files: &schema.FilesConfig{
			Links: []schema.FileLink{{Source: "assets/profile", Target: "~/.profile"}},
		},
	}
	_, err := CopyBundleAssets(cacheDir, destDir, f)
	if err == nil {
		t.Fatal("expected symlink copy to fail")
	}
	if !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("error = %v, want symlink mention", err)
	}
	assertNotExists(t, filepath.Join(destDir, "assets", "profile"))
}

func TestCopyBundleAssetsV8UnionsDefaultsAndTargets(t *testing.T) {
	cacheDir := t.TempDir()
	destDir := t.TempDir()
	writeAsset(t, cacheDir, "defaults/profile", "default")
	writeAsset(t, cacheDir, "targets/macos.tmpl", "macos")
	writeAsset(t, cacheDir, "targets/windows/profile", "windows")

	f := &schema.GenvFile{
		SchemaVersion: schema.Version8,
		Defaults: &schema.TargetBundle{Files: &schema.FilesConfig{
			Links: []schema.FileLink{{Source: "defaults/profile", Target: "~/.profile"}},
		}},
		Targets: map[string]*schema.TargetBundle{
			"macos": {Files: &schema.FilesConfig{
				Templates: []schema.FileTemplate{{Source: "targets/macos.tmpl", Target: "~/.config/app"}},
			}},
			"windows": {Files: &schema.FilesConfig{
				Links: []schema.FileLink{
					{Source: "defaults/profile", Target: "~/duplicate"},
					{Source: `targets\windows\profile`, Target: "~/profile"},
				},
			}},
		},
	}

	got, err := CopyBundleAssets(cacheDir, destDir, f)
	if err != nil {
		t.Fatalf("CopyBundleAssets: %v", err)
	}
	want := []string{"defaults/profile", "targets/macos.tmpl", "targets/windows/profile"}
	if !slices.Equal(got, want) {
		t.Fatalf("copied assets = %v, want %v", got, want)
	}
	assertFileContent(t, filepath.Join(destDir, "defaults", "profile"), "default")
	assertFileContent(t, filepath.Join(destDir, "targets", "macos.tmpl"), "macos")
	assertFileContent(t, filepath.Join(destDir, "targets", "windows", "profile"), "windows")
}

func writeAsset(t *testing.T, root, rel, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

func assertFileContent(t *testing.T, path, want string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", path, err)
	}
	if string(got) != want {
		t.Fatalf("%s = %q, want %q", path, got, want)
	}
}

func assertNotExists(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err == nil {
		t.Fatalf("%s exists, want absent", path)
	} else if !os.IsNotExist(err) {
		t.Fatalf("Stat(%s): %v", path, err)
	}
}
