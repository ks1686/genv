package main

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/ks1686/genv/internal/genvfile"
	"github.com/ks1686/genv/internal/schema"
)

func TestExportCmdStrictReturnsNonzeroForReportErrors(t *testing.T) {
	outDir := t.TempDir()
	code := run([]string{
		"export",
		"--file", filepath.Join("testdata", "export", "multi-target", "genv.json"),
		"--target", "arch",
		"--out", outDir,
		"--strict",
	})
	if code != exitLogic {
		t.Fatalf("export --strict = %d, want %d", code, exitLogic)
	}
	if _, err := genvfile.Read(filepath.Join(outDir, "genv.json")); err != nil {
		t.Fatalf("exported snapshot should validate: %v", err)
	}
}

func TestExportCmdFromV7MigratesInMemory(t *testing.T) {
	dir := t.TempDir()
	specPath := filepath.Join(dir, "genv.json")
	outDir := filepath.Join(dir, "out")
	writeTestFile(t, specPath, `{
	  "schemaVersion": "7",
	  "packages": [
	    {"id": "git", "prefer": "pacman", "host": "arch"}
	  ],
	  "env": {"EDITOR": {"value": "nvim"}}
	}`)

	var code int
	errOut := captureStderr(t, func() {
		code = run([]string{"export", "--file", specPath, "--target", "arch", "--out", outDir, "--from-v7"})
	})
	if code != exitOK {
		t.Fatalf("export --from-v7 = %d, want %d; stderr=%s", code, exitOK, errOut)
	}
	if strings.Contains(errOut, "schemaVersion") {
		t.Fatalf("unexpected schema error on stderr: %s", errOut)
	}
	f, err := genvfile.Read(filepath.Join(outDir, "genv.json"))
	if err != nil {
		t.Fatal(err)
	}
	if f.SchemaVersion != schema.Version8 || f.Targets["arch"] == nil || len(f.Targets["arch"].Packages) != 1 {
		t.Fatalf("unexpected exported migrated spec: %+v", f)
	}

	code = run([]string{"export", "--file", specPath, "--target", "arch", "--out", filepath.Join(dir, "out-no-migrate")})
	if code != exitUsage {
		t.Fatalf("export legacy without --from-v7 = %d, want %d", code, exitUsage)
	}
}

func TestExportCmdUsageFailures(t *testing.T) {
	dir := t.TempDir()
	specPath := filepath.Join(dir, "genv.json")
	writeTestFile(t, specPath, `{"schemaVersion":"8","targets":{"ubuntu":{}}}`)

	if code := run([]string{"export", "--file", specPath, "--out", filepath.Join(dir, "out")}); code != exitUsage {
		t.Fatalf("export without target = %d, want %d", code, exitUsage)
	}
	if code := run([]string{"export", "--file", specPath, "--target", "ubuntu"}); code != exitUsage {
		t.Fatalf("export without out = %d, want %d", code, exitUsage)
	}
	if code := run([]string{"export", "--file", filepath.Join(dir, "missing.json"), "--target", "ubuntu", "--out", filepath.Join(dir, "out")}); code != exitIO {
		t.Fatalf("export missing file = %d, want %d", code, exitIO)
	}
}
