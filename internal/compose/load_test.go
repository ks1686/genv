package compose

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeModule(t *testing.T, root, rel, body string) {
	t.Helper()
	full := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", rel, err)
	}
}

func TestLoadDocument_reads_valid_module(t *testing.T) {
	root := t.TempDir()
	writeModule(t, root, "modules/proxy.json", `{"schemaVersion":"10","defaults":{"packages":[{"id":"git"}]}}`)

	loaded, err := LoadDocument(root, "modules/proxy.json")
	if err != nil {
		t.Fatalf("LoadDocument() error: %v", err)
	}
	if loaded.RelPath != "modules/proxy.json" {
		t.Errorf("RelPath = %q", loaded.RelPath)
	}
	if loaded.Doc == nil || loaded.Doc.SchemaVersion != "10" {
		t.Errorf("Doc = %+v", loaded.Doc)
	}
}

func TestLoadDocument_rejects_symlinked_module(t *testing.T) {
	root := t.TempDir()
	writeModule(t, root, "modules/real.json", `{"schemaVersion":"10","defaults":{}}`)

	outside := t.TempDir()
	writeModule(t, outside, "evil.json", `{"schemaVersion":"10","defaults":{}}`)
	link := filepath.Join(root, "modules", "link.json")
	if err := os.Symlink(filepath.Join(outside, "evil.json"), link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	_, err := LoadDocument(root, "modules/link.json")
	if err == nil {
		t.Fatal("expected error for symlinked module document")
	}
	assertIsError(t, err, ErrPathEscape)
}

func TestLoadDocument_rejects_symlinked_directory_component(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	writeModule(t, outside, "evil.json", `{"schemaVersion":"10","defaults":{}}`)
	if err := os.Symlink(outside, filepath.Join(root, "modules")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	_, err := LoadDocument(root, "modules/evil.json")
	if err == nil {
		t.Fatal("expected error for symlinked parent directory")
	}
	assertIsError(t, err, ErrPathEscape)
}

func TestLoadDocument_rejects_traversal(t *testing.T) {
	root := t.TempDir()
	for _, rel := range []string{"../outside.json", "modules/../../outside.json", "/etc/passwd", "~/m.json"} {
		t.Run(rel, func(t *testing.T) {
			_, err := LoadDocument(root, rel)
			if err == nil {
				t.Fatal("expected error")
			}
			assertIsError(t, err, ErrPathEscape)
		})
	}
}

func TestLoadDocument_rejects_missing_file(t *testing.T) {
	_, err := LoadDocument(t.TempDir(), "modules/absent.json")
	if err == nil {
		t.Fatal("expected error")
	}
	assertIsError(t, err, ErrMissingModule)
}

func TestLoadDocument_rejects_directory(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "modules", "dir.json"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	_, err := LoadDocument(root, "modules/dir.json")
	if err == nil {
		t.Fatal("expected error for directory in place of a module document")
	}
	assertIsError(t, err, ErrPathEscape)
}

func TestLoadDocument_rejects_oversize(t *testing.T) {
	root := t.TempDir()
	body := `{"schemaVersion":"10","defaults":{}}`
	writeModule(t, root, "modules/big.json", body)

	old := maxModuleBytes
	maxModuleBytes = len(body) - 1
	defer func() { maxModuleBytes = old }()

	_, err := LoadDocument(root, "modules/big.json")
	if err == nil {
		t.Fatal("expected oversize error")
	}
	assertIsError(t, err, ErrLimit)
}

func TestLoadDocument_reports_invalid_module(t *testing.T) {
	root := t.TempDir()
	writeModule(t, root, "modules/bad.json", `{"schemaVersion":"9","defaults":{}}`)

	_, err := LoadDocument(root, "modules/bad.json")
	if err == nil {
		t.Fatal("expected validation error for a v9 module document")
	}
	assertIsError(t, err, ErrInvalidModule)
	if !strings.Contains(err.Error(), "schemaVersion") {
		t.Errorf("error should name the offending field, got: %v", err)
	}
}

func TestLoadAllRegistered_loads_every_registered_module(t *testing.T) {
	root := t.TempDir()
	writeModule(t, root, "modules/a.json", `{"schemaVersion":"10","defaults":{"packages":[{"id":"a"}]}}`)
	writeModule(t, root, "nested/b.json", `{"schemaVersion":"10","defaults":{"packages":[{"id":"b"}]}}`)

	docs, err := LoadAllRegistered(root, map[string]string{
		"alpha": "modules/a.json",
		"beta":  "nested/b.json",
	})
	if err != nil {
		t.Fatalf("LoadAllRegistered() error: %v", err)
	}
	if len(docs) != 2 {
		t.Fatalf("loaded %d modules, want 2", len(docs))
	}
	if docs["alpha"].Doc.Defaults.Packages[0].ID != "a" {
		t.Errorf("alpha decoded wrong: %+v", docs["alpha"].Doc.Defaults)
	}
	if docs["beta"].AbsPath == "" || !filepath.IsAbs(docs["beta"].AbsPath) {
		t.Errorf("AbsPath = %q, want absolute", docs["beta"].AbsPath)
	}
}

func TestLoadAllRegistered_reports_missing_registration(t *testing.T) {
	root := t.TempDir()
	_, err := LoadAllRegistered(root, map[string]string{"ghost": "modules/ghost.json"})
	if err == nil {
		t.Fatal("expected error for unregistered file")
	}
	assertIsError(t, err, ErrMissingModule)
	if !strings.Contains(err.Error(), "ghost") {
		t.Errorf("error should name the module, got: %v", err)
	}
}

func TestLoadAllRegistered_rejects_invalid_path_syntax(t *testing.T) {
	root := t.TempDir()
	_, err := LoadAllRegistered(root, map[string]string{"bad": "../escape.json"})
	if err == nil {
		t.Fatal("expected error")
	}
	assertIsError(t, err, ErrPathEscape)
}

func TestLoadAllRegistered_enforces_count_limit(t *testing.T) {
	root := t.TempDir()
	old := maxRegisteredModules
	maxRegisteredModules = 1
	defer func() { maxRegisteredModules = old }()

	registered := map[string]string{"a": "modules/a.json", "b": "modules/b.json"}
	_, err := LoadAllRegistered(root, registered)
	if err == nil {
		t.Fatal("expected limit error")
	}
	assertIsError(t, err, ErrLimit)
}

func TestLoadAllRegistered_empty_registry_is_noop(t *testing.T) {
	docs, err := LoadAllRegistered(t.TempDir(), nil)
	if err != nil {
		t.Fatalf("LoadAllRegistered() error: %v", err)
	}
	if len(docs) != 0 {
		t.Fatalf("expected no modules, got %d", len(docs))
	}
}

func TestLoadDocument_rejects_empty_registry_name(t *testing.T) {
	root := t.TempDir()
	writeModule(t, root, "modules/a.json", `{"schemaVersion":"10","defaults":{}}`)
	if _, err := LoadAllRegistered(root, map[string]string{"Bad Name": "modules/a.json"}); err == nil {
		t.Fatal("expected error for invalid module name")
	}
}
