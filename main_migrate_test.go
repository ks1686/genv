package main

import (
	"strings"
	"testing"

	"github.com/ks1686/genv/internal/genvfile"
	"github.com/ks1686/genv/internal/schema"
)

func TestMigrateCmdPrintsMigratedJSON(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/genv.json"
	writeTestFile(t, path, `{
	  "schemaVersion": "7",
	  "packages": [
	    {"id": "git", "host": "arch"},
	    {"id": "mas", "host": "macos"}
	  ],
	  "env": {"EDITOR": {"value": "nvim"}}
	}`)

	var code int
	out := captureStdout(t, func() {
		code = run([]string{"migrate", "--file", path})
	})
	if code != exitOK {
		t.Fatalf("migrate = %d, want %d", code, exitOK)
	}
	if !strings.Contains(out, `"schemaVersion": "8"`) || !strings.Contains(out, `"defaults"`) || !strings.Contains(out, `"arch"`) || !strings.Contains(out, `"macos"`) {
		t.Fatalf("unexpected migrate output:\n%s", out)
	}

	f, errs, err := schema.ParseAndValidate([]byte(out))
	if err != nil || len(errs) > 0 {
		t.Fatalf("output failed validation: err=%v errs=%v out=%s", err, errs, out)
	}
	if f.Targets["arch"] == nil || f.Targets["macos"] == nil || f.Defaults.Env["EDITOR"] == nil {
		t.Fatalf("output missing migrated buckets: %+v", f)
	}
}

func TestMigrateCmdWriteOverwritesSpec(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/genv.json"
	writeTestFile(t, path, `{"schemaVersion":"7","packages":[{"id":"git"}]}`)

	var code int
	errOut := captureStderr(t, func() {
		code = run([]string{"migrate", "--file", path, "--write"})
	})
	if code != exitOK {
		t.Fatalf("migrate --write = %d, want %d; stderr=%s", code, exitOK, errOut)
	}
	if !strings.Contains(errOut, "could not infer") {
		t.Fatalf("expected fallback warning on stderr, got %q", errOut)
	}

	f, err := genvfile.Read(path)
	if err != nil {
		t.Fatal(err)
	}
	if f.SchemaVersion != schema.Version8 {
		t.Fatalf("schemaVersion = %q, want %q", f.SchemaVersion, schema.Version8)
	}
	if f.Targets["linux"] == nil || len(f.Targets["linux"].Packages) != 1 || f.Targets["linux"].Packages[0].ID != "git" {
		t.Fatalf("linux fallback target not written: %+v", f.Targets)
	}
}
