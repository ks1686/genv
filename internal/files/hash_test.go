package files

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHashBytes_isVersionedSHA256(t *testing.T) {
	got := HashBytes([]byte("hello\n"))
	if !strings.HasPrefix(got, "sha256:") {
		t.Fatalf("HashBytes = %q, want sha256: prefix", got)
	}
	if len(got) != len("sha256:")+64 {
		t.Fatalf("HashBytes = %q, want 64 hex digits", got)
	}
	if HashBytes([]byte("hello\n")) != got {
		t.Fatal("HashBytes must be deterministic")
	}
	if HashBytes([]byte("hello\n")) == HashBytes([]byte("hello")) {
		t.Fatal("HashBytes must change when content changes")
	}
}

func TestHashFile_skipsDirectories(t *testing.T) {
	dir := t.TempDir()
	hash, err := HashFile(dir)
	if err != nil {
		t.Fatalf("HashFile(dir): %v", err)
	}
	if hash != "" {
		t.Fatalf("HashFile(dir) = %q, want empty", hash)
	}

	path := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	got, err := HashFile(path)
	if err != nil || got == "" {
		t.Fatalf("HashFile(file) = %q err=%v", got, err)
	}
}

func TestExpandTarget_cleansHomePath(t *testing.T) {
	home := t.TempDir()
	setTestHome(t, home)
	got, err := ExpandTarget("~/.npmrc")
	if err != nil {
		t.Fatalf("ExpandTarget: %v", err)
	}
	want := filepath.Join(home, ".npmrc")
	if got != want {
		t.Fatalf("ExpandTarget = %q, want %q", got, want)
	}
}

func TestHashableLinkMode(t *testing.T) {
	if !HashableLinkMode("") || !HashableLinkMode("link") || !HashableLinkMode("managed-link") {
		t.Fatal("link modes should be hashable")
	}
	if HashableLinkMode("merge-dir") || HashableLinkMode("dir") {
		t.Fatal("merge-dir and dir must not be hashed")
	}
}

func TestHashLinkSourceAndTemplate(t *testing.T) {
	home := t.TempDir()
	setTestHome(t, home)
	src := filepath.Join(home, "repo", "a.txt")
	if err := os.MkdirAll(filepath.Dir(src), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(src, []byte("body\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := HashLinkSource("", src)
	if err != nil || got == "" {
		t.Fatalf("HashLinkSource = %q err=%v", got, err)
	}
	tmpl := filepath.Join(home, "repo", "t.txt")
	if err := os.WriteFile(tmpl, []byte("home=__HOME__\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	th, err := HashTemplate("", tmpl, "any")
	if err != nil || th == "" || th == got {
		t.Fatalf("HashTemplate = %q err=%v", th, err)
	}
}
