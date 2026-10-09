package external

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ks1686/genv/internal/genvfile"
	"github.com/ks1686/genv/internal/schema"
)

func TestResolveArtifactAndTemplateFailures(t *testing.T) {
	release := Release{Version: "1.2.3", Tag: "v1.2.3", Assets: []Asset{{Name: "tool-darwin-arm64", URL: "https://example.test/tool"}}}
	host := Host{OS: "darwin", Arch: "arm64"}

	asset, err := resolveArtifact(release, schema.ExternalPlatform{AssetRegex: `^tool-`}, host)
	if err != nil || asset.Name != "tool-darwin-arm64" {
		t.Fatalf("regex asset = %+v, %v", asset, err)
	}
	asset, err = resolveArtifact(release, schema.ExternalPlatform{ArtifactURL: "https://example.test/{os}/{arch}/{version}"}, host)
	if err != nil || asset.Name != "1.2.3" || !strings.Contains(asset.URL, "/darwin/arm64/") {
		t.Fatalf("URL asset = %+v, %v", asset, err)
	}
	if _, err := resolveArtifact(release, schema.ExternalPlatform{ArtifactURL: "{unknown}"}, host); err == nil {
		t.Fatal("unknown URL placeholder accepted")
	}
	if _, err := ExpandArtifactURL("https://x/{version", release, host); err == nil {
		t.Fatal("malformed URL template accepted")
	}
}

func TestVerifyArtifactSHAAndPolicyFailures(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tool")
	payload := []byte("tool payload")
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(payload)
	digest := hex.EncodeToString(sum[:])
	engine := Engine{}
	recipe := &schema.ExternalRecipe{Verify: []schema.ExternalVerification{{Type: "githubDigest"}}}
	method, err := engine.verifyArtifact(context.Background(), recipe, Release{}, Asset{Digest: "sha256:" + digest}, DownloadedArtifact{Path: path, SHA256: digest}, t.TempDir())
	if err != nil || method != "githubDigest" {
		t.Fatalf("githubDigest = %q, %v", method, err)
	}
	if _, err := engine.verifyArtifact(context.Background(), &schema.ExternalRecipe{}, Release{}, Asset{}, DownloadedArtifact{}, t.TempDir()); err == nil {
		t.Fatal("missing verification policy accepted")
	}
	method, err = engine.verifyArtifact(context.Background(), &schema.ExternalRecipe{AllowUnverified: true}, Release{}, Asset{}, DownloadedArtifact{}, t.TempDir())
	if err != nil || method != "unverified" {
		t.Fatalf("allowUnverified = %q, %v", method, err)
	}
	if _, err := engine.verifyArtifact(context.Background(), &schema.ExternalRecipe{Verify: []schema.ExternalVerification{{Type: "unknown"}}}, Release{}, Asset{}, DownloadedArtifact{}, t.TempDir()); err == nil {
		t.Fatal("unknown verification type accepted")
	}
}

func TestExternalMetadataHelpers(t *testing.T) {
	if got, err := captureVersion("release-v1.2.3", `v([0-9.]+)`); err != nil || got != "1.2.3" {
		t.Fatalf("captureVersion = %q, %v", got, err)
	}
	for _, expression := range []string{"[", `v([0-9]+)-([a-z]+)`, `x([0-9]+)`} {
		if _, err := captureVersion("release-v1.2.3", expression); err == nil {
			t.Errorf("captureVersion accepted %q", expression)
		}
	}
	value, err := jsonPointer([]byte(`{"release":{"name":"a/b"},"list":["zero"]}`), "/release/name")
	if err != nil || value != "a/b" {
		t.Fatalf("jsonPointer = %q, %v", value, err)
	}
	if _, err := jsonPointer([]byte(`{"x":1}`), "/x"); err == nil {
		t.Error("non-string JSON pointer result accepted")
	}
	for _, pointer := range []string{"", "x", "/missing"} {
		if _, err := jsonPointer([]byte(`{"x":"y"}`), pointer); err == nil {
			t.Errorf("jsonPointer accepted %q", pointer)
		}
	}
}

func TestExternalRemoveGuardsModifiedAndMissingPaths(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tool")
	if err := os.WriteFile(path, []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}
	digest, err := fileSHA256(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("modified"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Remove(&genvfile.ExternalReceipt{Owned: true, Paths: []genvfile.ExternalPathReceipt{{Path: path, SHA256: digest}}}); err == nil {
		t.Fatal("Remove deleted a modified path")
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("modified path disappeared: %v", err)
	}
	if err := Remove(&genvfile.ExternalReceipt{Owned: true, Paths: []genvfile.ExternalPathReceipt{{Path: filepath.Join(t.TempDir(), "gone"), SHA256: digest}}}); err != nil {
		t.Fatalf("missing owned path should be idempotent: %v", err)
	}
}
