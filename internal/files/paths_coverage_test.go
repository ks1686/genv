package files

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveSourceAndExpandPath(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", root)
	t.Setenv("GENV_PATH_TEST", "value")
	if got, err := ResolveSource(root, "dir/file"); err != nil || got != filepath.Join(root, "dir", "file") {
		t.Fatalf("relative=%q,%v", got, err)
	}
	if _, err := ResolveSource(root, "../escape"); err == nil {
		t.Fatal("escape accepted")
	}
	if _, err := ResolveSource("", "relative"); err == nil {
		t.Fatal("relative source without root accepted")
	}
	if got, err := ExpandPath("~/x/$GENV_PATH_TEST"); err != nil || got != filepath.Join(root, "x", "value") {
		t.Fatalf("expand=%q,%v", got, err)
	}
}

func TestRemovePathAndPermissionHelpers(t *testing.T) {
	got := removePath([]string{"a", "b", "a"}, "a")
	if len(got) != 1 || got[0] != "b" {
		t.Fatalf("remove=%v", got)
	}
	p := filepath.Join(t.TempDir(), "f")
	if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	changed, err := applyPermIfNeeded(p, "0755", ApplyOptions{DryRun: true})
	if err != nil || !changed {
		t.Fatalf("dry=%v,%v", changed, err)
	}
	changed, err = applyPermIfNeeded(p, "0755", ApplyOptions{})
	if err != nil || !changed {
		t.Fatalf("apply=%v,%v", changed, err)
	}
	changed, err = applyPermIfNeeded(p, "0755", ApplyOptions{})
	if err != nil || changed {
		t.Fatalf("noop=%v,%v", changed, err)
	}
}
