package external

import (
	"strings"
	"testing"

	"github.com/ks1686/genv/internal/schema"
)

func TestSelectPlatformAndGitHubAsset(t *testing.T) {
	platforms := []schema.ExternalPlatform{
		{OS: []string{"linux"}, Arch: []string{"amd64"}, Libc: []string{"glibc"}, AssetRegex: `^tool_linux_amd64\.tar\.gz$`},
		{OS: []string{"windows"}, Arch: []string{"amd64"}, AssetRegex: `^tool_windows_amd64\.zip$`},
	}
	platform, err := SelectPlatform(platforms, Host{OS: "linux", Arch: "amd64", Libc: "glibc"})
	if err != nil {
		t.Fatalf("SelectPlatform() error: %v", err)
	}
	release := Release{Assets: []Asset{
		{Name: "checksums.txt", URL: "https://example.test/checksums.txt"},
		{Name: "tool_linux_amd64.tar.gz", URL: "https://example.test/tool.tar.gz"},
	}}
	asset, err := SelectAsset(release, platform)
	if err != nil {
		t.Fatalf("SelectAsset() error: %v", err)
	}
	if asset.Name != "tool_linux_amd64.tar.gz" {
		t.Fatalf("asset = %+v", asset)
	}
}

func TestSelectPlatformRejectsNoMatchAndAmbiguity(t *testing.T) {
	matching := schema.ExternalPlatform{OS: []string{"linux"}, Arch: []string{"amd64"}}
	tests := []struct {
		name      string
		platforms []schema.ExternalPlatform
		want      string
	}{
		{name: "none", platforms: []schema.ExternalPlatform{{OS: []string{"windows"}, Arch: []string{"amd64"}}}, want: "no external platform"},
		{name: "ambiguous", platforms: []schema.ExternalPlatform{matching, matching}, want: "multiple external platforms"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := SelectPlatform(tc.platforms, Host{OS: "linux", Arch: "amd64"})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestExpandArtifactURL(t *testing.T) {
	got, err := ExpandArtifactURL("https://example.test/{version}/tool_{os}_{arch}.zip", Release{Version: "1.2.3"}, Host{OS: "windows", Arch: "arm64"})
	if err != nil {
		t.Fatalf("ExpandArtifactURL() error: %v", err)
	}
	if got != "https://example.test/1.2.3/tool_windows_arm64.zip" {
		t.Fatalf("URL = %q", got)
	}
	if _, err := ExpandArtifactURL("https://example.test/{unknown}", Release{}, Host{}); err == nil {
		t.Fatal("unknown placeholder accepted")
	}
}
