package selfpath

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPreferStableAndBrewStableBin(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "genv")
	if err := os.WriteFile(exe, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := PreferStable(exe, "", func(string) (string, error) { return exe, nil }); got != exe {
		t.Fatalf("PATH stable=%q", got)
	}
	if got := PreferStable(exe, "missing", func(string) (string, error) { return "", os.ErrNotExist }); got != exe {
		t.Fatalf("missing arg0=%q", got)
	}
	if got := brewStableBin("/opt/homebrew/Caskroom/genv/4.7.0/genv"); got != "/opt/homebrew/bin/genv" {
		t.Fatalf("cask=%q", got)
	}
	if got := brewStableBin("/opt/homebrew/Cellar/genv/4.7.0/bin/genv"); got != "/opt/homebrew/bin/genv" {
		t.Fatalf("cellar=%q", got)
	}
	if got := brewStableBin("/usr/local/bin/genv"); got != "" {
		t.Fatalf("nonbrew=%q", got)
	}
}
