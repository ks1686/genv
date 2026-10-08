package external

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"github.com/ks1686/genv/internal/schema"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestVerifyArtifactSHA256MetadataAndChecksumFile(t *testing.T) {
	payload := []byte("payload")
	sum := sha256.Sum256(payload)
	digest := hex.EncodeToString(sum[:])
	path := filepath.Join(t.TempDir(), "tool")
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(digest + "  tool\n")) }))
	t.Cleanup(srv.Close)
	e := Engine{Client: Client{HTTPClient: srv.Client()}}
	artifact := DownloadedArtifact{Path: path, SHA256: digest}
	release := Release{Metadata: []byte(`{"sha":"` + digest + `"}`), Assets: []Asset{{Name: "checksums.txt", URL: srv.URL}}}
	for _, v := range []schema.ExternalVerification{{Type: "sha256", ValuePointer: "/sha"}, {Type: "sha256File", AssetRegex: `checksums\.txt`}} {
		got, err := e.verifyArtifact(context.Background(), &schema.ExternalRecipe{AllowInsecureHTTP: true, Verify: []schema.ExternalVerification{v}}, release, Asset{Name: "tool"}, artifact, t.TempDir())
		if err != nil || got == "" {
			t.Fatalf("%s = %q,%v", v.Type, got, err)
		}
	}
}
