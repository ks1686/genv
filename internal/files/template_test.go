package files

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestRenderString_Home(t *testing.T) {
	home := t.TempDir()
	setTestHome(t, home)

	got, err := RenderString("path = __HOME__/.config")
	if err != nil {
		t.Fatalf("RenderString error = %v", err)
	}
	want := home + "/.config"
	if got != "path = "+want {
		t.Fatalf("RenderString = %q, want %q", got, "path = "+want)
	}
}

func TestRenderString_UnknownPlaceholderLeftLiteral(t *testing.T) {
	got, err := RenderString("keep __UNKNOWN__ and __HOME__")
	if err != nil {
		t.Fatalf("RenderString error = %v", err)
	}
	if !strings.Contains(got, "__UNKNOWN__") {
		t.Fatalf("unknown placeholder should be preserved, got %q", got)
	}
	if strings.Contains(got, "__HOME__") {
		t.Fatalf("__HOME__ should be replaced, got %q", got)
	}
}

func TestRenderString_AllPlaceholders(t *testing.T) {
	home := t.TempDir()
	setTestHome(t, home)

	input := "home=__HOME__ user=__USER__ host=__HOST__ os=__OS__ arch=__ARCH__"
	got, err := RenderString(input)
	if err != nil {
		t.Fatalf("RenderString error = %v", err)
	}
	if !strings.Contains(got, "home="+home) {
		t.Fatalf("home not rendered, got %q", got)
	}
	if !strings.Contains(got, "os="+runtime.GOOS) {
		t.Fatalf("os not rendered, got %q", got)
	}
	if !strings.Contains(got, "arch="+runtime.GOARCH) {
		t.Fatalf("arch not rendered, got %q", got)
	}
	if strings.Contains(got, "__HOME__") || strings.Contains(got, "__OS__") || strings.Contains(got, "__ARCH__") {
		t.Fatalf("placeholders should be replaced, got %q", got)
	}
}

func TestRenderTemplate_CreatesMissingTarget(t *testing.T) {
	home := t.TempDir()
	setTestHome(t, home)

	src := filepath.Join(home, "src", "config.toml")
	if err := os.MkdirAll(filepath.Dir(src), 0o755); err != nil {
		t.Fatalf("mkdir src: %v", err)
	}
	if err := os.WriteFile(src, []byte("home = __HOME__\n"), 0o640); err != nil {
		t.Fatalf("write src: %v", err)
	}

	dst := filepath.Join(home, ".config", "codex", "config.toml")
	if err := RenderTemplate(src, dst, "any", RenderOptions{}); err != nil {
		t.Fatalf("RenderTemplate error = %v", err)
	}

	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("read dst: %v", err)
	}
	want := "home = " + home + "\n"
	if string(got) != want {
		t.Fatalf("dst = %q, want %q", got, want)
	}
	fi, err := os.Stat(dst)
	if err != nil {
		t.Fatalf("stat dst: %v", err)
	}
	if runtime.GOOS != "windows" && fi.Mode().Perm() != 0o640 {
		t.Fatalf("dst mode = %o, want 640", fi.Mode().Perm())
	}
}

func TestRenderTemplate_SkipsMatchingContent(t *testing.T) {
	home := t.TempDir()
	setTestHome(t, home)

	src := filepath.Join(home, "src", "config.toml")
	if err := os.MkdirAll(filepath.Dir(src), 0o755); err != nil {
		t.Fatalf("mkdir src: %v", err)
	}
	if err := os.WriteFile(src, []byte("home = __HOME__\n"), 0o644); err != nil {
		t.Fatalf("write src: %v", err)
	}

	dst := filepath.Join(home, ".config", "codex", "config.toml")
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		t.Fatalf("mkdir dst parent: %v", err)
	}
	want := "home = " + home + "\n"
	if err := os.WriteFile(dst, []byte(want), 0o644); err != nil {
		t.Fatalf("write dst: %v", err)
	}

	if err := RenderTemplate(src, dst, "any", RenderOptions{}); err != nil {
		t.Fatalf("RenderTemplate error = %v", err)
	}
}

func TestRenderTemplate_MismatchNoForce(t *testing.T) {
	home := t.TempDir()
	setTestHome(t, home)

	src := filepath.Join(home, "src", "config.toml")
	if err := os.MkdirAll(filepath.Dir(src), 0o755); err != nil {
		t.Fatalf("mkdir src: %v", err)
	}
	if err := os.WriteFile(src, []byte("home = __HOME__\n"), 0o644); err != nil {
		t.Fatalf("write src: %v", err)
	}

	dst := filepath.Join(home, ".config", "codex", "config.toml")
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		t.Fatalf("mkdir dst parent: %v", err)
	}
	original := []byte("home = /old/home\n")
	if err := os.WriteFile(dst, original, 0o644); err != nil {
		t.Fatalf("write dst: %v", err)
	}

	err := RenderTemplate(src, dst, "any", RenderOptions{})
	if !errors.Is(err, ErrMismatch) {
		t.Fatalf("RenderTemplate error = %v, want ErrMismatch", err)
	}

	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("read dst: %v", err)
	}
	if !bytes.Equal(got, original) {
		t.Fatalf("dst was modified: got %q, want %q", got, original)
	}
}

func TestRenderTemplate_MismatchForceBackup(t *testing.T) {
	home := t.TempDir()
	setTestHome(t, home)

	src := filepath.Join(home, "repo", "codex-config.toml")
	if err := os.MkdirAll(filepath.Dir(src), 0o755); err != nil {
		t.Fatalf("mkdir src: %v", err)
	}
	if err := os.WriteFile(src, []byte("[core]\nhome = \"__HOME__\"\nuser = \"__USER__\"\n"), 0o644); err != nil {
		t.Fatalf("write src: %v", err)
	}

	dst := filepath.Join(home, ".config", "codex", "config.toml")
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		t.Fatalf("mkdir dst parent: %v", err)
	}
	original := []byte("[core]\nhome = \"/old/home\"\nuser = \"olduser\"\n")
	if err := os.WriteFile(dst, original, 0o644); err != nil {
		t.Fatalf("write dst: %v", err)
	}

	if err := RenderTemplate(src, dst, "any", RenderOptions{Force: true, Backup: true}); err != nil {
		t.Fatalf("RenderTemplate error = %v", err)
	}

	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("read dst: %v", err)
	}
	if !strings.Contains(string(got), home) {
		t.Fatalf("dst should contain home %q, got %q", home, got)
	}
	if strings.Contains(string(got), "__HOME__") {
		t.Fatalf("dst should not contain literal __HOME__, got %q", got)
	}

	matches, err := filepath.Glob(dst + ".backup.*")
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
	if !bytes.Equal(backupData, original) {
		t.Fatalf("backup data mismatch: got %q, want %q", backupData, original)
	}
}

func TestRenderTemplate_MismatchForceNoBackup(t *testing.T) {
	home := t.TempDir()
	setTestHome(t, home)

	src := filepath.Join(home, "src", "config.toml")
	if err := os.MkdirAll(filepath.Dir(src), 0o755); err != nil {
		t.Fatalf("mkdir src: %v", err)
	}
	if err := os.WriteFile(src, []byte("home = __HOME__\n"), 0o644); err != nil {
		t.Fatalf("write src: %v", err)
	}

	dst := filepath.Join(home, ".config", "codex", "config.toml")
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		t.Fatalf("mkdir dst parent: %v", err)
	}
	if err := os.WriteFile(dst, []byte("home = /old/home\n"), 0o644); err != nil {
		t.Fatalf("write dst: %v", err)
	}

	if err := RenderTemplate(src, dst, "any", RenderOptions{Force: true, Backup: false}); err != nil {
		t.Fatalf("RenderTemplate error = %v", err)
	}

	matches, err := filepath.Glob(dst + ".backup.*")
	if err != nil {
		t.Fatalf("glob backups: %v", err)
	}
	if len(matches) != 0 {
		t.Fatalf("expected no backups, got %v", matches)
	}
}

func TestRenderTemplate_DryRun(t *testing.T) {
	home := t.TempDir()
	setTestHome(t, home)

	src := filepath.Join(home, "src", "config.toml")
	if err := os.MkdirAll(filepath.Dir(src), 0o755); err != nil {
		t.Fatalf("mkdir src: %v", err)
	}
	if err := os.WriteFile(src, []byte("home = __HOME__\n"), 0o644); err != nil {
		t.Fatalf("write src: %v", err)
	}

	dst := filepath.Join(home, ".config", "codex", "config.toml")
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		t.Fatalf("mkdir dst parent: %v", err)
	}
	original := []byte("home = /old/home\n")
	if err := os.WriteFile(dst, original, 0o644); err != nil {
		t.Fatalf("write dst: %v", err)
	}

	if err := RenderTemplate(src, dst, "any", RenderOptions{Force: true, Backup: true, DryRun: true}); err != nil {
		t.Fatalf("RenderTemplate error = %v", err)
	}

	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("read dst: %v", err)
	}
	if !bytes.Equal(got, original) {
		t.Fatalf("dst was modified during dry-run: got %q, want %q", got, original)
	}
	matches, err := filepath.Glob(dst + ".backup.*")
	if err != nil {
		t.Fatalf("glob backups: %v", err)
	}
	if len(matches) != 0 {
		t.Fatalf("expected no backups during dry-run, got %v", matches)
	}
}

func TestRenderTemplate_ExpandsHomeInTarget(t *testing.T) {
	home := t.TempDir()
	setTestHome(t, home)

	src := filepath.Join(home, "src", "config.toml")
	if err := os.MkdirAll(filepath.Dir(src), 0o755); err != nil {
		t.Fatalf("mkdir src: %v", err)
	}
	if err := os.WriteFile(src, []byte("ok\n"), 0o644); err != nil {
		t.Fatalf("write src: %v", err)
	}

	if err := RenderTemplate(src, "~/.config/codex/config.toml", "any", RenderOptions{}); err != nil {
		t.Fatalf("RenderTemplate error = %v", err)
	}

	dst := filepath.Join(home, ".config", "codex", "config.toml")
	if _, err := os.Stat(dst); err != nil {
		t.Fatalf("target should exist at expanded path: %v", err)
	}
}

func TestRenderTemplate_S5DriftScenario(t *testing.T) {
	home := t.TempDir()
	setTestHome(t, home)

	repo := filepath.Join(home, "repo")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatalf("mkdir repo: %v", err)
	}
	src := filepath.Join(repo, "codex-config.toml")
	srcData := "[core]\nhome = \"__HOME__\"\nuser = \"__USER__\"\n"
	if err := os.WriteFile(src, []byte(srcData), 0o644); err != nil {
		t.Fatalf("write src: %v", err)
	}

	dst := filepath.Join(home, ".config", "codex", "config.toml")
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		t.Fatalf("mkdir dst parent: %v", err)
	}
	oldRendered := []byte("[core]\nhome = \"/old/home\"\nuser = \"olduser\"\n")
	if err := os.WriteFile(dst, oldRendered, 0o644); err != nil {
		t.Fatalf("write old rendered file: %v", err)
	}

	if err := RenderTemplate(src, dst, "any", RenderOptions{Force: true, Backup: true}); err != nil {
		t.Fatalf("RenderTemplate error = %v", err)
	}

	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("read rendered file: %v", err)
	}
	if !strings.Contains(string(got), home) {
		t.Fatalf("rendered file should contain HOME %q, got: %q", home, got)
	}
	if strings.Contains(string(got), "__HOME__") {
		t.Fatalf("rendered file should not contain literal __HOME__, got: %q", got)
	}

	matches, err := filepath.Glob(dst + ".backup.*")
	if err != nil {
		t.Fatalf("glob backups: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected exactly one backup file, got %v", matches)
	}
}
