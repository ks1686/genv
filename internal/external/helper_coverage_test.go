package external

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/ks1686/genv/internal/schema"
)

func TestLocalKeyPrefersInlineAndResolvesRelativeSourceRoot(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "key.pub"), []byte("from-file"), 0o600); err != nil {
		t.Fatal(err)
	}
	engine := Engine{SourceRoot: dir}
	if key, err := engine.localKey("inline", "key.pub"); err != nil || string(key) != "inline" {
		t.Fatalf("inline key = %q, %v", key, err)
	}
	if key, err := engine.localKey("", "key.pub"); err != nil || string(key) != "from-file" {
		t.Fatalf("relative key = %q, %v", key, err)
	}
	if _, err := engine.localKey("", "missing.pub"); err == nil {
		t.Fatal("missing key file accepted")
	}
}

func TestEnsureScriptExtensionAndInterpreterResolution(t *testing.T) {
	path := filepath.Join(t.TempDir(), "installer")
	if err := os.WriteFile(path, []byte("echo ok"), 0o700); err != nil {
		t.Fatal(err)
	}
	if got, err := ensureScriptExtension(path, "sh"); err != nil || got != path {
		t.Fatalf("sh extension = %q, %v", got, err)
	}
	got, err := ensureScriptExtension(path, "pwsh")
	if err != nil || filepath.Ext(got) != ".ps1" {
		t.Fatalf("pwsh extension = %q, %v", got, err)
	}
	if _, err := os.Stat(got); err != nil {
		t.Fatalf("renamed script missing: %v", err)
	}
	if _, _, err := resolveInterpreter("not-an-interpreter"); err == nil {
		t.Fatal("unsupported interpreter accepted")
	}
	if runtime.GOOS != "windows" {
		if bin, prefix, err := resolveInterpreter("sh"); err != nil || bin == "" || len(prefix) != 0 {
			t.Fatalf("sh interpreter = %q, %v, %v", bin, prefix, err)
		}
	}
}

func TestExpandInstallTemplatesAndLatestVersionFailures(t *testing.T) {
	values := TemplateValues{Version: "1.2.3", Tag: "v1.2.3", OS: "darwin", Arch: "arm64", Script: "/tmp/x", Destination: "/tmp/y"}
	got, err := expandInstallTemplates(schema.ExternalInstall{
		Destination: "{destination}",
		Files:       []schema.ExternalInstallFile{{From: "tool", To: "bin/{version}"}},
	}, values)
	if err != nil || got.Destination != "/tmp/y" || got.Files[0].To != "bin/1.2.3" {
		t.Fatalf("expandInstallTemplates = %+v, %v", got, err)
	}
	if _, err := expandInstallTemplates(schema.ExternalInstall{Destination: "{nope}"}, values); err == nil {
		t.Fatal("unknown install template accepted")
	}
	if _, err := (Engine{}).LatestVersion(testContext(t), schema.Package{}); err == nil {
		t.Fatal("LatestVersion accepted a package without external recipe")
	}
}

func testContext(t *testing.T) context.Context {
	t.Helper()
	return context.Background()
}
