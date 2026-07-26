package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMapCmdPrintsSuggestionsWithoutMutatingSpec(t *testing.T) {
	dir := t.TempDir()
	specPath := filepath.Join(dir, "genv.json")
	original := `{
  "schemaVersion": "8",
  "targets": {
    "macos": {
      "packages": [
        {
          "id": "numbers",
          "prefer": "mas",
          "managers": {
            "mas": "409203825"
          }
        }
      ]
    }
  }
}
`
	writeTestFile(t, specPath, original)

	var code int
	out := captureStdout(t, func() {
		code = run([]string{"map", "--file", specPath, "--target", "ubuntu"})
	})
	if code != exitOK {
		t.Fatalf("map = %d, want %d; stdout=%s", code, exitOK, out)
	}
	for _, want := range []string{"ubuntu", "numbers", "snap", "linuxbrew", "mas"} {
		if !strings.Contains(out, want) {
			t.Fatalf("map output missing %q:\n%s", want, out)
		}
	}
	data, err := os.ReadFile(specPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != original {
		t.Fatalf("map mutated spec:\n%s", data)
	}
}
