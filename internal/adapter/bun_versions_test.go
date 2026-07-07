package adapter

import (
	"maps"
	"os"
	"testing"
)

func TestBun_ListInstalledVersions_returnsVersionsAndExecsListOnce(t *testing.T) {
	// Given
	counterPath := t.TempDir() + "/count"
	t.Setenv("GENV_FAKE_COUNTER", counterPath)
	installFakeBinary(t, "bun",
		`if [ "$1" = "pm" ] && [ "$2" = "ls" ] && [ "$3" = "--global" ]; then
  count=$(cat "$GENV_FAKE_COUNTER" 2>/dev/null || printf 0)
  count=$((count + 1))
  printf "%s" "$count" > "$GENV_FAKE_COUNTER"
  echo "/path/global node_modules (3)"
  echo "├── add-gitignore@1.1.1"
  echo "├── @colbymchenry/codegraph@1.0.1"
  echo "└── cf@0.0.6"
fi`)

	// When
	versions, err := Bun{}.ListInstalledVersions()

	// Then
	if err != nil {
		t.Fatalf("ListInstalledVersions: %v", err)
	}
	want := map[string]string{
		"add-gitignore":           "1.1.1",
		"@colbymchenry/codegraph": "1.0.1",
		"cf":                      "0.0.6",
	}
	if !maps.Equal(versions, want) {
		t.Errorf("ListInstalledVersions = %v, want %v", versions, want)
	}
	count, err := os.ReadFile(counterPath)
	if err != nil {
		t.Fatalf("counter: %v", err)
	}
	if string(count) != "1" {
		t.Errorf("bun pm ls --global exec count = %q, want 1", string(count))
	}
}
