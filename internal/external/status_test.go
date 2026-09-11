package external

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/ks1686/genv/internal/genvfile"
	"github.com/ks1686/genv/internal/schema"
)

func TestInspectLocalReportsPresentAndRecipeDrift(t *testing.T) {
	tool := filepath.Join(t.TempDir(), "tool")
	if err := os.WriteFile(tool, []byte("#!/bin/sh\necho tool-1.2.3\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	pkg := schema.Package{ID: "tool", External: &schema.ExternalRecipe{Detect: schema.ExternalDetect{Command: []string{tool}, VersionRegex: `tool-([0-9.]+)`}}}
	if got := InspectLocal(context.Background(), pkg, nil); !got.Present || got.Version != "1.2.3" || got.Drift {
		t.Fatalf("unlocked = %+v", got)
	}
	locked := &genvfile.LockedPackage{InstalledVersion: "1.0.0", External: &genvfile.ExternalReceipt{RecipeSHA256: "stale"}}
	if got := InspectLocal(context.Background(), pkg, locked); !got.Present || !got.Drift {
		t.Fatalf("drift = %+v", got)
	}
	digest, err := RecipeSHA256(pkg.External)
	if err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(t.TempDir(), "missing")
	owned := &genvfile.LockedPackage{InstalledVersion: "1.2.3", External: &genvfile.ExternalReceipt{RecipeSHA256: digest, Paths: []genvfile.ExternalPathReceipt{{Path: missing, SHA256: "abc"}}}}
	if got := InspectLocal(context.Background(), pkg, owned); !got.Drift {
		t.Fatalf("missing path should drift: %+v", got)
	}
}

func TestCurrentHostHasOSAndArch(t *testing.T) {
	host := CurrentHost()
	if host.OS == "" || host.Arch == "" {
		t.Fatalf("CurrentHost() = %+v", host)
	}
}

func TestRecipeSHA256IsStable(t *testing.T) {
	recipe := &schema.ExternalRecipe{Source: schema.ExternalSource{Type: "httpRelease"}}
	first, err := RecipeSHA256(recipe)
	if err != nil {
		t.Fatal(err)
	}
	second, err := RecipeSHA256(recipe)
	if err != nil || first == "" || first != second {
		t.Fatalf("RecipeSHA256() = %q %q %v", first, second, err)
	}
}

func TestExpandDestinationHomeRelative(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	got, err := expandDestination("~/bin/tool")
	if err != nil {
		t.Fatal(err)
	}
	if got != filepath.Join(home, "bin", "tool") {
		t.Fatalf("expandDestination() = %q", got)
	}
	if _, err := expandDestination("relative/tool"); err == nil {
		t.Fatal("relative destination accepted")
	}
}

func TestLocalKeyReadsInlineAndFile(t *testing.T) {
	if got, err := localKey("inline", ""); err != nil || string(got) != "inline" {
		t.Fatalf("inline key = %q %v", got, err)
	}
	path := filepath.Join(t.TempDir(), "key")
	if err := os.WriteFile(path, []byte("file-key"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := localKey("", path)
	if err != nil || string(got) != "file-key" {
		t.Fatalf("file key = %q %v", got, err)
	}
}
