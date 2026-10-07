package genvfile

import (
	"os"
	"path/filepath"
	"testing"
)

// A v10 spec's module registry and per-bundle selection are non-package state.
// If the in-place package rewrite ignores them, Write reports success while
// dropping the change — the silent-data-loss trap this test pins shut.
func TestRewritePackagesInPlace_compares_module_state(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "genv.json")
	original := `{
  "schemaVersion": "10",
  "modules": {
    "base": "modules/base.json"
  },
  "defaults": {
    "useModules": [
      "base"
    ]
  },
  "targets": {
    "macos": {
      "packages": [
        {
          "id": "jq"
        }
      ]
    }
  }
}
`
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatalf("write spec: %v", err)
	}

	// A package-only change must still take the in-place path and keep modules.
	before, errs, parseErr := schemaParse(original)
	if parseErr != nil || len(errs) > 0 {
		t.Fatalf("fixture invalid: %v %v", parseErr, errs)
	}
	before.Targets["macos"].Packages = append(before.Targets["macos"].Packages, packageWithID("ripgrep"))
	if err := Write(path, before); err != nil {
		t.Fatalf("Write: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if !containsAll(string(data), `"modules"`, "modules/base.json", `"useModules"`, "ripgrep") {
		t.Fatalf("package rewrite dropped module state:\n%s", data)
	}

	// A module-only change must NOT take the in-place path, or it would be lost.
	after, _, _ := schemaParse(string(data))
	after.Modules["extra"] = "modules/extra.json"
	if err := Write(path, after); err != nil {
		t.Fatalf("Write: %v", err)
	}
	data, err = os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if !containsAll(string(data), "modules/extra.json", "ripgrep") {
		t.Fatalf("module change was dropped:\n%s", data)
	}
}

func TestRewritePackagesInPlace_detects_selection_change(t *testing.T) {
	a := specWithSelection([]string{"base"})
	b := specWithSelection([]string{"base", "extra"})
	if sameNonPackageState(a, b) {
		t.Error("a useModules change must count as non-package state")
	}
}
