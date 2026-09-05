package genvfile

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ks1686/genv/internal/schema"
)

const rewriteFixture = `{
  "updates": {},
  "defaults": {
    "env": {},
    "shell": {
      "aliases": {},
      "functions": {}
    },
    "services": {},
    "files": {
      "links": [],
      "templates": [],
      "dirs": []
    },
    "hooks": {
      "preApply": [],
      "postApply": [],
      "preAdd": [],
      "postAdd": [],
      "preRemove": [],
      "postRemove": [],
      "preUpgrade": [],
      "postUpgrade": []
    }
  },
  "targets": {
    "windows": {
      "env": {},
      "shell": {},
      "services": {},
      "files": {},
      "hooks": {}
    },
    "macos": {
      "packages": [
        {
          "prefer": "brew",
          "id": "git"
        }
      ],
      "env": {},
      "shell": {},
      "services": {},
      "files": {},
      "hooks": {}
    }
  },
  "schemaVersion": "8"
}
`

func TestWrite_PackageEditPreservesEmptyBlocks(t *testing.T) {
	path := filepath.Join(t.TempDir(), "genv.json")
	if err := os.WriteFile(path, []byte(rewriteFixture), 0o644); err != nil {
		t.Fatal(err)
	}
	f, err := Read(path)
	if err != nil {
		t.Fatal(err)
	}
	f.Targets["macos"].Packages = append(f.Targets["macos"].Packages, schema.Package{ID: "bash"})
	if err := Write(path, f); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Replace(rewriteFixture, `{
          "prefer": "brew",
          "id": "git"
        }`, `{
          "prefer": "brew",
          "id": "git"
        },
        {
          "id": "bash"
        }`, 1)
	if string(got) != want {
		t.Fatalf("Write changed more than the new package\nwant:\n%s\ngot:\n%s", want, got)
	}
}

func TestWrite_PackageRemovePreservesEmptyBlocks(t *testing.T) {
	path := filepath.Join(t.TempDir(), "genv.json")
	two := strings.Replace(rewriteFixture, `{
          "prefer": "brew",
          "id": "git"
        }`, `{
          "prefer": "brew",
          "id": "git"
        },
        {
          "id": "neovim"
        }`, 1)
	if err := os.WriteFile(path, []byte(two), 0o644); err != nil {
		t.Fatal(err)
	}
	f, err := Read(path)
	if err != nil {
		t.Fatal(err)
	}
	f.Targets["macos"].Packages = f.Targets["macos"].Packages[:1]
	if err := Write(path, f); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != rewriteFixture {
		t.Fatalf("Write changed more than the removed package\nwant:\n%s\ngot:\n%s", rewriteFixture, got)
	}
}

func TestWrite_InsertsPackagesKeyInEmptyTarget(t *testing.T) {
	path := filepath.Join(t.TempDir(), "genv.json")
	original := `{"schemaVersion":"8","targets":{"arch":{},"macos":{"services":{}}}}` + "\n"
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	f, err := Read(path)
	if err != nil {
		t.Fatal(err)
	}
	f.Targets["arch"].Packages = []schema.Package{{ID: "git", Prefer: "brew"}}
	if err := Write(path, f); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"schemaVersion":"8","targets":{"arch":{"packages":[{"id":"git","prefer":"brew"}]},"macos":{"services":{}}}}` + "\n"
	if string(got) != want {
		t.Fatalf("compact insert\nwant %s\ngot  %s", want, got)
	}
}

func TestWrite_FallsBackWhenNonPackageFieldsChange(t *testing.T) {
	path := filepath.Join(t.TempDir(), "genv.json")
	if err := os.WriteFile(path, []byte(rewriteFixture), 0o644); err != nil {
		t.Fatal(err)
	}
	f, err := Read(path)
	if err != nil {
		t.Fatal(err)
	}
	f.Updates.Enabled = true
	f.Updates.Interval = "1h"
	if err := Write(path, f); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(got), `"services": {}`) {
		t.Fatalf("fallback remashal should drop empty maps, got:\n%s", got)
	}
	if !strings.Contains(string(got), `"enabled": true`) {
		t.Fatalf("fallback remashal missing updates change:\n%s", got)
	}
}

func TestRewritePackagesInPlace_RejectsInvalidOriginal(t *testing.T) {
	if _, ok := rewritePackagesInPlace([]byte(`{`), &schema.GenvFile{SchemaVersion: schema.Version8}); ok {
		t.Fatal("expected invalid JSON to fail rewrite")
	}
}

func TestLocateJSONSpans_NestedPackages(t *testing.T) {
	spans := locateJSONSpans([]byte(rewriteFixture))
	if spans == nil {
		t.Fatal("locateJSONSpans returned nil")
	}
	if _, ok := spans["targets.macos.packages"]; !ok {
		t.Fatal("missing targets.macos.packages span")
	}
	if _, ok := spans["targets.macos.packages[0]"]; !ok {
		t.Fatal("missing first package span")
	}
	if _, ok := spans["defaults.shell.aliases"]; !ok {
		t.Fatal("missing defaults.shell.aliases span")
	}
}

func TestWrite_TopLevelPackagesPreserveEmptyBlocks(t *testing.T) {
	path := filepath.Join(t.TempDir(), "genv.json")
	original := `{
  "schemaVersion": "4",
  "env": {},
  "shell": {},
  "services": {},
  "packages": [
    {
      "id": "git"
    }
  ]
}
`
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	f, err := Read(path)
	if err != nil {
		t.Fatal(err)
	}
	f.Packages = append(f.Packages, schema.Package{ID: "bash"})
	if err := Write(path, f); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Replace(original, `{
      "id": "git"
    }`, `{
      "id": "git"
    },
    {
      "id": "bash"
    }`, 1)
	if string(got) != want {
		t.Fatalf("v4 top-level rewrite\nwant:\n%s\ngot:\n%s", want, got)
	}
}

func TestWrite_InsertsPackagesIntoPrettyEmptyTarget(t *testing.T) {
	path := filepath.Join(t.TempDir(), "genv.json")
	original := `{
  "schemaVersion": "8",
  "targets": {
    "macos": {
      "services": {},
      "shell": {}
    }
  }
}
`
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	f, err := Read(path)
	if err != nil {
		t.Fatal(err)
	}
	f.Targets["macos"].Packages = []schema.Package{{ID: "bash"}}
	if err := Write(path, f); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := `{
  "schemaVersion": "8",
  "targets": {
    "macos": {
      "services": {},
      "shell": {},
      "packages": [
        {
          "id": "bash"
        }
      ]
    }
  }
}
`
	if string(got) != want {
		t.Fatalf("pretty insert\nwant:\n%s\ngot:\n%s", want, got)
	}
}

func TestWrite_InsertsPackagesIntoPrettyEmptyObject(t *testing.T) {
	path := filepath.Join(t.TempDir(), "genv.json")
	original := `{
  "schemaVersion": "8",
  "targets": {
    "macos": {
    }
  }
}
`
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	f, err := Read(path)
	if err != nil {
		t.Fatal(err)
	}
	f.Targets["macos"].Packages = []schema.Package{{ID: "bash"}}
	if err := Write(path, f); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := `{
  "schemaVersion": "8",
  "targets": {
    "macos": {
      "packages": [
        {
          "id": "bash"
        }
      ]
    }
  }
}
`
	if string(got) != want {
		t.Fatalf("pretty empty object insert\nwant:\n%s\ngot:\n%s", want, got)
	}
}

func TestWrite_AppendsToEmptyPackagesArray(t *testing.T) {
	path := filepath.Join(t.TempDir(), "genv.json")
	original := `{
  "schemaVersion": "8",
  "targets": {
    "macos": {
      "packages": [],
      "services": {}
    }
  }
}
`
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	f, err := Read(path)
	if err != nil {
		t.Fatal(err)
	}
	f.Targets["macos"].Packages = []schema.Package{{ID: "bash"}}
	if err := Write(path, f); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), `"services": {}`) {
		t.Fatalf("empty-array append dropped services:\n%s", got)
	}
	want := `{
  "schemaVersion": "8",
  "targets": {
    "macos": {
      "packages": [
        {
          "id": "bash"
        }
      ],
      "services": {}
    }
  }
}
`
	if string(got) != want {
		t.Fatalf("empty-array append\nwant:\n%s\ngot:\n%s", want, got)
	}
}

func TestWrite_RemovesLastPackageLeavesEmptyArray(t *testing.T) {
	path := filepath.Join(t.TempDir(), "genv.json")
	original := `{
  "schemaVersion": "8",
  "targets": {
    "macos": {
      "packages": [
        {
          "id": "git"
        }
      ],
      "services": {}
    }
  }
}
`
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	f, err := Read(path)
	if err != nil {
		t.Fatal(err)
	}
	f.Targets["macos"].Packages = f.Targets["macos"].Packages[:0]
	if err := Write(path, f); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), `"packages": []`) {
		t.Fatalf("expected empty packages array, got:\n%s", got)
	}
	if !strings.Contains(string(got), `"services": {}`) {
		t.Fatalf("last-package remove dropped services:\n%s", got)
	}
}

func TestWrite_NoPackageChangeLeavesBytesUntouched(t *testing.T) {
	path := filepath.Join(t.TempDir(), "genv.json")
	if err := os.WriteFile(path, []byte(rewriteFixture), 0o644); err != nil {
		t.Fatal(err)
	}
	f, err := Read(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := Write(path, f); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != rewriteFixture {
		t.Fatalf("no-op Write changed bytes\nwant:\n%s\ngot:\n%s", rewriteFixture, got)
	}
}

func TestWrite_PreservesCRLF(t *testing.T) {
	path := filepath.Join(t.TempDir(), "genv.json")
	original := strings.ReplaceAll(`{
  "schemaVersion": "8",
  "targets": {
    "macos": {
      "packages": [
        {
          "id": "git"
        }
      ],
      "services": {}
    }
  }
}
`, "\n", "\r\n")
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	f, err := Read(path)
	if err != nil {
		t.Fatal(err)
	}
	f.Targets["macos"].Packages = append(f.Targets["macos"].Packages, schema.Package{ID: "bash"})
	if err := Write(path, f); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(got, []byte("\r\n")) {
		t.Fatalf("CRLF was rewritten to LF:\n%q", got)
	}
	if !bytes.Contains(got, []byte(`"services": {}`)) {
		t.Fatalf("CRLF rewrite dropped services:\n%s", got)
	}
}

func TestWrite_DefaultsPackages(t *testing.T) {
	path := filepath.Join(t.TempDir(), "genv.json")
	original := `{
  "schemaVersion": "8",
  "defaults": {
    "packages": [
      {
        "id": "git"
      }
    ],
    "services": {}
  },
  "targets": {
    "macos": {}
  }
}
`
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	f, err := Read(path)
	if err != nil {
		t.Fatal(err)
	}
	f.Defaults.Packages = append(f.Defaults.Packages, schema.Package{ID: "bash"})
	if err := Write(path, f); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), `"id": "bash"`) {
		t.Fatalf("defaults.packages missing bash:\n%s", got)
	}
	if !strings.Contains(string(got), `"services": {}`) {
		t.Fatalf("defaults.packages rewrite dropped services:\n%s", got)
	}
}

func TestRewriteHelpers(t *testing.T) {
	if parentPath("packages") != "" {
		t.Fatalf("parentPath(packages) = %q", parentPath("packages"))
	}
	if locateJSONSpans(nil) != nil || locateJSONSpans([]byte(`[]`)) != nil || locateJSONSpans([]byte(`"x"`)) != nil {
		t.Fatal("locateJSONSpans should reject non-objects")
	}
	if packageID([]byte(`[]`)) != "" {
		t.Fatal("packageID should reject non-objects")
	}
	if packageListAt(&schema.GenvFile{}, "unknown.path") != nil {
		t.Fatal("packageListAt unknown path")
	}
	f := &schema.GenvFile{SchemaVersion: schema.Version8}
	if _, ok := rewritePackagesInPlace([]byte(rewriteFixture), f); ok {
		t.Fatal("rewrite should refuse a structurally different spec")
	}
}
