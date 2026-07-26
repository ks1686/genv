package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMapCmdPrintsTargetAndManagerSuggestions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "genv.json")
	writeTestFile(t, path, `{
	  "schemaVersion": "8",
	  "targets": {
	    "macos": {
	      "packages": [
	        {"id": "numbers", "prefer": "mas", "managers": {"mas": "409203825"}}
	      ]
	    }
	  }
	}`)

	var code int
	out := captureStdout(t, func() {
		code = run([]string{"map", "--file", path, "--target", "ubuntu"})
	})
	if code != exitOK {
		t.Fatalf("map = %d, want %d", code, exitOK)
	}
	if !strings.Contains(out, "targets.ubuntu does not exist") {
		t.Fatalf("missing target suggestion: %q", out)
	}
	if !strings.Contains(out, "numbers") || !strings.Contains(out, "snap") || !strings.Contains(out, "linuxbrew") {
		t.Fatalf("missing manager suggestion: %q", out)
	}
}

func TestMapCmdNoSuggestions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "genv.json")
	writeTestFile(t, path, `{
	  "schemaVersion": "8",
	  "targets": {
	    "ubuntu": {
	      "packages": [
	        {"id": "ripgrep", "prefer": "snap"}
	      ]
	    }
	  }
	}`)

	var code int
	out := captureStdout(t, func() {
		code = run([]string{"map", "--file", path, "--target", "ubuntu"})
	})
	if code != exitOK {
		t.Fatalf("map = %d, want %d", code, exitOK)
	}
	if !strings.Contains(out, "No mapping suggestions for target ubuntu.") {
		t.Fatalf("unexpected no-suggestions output: %q", out)
	}
}

func TestMapCmdValidationFailures(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "genv.json")
	writeTestFile(t, path, `{"schemaVersion":"7","packages":[]}`)

	if code := run([]string{"map", "--file", path}); code != exitUsage {
		t.Fatalf("map without target = %d, want %d", code, exitUsage)
	}
	if code := run([]string{"map", "--file", path, "--target", "ubuntu"}); code != exitUsage {
		t.Fatalf("map legacy spec = %d, want %d", code, exitUsage)
	}
	if code := run([]string{"map", "--file", filepath.Join(dir, "missing.json"), "--target", "ubuntu"}); code != exitIO {
		t.Fatalf("map missing file = %d, want %d", code, exitIO)
	}
}

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
