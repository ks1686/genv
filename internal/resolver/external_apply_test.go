package resolver

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/ks1686/genv/internal/genvfile"
	"github.com/ks1686/genv/internal/schema"
)

func TestExecuteApplyInstallsManagedExternalRecipe(t *testing.T) {
	payload := []byte("#!/bin/sh\necho 'tool 1.2.3'\n")
	digestBytes := sha256.Sum256(payload)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/latest" {
			w.Header().Set("Content-Type", "text/plain")
			fmt.Fprint(w, "1.2.3")
			return
		}
		_, _ = w.Write(payload)
	}))
	t.Cleanup(server.Close)
	destination := filepath.Join(t.TempDir(), "tool")
	pkg := schema.Package{ID: "tool", Prefer: "external", External: &schema.ExternalRecipe{
		Detect:    schema.ExternalDetect{Command: []string{destination}, VersionRegex: `tool ([0-9.]+)`},
		Source:    schema.ExternalSource{Type: "httpRelease", VersionURL: server.URL + "/latest", Format: "text", VersionRegex: `([0-9.]+)`},
		Platforms: []schema.ExternalPlatform{{OS: []string{runtime.GOOS}, Arch: []string{runtime.GOARCH}, ArtifactURL: server.URL + "/tool", Install: schema.ExternalInstall{Type: "direct", Scope: "system", Destination: destination}}},
		Verify:    []schema.ExternalVerification{{Type: "sha256", Value: hex.EncodeToString(digestBytes[:])}}, AllowInsecureHTTP: true,
	}}
	result := ReconcileResult{ToInstall: []Action{{Pkg: pkg, Manager: "external", PkgName: "tool"}}}

	got := ExecuteApply(context.Background(), result, nil, io.Discard, io.Discard)
	if len(got.Errors) != 0 {
		t.Fatalf("ExecuteApply() errors: %v", got.Errors)
	}
	if len(got.Installed) != 1 || got.Installed[0].External == nil || got.Installed[0].InstalledVersion != "1.2.3" {
		t.Fatalf("installed = %+v", got.Installed)
	}
}

func TestReconcileReinstallsDriftedExternalReceipt(t *testing.T) {
	tool := filepath.Join(t.TempDir(), "tool")
	if err := os.WriteFile(tool, []byte("#!/bin/sh\necho tool-1.0.0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	pkg := schema.Package{ID: "tool", Prefer: "external", External: &schema.ExternalRecipe{
		Detect: schema.ExternalDetect{Command: []string{tool}, VersionRegex: `tool-([0-9.]+)`},
	}}
	lock := genvfile.LockedPackage{ID: "tool", Manager: "external", PkgName: "tool", InstalledVersion: "0.9.0", External: &genvfile.ExternalReceipt{RecipeSHA256: "stale"}}
	got := Reconcile([]schema.Package{pkg}, []genvfile.LockedPackage{lock}, map[string]bool{"external": true})
	if len(got.ToInstall) != 1 || got.ToInstall[0].Pkg.ID != "tool" || len(got.Unchanged) != 0 {
		t.Fatalf("Reconcile() = %+v", got)
	}
}

func TestEnrichExternalPlanUsesMetadataDetail(t *testing.T) {
	original := planExternal
	planExternal = func(context.Context, schema.Package) (string, error) {
		return "direct 1.2.3; verify=sha256; scope=user", nil
	}
	t.Cleanup(func() { planExternal = original })
	result := ReconcileResult{ToInstall: []Action{{Pkg: schema.Package{ID: "tool", External: &schema.ExternalRecipe{}}, Manager: "external"}}}
	EnrichExternalPlan(context.Background(), &result)
	if result.ToInstall[0].Detail != "direct 1.2.3; verify=sha256; scope=user" {
		t.Fatalf("Detail = %q", result.ToInstall[0].Detail)
	}
}

func TestExecuteApplyRemovesOwnedExternalPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tool")
	if err := os.WriteFile(path, []byte("payload"), 0o755); err != nil {
		t.Fatal(err)
	}
	digestBytes := sha256.Sum256([]byte("payload"))
	locked := genvfile.LockedPackage{ID: "tool", Manager: "external", PkgName: "tool", External: &genvfile.ExternalReceipt{
		Owned: true, InstallType: "direct", Paths: []genvfile.ExternalPathReceipt{{Path: path, SHA256: hex.EncodeToString(digestBytes[:])}},
	}}
	result := ReconcileResult{ToRemove: []Action{{Pkg: schema.Package{ID: "tool"}, Manager: "external", PkgName: "tool", Locked: &locked}}}
	got := ExecuteApply(context.Background(), result, nil, io.Discard, io.Discard)
	if len(got.Errors) != 0 || len(got.Uninstalled) != 1 {
		t.Fatalf("ExecuteApply() = %+v", got)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("owned path still exists: %v", err)
	}
}
