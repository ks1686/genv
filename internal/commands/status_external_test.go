package commands

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ks1686/genv/internal/genvfile"
	"github.com/ks1686/genv/internal/schema"
)

func TestStatusDetectsUnlockedExternalRecipeAsPresent(t *testing.T) {
	tool := filepath.Join(t.TempDir(), "tool")
	if err := os.WriteFile(tool, []byte("#!/bin/sh\necho tool-1.2.3\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	f := &schema.GenvFile{Packages: []schema.Package{{ID: "tool", External: &schema.ExternalRecipe{Detect: schema.ExternalDetect{Command: []string{tool}, VersionRegex: `tool-([0-9.]+)`}}}}}
	entries := Status(f, &genvfile.LockFile{})
	if len(entries) != 1 || entries[0].Kind != StatusPresent || entries[0].InstalledVersion != "1.2.3" {
		t.Fatalf("Status() = %+v", entries)
	}
}
