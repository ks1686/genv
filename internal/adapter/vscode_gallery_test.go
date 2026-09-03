package adapter

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestVscodeLatestStableFromVersions_skipsNewerPreRelease(t *testing.T) {
	// Gallery lists newest-first: a pre-release newer than the installed
	// stable must not be treated as the available update.
	versions := []vscodeGalleryVersion{
		{
			Version: "1.0.40",
			Properties: []vscodeGalleryProperty{
				{Key: "Microsoft.VisualStudio.Code.PreRelease", Value: "true"},
			},
		},
		{Version: "1.0.39"},
	}
	if got := vscodeLatestStableFromVersions(versions); got != "1.0.39" {
		t.Fatalf("vscodeLatestStableFromVersions = %q, want 1.0.39 (newest stable)", got)
	}
}

func TestVscodeLatestStableFromVersions_picksNewerStable(t *testing.T) {
	versions := []vscodeGalleryVersion{
		{Version: "1.0.41"},
		{
			Version: "1.0.40",
			Properties: []vscodeGalleryProperty{
				{Key: "Microsoft.VisualStudio.Code.PreRelease", Value: "true"},
			},
		},
		{Version: "1.0.39"},
	}
	if got := vscodeLatestStableFromVersions(versions); got != "1.0.41" {
		t.Fatalf("vscodeLatestStableFromVersions = %q, want 1.0.41", got)
	}
}

func TestVscodeLatestStableFromVersions_allPreReleaseIsUnknown(t *testing.T) {
	versions := []vscodeGalleryVersion{
		{
			Version: "1.0.40",
			Properties: []vscodeGalleryProperty{
				{Key: "Microsoft.VisualStudio.Code.PreRelease", Value: "true"},
			},
		},
	}
	if got := vscodeLatestStableFromVersions(versions); got != "" {
		t.Fatalf("vscodeLatestStableFromVersions = %q, want empty when no stable exists", got)
	}
}

func TestVscode_ListOutdated_skipsNewerPreRelease(t *testing.T) {
	installFakeBinary(t, "code", `if [ "$1" = "--list-extensions" ] && [ "$2" = "--show-versions" ]; then
  echo 'anysphere.remote-containers@1.0.39'
  echo 'anysphere.remote-ssh@1.1.14'
fi`)
	srv := newVscodeGalleryServer(t, map[string][]vscodeGalleryVersion{
		"anysphere.remote-containers": {
			preReleaseGalleryVersion("1.0.40"),
			{Version: "1.0.39"},
		},
		"anysphere.remote-ssh": {
			preReleaseGalleryVersion("1.1.15"),
			{Version: "1.1.14"},
		},
	})
	defer srv.Close()
	setVscodeGalleryBase(t, srv.URL)

	got, err := Vscode{}.ListOutdated([]string{"anysphere.remote-containers", "anysphere.remote-ssh"})
	if err != nil {
		t.Fatalf("ListOutdated: %v", err)
	}
	if got != nil {
		t.Fatalf("ListOutdated = %v, want nil (installed is already newest stable)", got)
	}
}

func TestVscode_ListOutdated_reportsNewerStable(t *testing.T) {
	installFakeBinary(t, "code", `if [ "$1" = "--list-extensions" ] && [ "$2" = "--show-versions" ]; then
  echo 'golang.go@0.42.0'
  echo 'anysphere.remote-containers@1.0.39'
fi`)
	srv := newVscodeGalleryServer(t, map[string][]vscodeGalleryVersion{
		"golang.go": {
			{Version: "0.43.0"},
			{Version: "0.42.0"},
		},
		"anysphere.remote-containers": {
			preReleaseGalleryVersion("1.0.40"),
			{Version: "1.0.39"},
		},
	})
	defer srv.Close()
	setVscodeGalleryBase(t, srv.URL)

	got, err := Vscode{}.ListOutdated([]string{"golang.go", "anysphere.remote-containers"})
	if err != nil {
		t.Fatalf("ListOutdated: %v", err)
	}
	want := map[string]string{"golang.go": "0.43.0"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ListOutdated = %v, want %v", got, want)
	}
}

func TestFetchVscodeLatestStableVersions_omitsIncludeLatestVersionOnly(t *testing.T) {
	var gotFlags int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if !strings.HasSuffix(r.URL.Path, "/extensionquery") {
			t.Errorf("path = %q, want .../extensionquery", r.URL.Path)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
			return
		}
		var q vscodeGalleryQuery
		if err := json.Unmarshal(body, &q); err != nil {
			t.Errorf("decode query: %v", err)
			return
		}
		gotFlags = q.Flags
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[{"extensions":[]}]}`))
	}))
	defer srv.Close()
	setVscodeGalleryBase(t, srv.URL)

	if _, err := fetchVscodeLatestStableVersions([]string{"golang.go"}); err != nil {
		t.Fatalf("fetchVscodeLatestStableVersions: %v", err)
	}
	if gotFlags&vscodeGalleryIncludeLatestVersionOnly != 0 {
		t.Fatalf("flags = %d includes IncludeLatestVersionOnly (%d); that returns a pre-release as latest", gotFlags, vscodeGalleryIncludeLatestVersionOnly)
	}
	if gotFlags&vscodeGalleryIncludeVersions == 0 {
		t.Fatalf("flags = %d missing IncludeVersions (%d)", gotFlags, vscodeGalleryIncludeVersions)
	}
	if gotFlags&vscodeGalleryIncludeVersionProperties == 0 {
		t.Fatalf("flags = %d missing IncludeVersionProperties (%d); PreRelease lives on version properties", gotFlags, vscodeGalleryIncludeVersionProperties)
	}
}

func TestVscodeGalleryURLFromProductJSON_readsCursorGallery(t *testing.T) {
	raw := []byte(`{
	  "nameShort": "Cursor",
	  "extensionsGallery": {
	    "serviceUrl": "https://marketplace.cursorapi.com/_apis/public/gallery"
	  }
	}`)
	got := vscodeGalleryURLFromProductJSON(raw)
	want := "https://marketplace.cursorapi.com/_apis/public/gallery"
	if got != want {
		t.Fatalf("vscodeGalleryURLFromProductJSON = %q, want %q", got, want)
	}
}

func TestVscodeGalleryURLFromProductJSON_invalidJSON(t *testing.T) {
	if got := vscodeGalleryURLFromProductJSON([]byte(`{`)); got != "" {
		t.Fatalf("vscodeGalleryURLFromProductJSON(invalid) = %q, want empty", got)
	}
}

func TestVscodeGalleryServiceURL_readsProductJSONFromCodeBinary(t *testing.T) {
	orig := vscodeGalleryBase
	vscodeGalleryBase = ""
	t.Cleanup(func() { vscodeGalleryBase = orig })

	root := t.TempDir()
	binDir := filepath.Join(root, "app", "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	want := "https://marketplace.cursorapi.com/_apis/public/gallery"
	if err := os.WriteFile(filepath.Join(root, "app", "product.json"), []byte(`{"extensionsGallery":{"serviceUrl":"`+want+`"}}`), 0o644); err != nil {
		t.Fatalf("WriteFile product.json: %v", err)
	}
	if err := os.WriteFile(filepath.Join(binDir, "code"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("WriteFile code: %v", err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	if got := vscodeGalleryServiceURL(); got != want {
		t.Fatalf("vscodeGalleryServiceURL = %q, want Cursor gallery %q", got, want)
	}
}

func TestFetchVscodeLatestStableVersions_serverError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()
	setVscodeGalleryBase(t, srv.URL)

	_, err := fetchVscodeLatestStableVersions([]string{"golang.go"})
	if err == nil {
		t.Fatal("fetchVscodeLatestStableVersions: want error on gallery 500")
	}
}

func TestVscodeFindProductJSON_nextToCodeBinary(t *testing.T) {
	root := t.TempDir()
	binDir := filepath.Join(root, "app", "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	product := filepath.Join(root, "app", "product.json")
	if err := os.WriteFile(product, []byte(`{"extensionsGallery":{"serviceUrl":"https://example.test/gallery"}}`), 0o644); err != nil {
		t.Fatalf("WriteFile product.json: %v", err)
	}
	codePath := filepath.Join(binDir, "code")
	if err := os.WriteFile(codePath, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("WriteFile code: %v", err)
	}

	got := vscodeFindProductJSON(codePath)
	if got != product {
		t.Fatalf("vscodeFindProductJSON = %q, want %q", got, product)
	}
}

func preReleaseGalleryVersion(version string) vscodeGalleryVersion {
	return vscodeGalleryVersion{
		Version: version,
		Properties: []vscodeGalleryProperty{
			{Key: "Microsoft.VisualStudio.Code.PreRelease", Value: "true"},
		},
	}
}

func setVscodeGalleryBase(t *testing.T, base string) {
	t.Helper()
	orig := vscodeGalleryBase
	vscodeGalleryBase = base
	t.Cleanup(func() { vscodeGalleryBase = orig })
}

func newVscodeGalleryServer(t *testing.T, byID map[string][]vscodeGalleryVersion) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method", http.StatusMethodNotAllowed)
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		var q vscodeGalleryQuery
		if err := json.Unmarshal(body, &q); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if q.Flags&vscodeGalleryIncludeLatestVersionOnly != 0 {
			http.Error(w, "IncludeLatestVersionOnly must not be set", http.StatusBadRequest)
			return
		}

		wanted := make(map[string]bool)
		if len(q.Filters) > 0 {
			for _, c := range q.Filters[0].Criteria {
				wanted[strings.ToLower(c.Value)] = true
			}
		}

		var exts []vscodeGalleryExtension
		for id, versions := range byID {
			if !wanted[strings.ToLower(id)] {
				continue
			}
			publisher, name, _ := strings.Cut(id, ".")
			exts = append(exts, vscodeGalleryExtension{
				ExtensionName: name,
				Publisher:     vscodeGalleryPublisher{PublisherName: publisher},
				Versions:      versions,
			})
		}
		payload := vscodeGalleryResponse{
			Results: []vscodeGalleryResult{{Extensions: exts}},
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(payload); err != nil {
			t.Errorf("encode gallery response: %v", err)
		}
	}))
}
