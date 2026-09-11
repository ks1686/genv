package external

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ks1686/genv/internal/schema"
)

func TestResolveGitHubReleaseSelectsStableAndNormalizesTag(t *testing.T) {
	var gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `[
          {"id":43,"tag_name":"v2.0.0-beta.1","prerelease":true,"draft":false,"assets":[]},
          {"id":42,"tag_name":"v1.2.3","prerelease":false,"draft":false,"assets":[{"name":"tool.tar.gz","browser_download_url":"https://example.test/tool.tar.gz","digest":"sha256:abc"}]}
        ]`)
	}))
	t.Cleanup(server.Close)

	client := Client{HTTPClient: server.Client(), GitHubToken: "secret"}
	release, err := client.Resolve(context.Background(), schema.ExternalSource{
		Type: "githubRelease", Repository: "owner/tool", Release: "stable", APIBase: server.URL, TagRegex: `^v(.+)$`,
	}, true)
	if err != nil {
		t.Fatalf("Resolve() error: %v", err)
	}
	if release.ID != "42" || release.Tag != "v1.2.3" || release.Version != "1.2.3" || len(release.Assets) != 1 {
		t.Fatalf("release = %+v", release)
	}
	if gotAuth != "Bearer secret" {
		t.Fatalf("Authorization = %q", gotAuth)
	}
}

func TestResolveHTTPReleaseFromJSONPointer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"release":{"version":"3.4.5"},"sha256":"abc"}`)
	}))
	t.Cleanup(server.Close)

	release, err := (Client{HTTPClient: server.Client()}).Resolve(context.Background(), schema.ExternalSource{
		Type: "httpRelease", VersionURL: server.URL, Format: "json", VersionPointer: "/release/version",
	}, true)
	if err != nil {
		t.Fatalf("Resolve() error: %v", err)
	}
	if release.Version != "3.4.5" || string(release.Metadata) != `{"release":{"version":"3.4.5"},"sha256":"abc"}` {
		t.Fatalf("release = %+v", release)
	}
}

func TestResolveHTTPReleaseRejectsHTMLAndOversizedResponses(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		body        string
		max         int64
	}{
		{name: "html", contentType: "text/html", body: `<html>1.0.0</html>`, max: 1024},
		{name: "oversized", contentType: "text/plain", body: "version=123456789", max: 4},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", tc.contentType)
				fmt.Fprint(w, tc.body)
			}))
			defer server.Close()
			_, err := (Client{HTTPClient: server.Client(), MaxMetadataBytes: tc.max}).Resolve(context.Background(), schema.ExternalSource{
				Type: "httpRelease", VersionURL: server.URL, Format: "text", VersionRegex: `version=([0-9]+)`,
			}, true)
			if err == nil {
				t.Fatal("Resolve() error = nil")
			}
		})
	}
}
