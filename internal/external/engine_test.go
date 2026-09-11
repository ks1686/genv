package external

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/ks1686/genv/internal/schema"
	"github.com/ks1686/genv/internal/testutil"
)

func TestEngineInstallsVerifiedDirectArtifactAndReturnsReceipt(t *testing.T) {
	payload, suffix := testutil.DirectToolArtifact("tool 1.2.3")
	digestBytes := sha256.Sum256(payload)
	digest := hex.EncodeToString(digestBytes[:])
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/latest.json":
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"version":"1.2.3"}`)
		case "/tool-1.2.3":
			_, _ = w.Write(payload)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	destination := filepath.Join(t.TempDir(), "bin", "tool"+suffix)
	pkg := schema.Package{ID: "tool", Prefer: "external", External: &schema.ExternalRecipe{
		Detect: schema.ExternalDetect{Command: []string{destination, "--version"}, VersionRegex: `tool ([0-9.]+)`},
		Source: schema.ExternalSource{Type: "httpRelease", VersionURL: server.URL + "/latest.json", Format: "json", VersionPointer: "/version"},
		Platforms: []schema.ExternalPlatform{{
			OS: []string{"linux"}, Arch: []string{"amd64"}, ArtifactURL: server.URL + "/tool-{version}",
			Install: schema.ExternalInstall{Type: "direct", Scope: "system", Destination: destination},
		}},
		Verify:            []schema.ExternalVerification{{Type: "sha256", Value: digest}},
		AllowInsecureHTTP: true,
	}}
	engine := Engine{Client: Client{HTTPClient: server.Client()}, Host: Host{OS: "linux", Arch: "amd64"}}

	installed, err := engine.Install(context.Background(), pkg)
	if err != nil {
		t.Fatalf("Install() error: %v", err)
	}
	if installed.Version != "1.2.3" || installed.Receipt == nil || !installed.Receipt.Owned {
		t.Fatalf("installed = %+v", installed)
	}
	if installed.Receipt.ArtifactSHA256 != digest || len(installed.Receipt.Paths) != 1 || installed.Receipt.Paths[0].Path != destination {
		t.Fatalf("receipt = %+v", installed.Receipt)
	}
	info, err := os.Stat(destination)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o755 {
		t.Fatalf("mode = %o", info.Mode().Perm())
	}
}

func TestEngineLeavesExistingDestinationAfterFailedDetection(t *testing.T) {
	dir := t.TempDir()
	destination := filepath.Join(dir, "tool")
	if err := os.WriteFile(destination, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/latest" {
			w.Header().Set("Content-Type", "text/plain")
			fmt.Fprint(w, "1.0.0")
			return
		}
		fmt.Fprint(w, "not executable content")
	}))
	t.Cleanup(server.Close)
	digestBytes := sha256.Sum256([]byte("not executable content"))
	pkg := schema.Package{ID: "tool", Prefer: "external", External: &schema.ExternalRecipe{
		Detect:    schema.ExternalDetect{Command: []string{destination}, VersionRegex: `([0-9.]+)`},
		Source:    schema.ExternalSource{Type: "httpRelease", VersionURL: server.URL + "/latest", Format: "text", VersionRegex: `([0-9.]+)`},
		Platforms: []schema.ExternalPlatform{{OS: []string{"linux"}, Arch: []string{"amd64"}, ArtifactURL: server.URL + "/asset", Install: schema.ExternalInstall{Type: "direct", Scope: "system", Destination: destination}}},
		Verify:    []schema.ExternalVerification{{Type: "sha256", Value: hex.EncodeToString(digestBytes[:])}}, AllowInsecureHTTP: true,
	}}
	_, err := (Engine{Client: Client{HTTPClient: server.Client()}, Host: Host{OS: "linux", Arch: "amd64"}}).Install(context.Background(), pkg)
	if err == nil {
		t.Fatal("Install() error = nil")
	}
	got, readErr := os.ReadFile(destination)
	if readErr != nil || string(got) != "old" {
		t.Fatalf("destination = %q, read error = %v", got, readErr)
	}
}

func TestEngineRunsVerifiedScriptAndStoresUninstallReceipt(t *testing.T) {
	dir := t.TempDir()
	var (
		destination string
		payload     []byte
		interpreter string
		uninstall   []string
	)
	if runtime.GOOS == "windows" {
		// Script installs on Windows use pwsh; bash/sh are not guaranteed on CI.
		if _, err := exec.LookPath("pwsh"); err != nil {
			t.Skip("pwsh unavailable; script installs use pwsh on Windows")
		}
		destination = filepath.Join(dir, "tool.cmd")
		interpreter = "pwsh"
		payload = []byte("$dest = $args[0]\nSet-Content -Path $dest -Value \"@echo off`r`necho tool-1.2.3`r`n\"\n")
		uninstall = []string{"cmd", "/C", "del", "{destination}"}
	} else {
		destination = filepath.Join(dir, "tool")
		interpreter = "sh"
		payload = []byte("#!/bin/sh\nprintf '#!/bin/sh\\necho tool-1.2.3\\n' > \"$1\"\nchmod 755 \"$1\"\n")
		uninstall = []string{"rm", "{destination}"}
	}
	digestBytes := sha256.Sum256(payload)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/latest" {
			fmt.Fprint(w, "1.2.3")
			return
		}
		_, _ = w.Write(payload)
	}))
	t.Cleanup(server.Close)
	pkg := schema.Package{ID: "tool", External: &schema.ExternalRecipe{
		Detect: schema.ExternalDetect{Command: []string{destination}, VersionRegex: `tool-([0-9.]+)`},
		Source: schema.ExternalSource{Type: "httpRelease", VersionURL: server.URL + "/latest", Format: "text", VersionRegex: `([0-9.]+)`},
		Platforms: []schema.ExternalPlatform{{OS: []string{"linux"}, Arch: []string{"amd64"}, ArtifactURL: server.URL + "/install.sh", Install: schema.ExternalInstall{
			Type: "script", Interpreter: interpreter, Destination: destination, Args: []string{"{destination}"}, Uninstall: uninstall,
		}}},
		Verify: []schema.ExternalVerification{{Type: "sha256", Value: hex.EncodeToString(digestBytes[:])}}, AllowInsecureHTTP: true,
	}}
	installed, err := (Engine{Client: Client{HTTPClient: server.Client()}, Host: Host{OS: "linux", Arch: "amd64"}}).Install(context.Background(), pkg)
	if err != nil {
		t.Fatalf("Install() error: %v", err)
	}
	if installed.Receipt.Owned || len(installed.Receipt.Paths) != 0 || len(installed.Receipt.Uninstall) != len(uninstall) || installed.Receipt.Uninstall[len(installed.Receipt.Uninstall)-1] != destination {
		t.Fatalf("receipt = %+v", installed.Receipt)
	}
}

func TestEngineVerifiesChecksumFileMaterial(t *testing.T) {
	payload, suffix := testutil.DirectToolArtifact("tool 1.2.3")
	digestBytes := sha256.Sum256(payload)
	checksums := hex.EncodeToString(digestBytes[:]) + "  tool\n"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/latest":
			fmt.Fprint(w, "1.2.3")
		case "/checksums.txt":
			fmt.Fprint(w, checksums)
		case "/tool":
			_, _ = w.Write(payload)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	destination := filepath.Join(t.TempDir(), "tool"+suffix)
	pkg := schema.Package{ID: "tool", External: &schema.ExternalRecipe{
		Detect:    schema.ExternalDetect{Command: []string{destination}, VersionRegex: `tool ([0-9.]+)`},
		Source:    schema.ExternalSource{Type: "httpRelease", VersionURL: server.URL + "/latest", Format: "text", VersionRegex: `([0-9.]+)`},
		Platforms: []schema.ExternalPlatform{{OS: []string{"linux"}, Arch: []string{"amd64"}, ArtifactURL: server.URL + "/tool", Install: schema.ExternalInstall{Type: "direct", Scope: "system", Destination: destination}}},
		Verify:    []schema.ExternalVerification{{Type: "sha256File", URL: server.URL + "/checksums.txt"}}, AllowInsecureHTTP: true,
	}}
	installed, err := (Engine{Client: Client{HTTPClient: server.Client()}, Host: Host{OS: "linux", Arch: "amd64"}}).Install(context.Background(), pkg)
	if err != nil {
		t.Fatalf("Install() error: %v", err)
	}
	if installed.Receipt.Verification != "sha256File" {
		t.Fatalf("verification = %q", installed.Receipt.Verification)
	}
}

func TestEngineLatestVersionUsesMetadataOnly(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/latest" {
			t.Errorf("unexpected path %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		fmt.Fprint(w, "9.9.9")
	}))
	t.Cleanup(server.Close)
	pkg := schema.Package{ID: "tool", External: &schema.ExternalRecipe{
		Source:            schema.ExternalSource{Type: "httpRelease", VersionURL: server.URL + "/latest", Format: "text", VersionRegex: `([0-9.]+)`},
		Platforms:         []schema.ExternalPlatform{{OS: []string{"linux"}, Arch: []string{"amd64"}, ArtifactURL: server.URL + "/tool", Install: schema.ExternalInstall{Type: "direct", Scope: "system", Destination: filepath.Join(t.TempDir(), "tool")}}},
		AllowInsecureHTTP: true,
	}}
	got, err := (Engine{Client: Client{HTTPClient: server.Client()}, Host: Host{OS: "linux", Arch: "amd64"}}).LatestVersion(context.Background(), pkg)
	if err != nil || got != "9.9.9" {
		t.Fatalf("LatestVersion() = %q, %v", got, err)
	}
}

func TestEnginePlanResolvesMetadataWithoutDownloadingArtifact(t *testing.T) {
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		if r.URL.Path == "/latest" {
			fmt.Fprint(w, "1.2.3")
			return
		}
		t.Errorf("unexpected download of %s", r.URL.Path)
	}))
	t.Cleanup(server.Close)
	pkg := schema.Package{ID: "tool", External: &schema.ExternalRecipe{
		Detect:    schema.ExternalDetect{Command: []string{"tool"}, VersionRegex: `([0-9.]+)`},
		Source:    schema.ExternalSource{Type: "httpRelease", VersionURL: server.URL + "/latest", Format: "text", VersionRegex: `([0-9.]+)`},
		Platforms: []schema.ExternalPlatform{{OS: []string{"linux"}, Arch: []string{"amd64"}, ArtifactURL: server.URL + "/tool-{version}", Install: schema.ExternalInstall{Type: "direct", Scope: "system", Destination: filepath.Join(t.TempDir(), "tool")}}},
		Verify:    []schema.ExternalVerification{{Type: "sha256", Value: "abc"}}, AllowInsecureHTTP: true,
	}}
	planned, err := (Engine{Client: Client{HTTPClient: server.Client()}, Host: Host{OS: "linux", Arch: "amd64"}}).Plan(context.Background(), pkg)
	if err != nil {
		t.Fatalf("Plan() error: %v", err)
	}
	if planned.Version != "1.2.3" || planned.InstallType != "direct" || planned.Scope != "system" || !strings.Contains(planned.Detail, "requires sudo") && !strings.Contains(planned.Detail, "elevated") {
		t.Fatalf("planned = %+v", planned)
	}
	if len(paths) != 1 || paths[0] != "/latest" {
		t.Fatalf("paths = %v, want only metadata", paths)
	}
}

func TestEngineRejectsUnverifiedInstallWithAssumeYes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/latest" {
			fmt.Fprint(w, "1.0.0")
			return
		}
		fmt.Fprint(w, "payload")
	}))
	t.Cleanup(server.Close)
	pkg := schema.Package{ID: "tool", External: &schema.ExternalRecipe{
		Detect:          schema.ExternalDetect{Command: []string{"tool"}, VersionRegex: `([0-9.]+)`},
		Source:          schema.ExternalSource{Type: "httpRelease", VersionURL: server.URL + "/latest", Format: "text", VersionRegex: `([0-9.]+)`},
		Platforms:       []schema.ExternalPlatform{{OS: []string{"linux"}, Arch: []string{"amd64"}, ArtifactURL: server.URL + "/tool", Install: schema.ExternalInstall{Type: "direct", Scope: "system", Destination: filepath.Join(t.TempDir(), "tool")}}},
		AllowUnverified: true, AllowInsecureHTTP: true,
	}}
	_, err := (Engine{Client: Client{HTTPClient: server.Client()}, Host: Host{OS: "linux", Arch: "amd64"}, Mode: ExecutionAssumeYes}).Install(context.Background(), pkg)
	if err == nil || !strings.Contains(err.Error(), "interactive acknowledgement") {
		t.Fatalf("Install() error = %v", err)
	}
}
