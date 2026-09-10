package external

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestDownloadStagesPrivateArtifactAndHashesIt(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "payload")
	}))
	t.Cleanup(server.Close)
	dir := t.TempDir()

	artifact, err := (Client{HTTPClient: server.Client()}).Download(context.Background(), server.URL+"/tool", true, dir, 1024)
	if err != nil {
		t.Fatalf("Download() error: %v", err)
	}
	if artifact.SHA256 != "239f59ed55e737c77147cf55ad0c1b030b6d7ee748a7426952f9b852d5a935e5" || artifact.Size != 7 {
		t.Fatalf("artifact = %+v", artifact)
	}
	info, err := os.Stat(artifact.Path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 || filepath.Dir(artifact.Path) != dir {
		t.Fatalf("mode=%o path=%q", info.Mode().Perm(), artifact.Path)
	}
}

func TestDownloadRejectsOversizedArtifact(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "too large")
	}))
	t.Cleanup(server.Close)
	_, err := (Client{HTTPClient: server.Client()}).Download(context.Background(), server.URL, true, t.TempDir(), 3)
	if err == nil {
		t.Fatal("Download() error = nil")
	}
}
