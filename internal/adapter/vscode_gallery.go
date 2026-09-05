package adapter

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// Gallery query flags from the VS Code extension gallery protocol.
// IncludeLatestVersionOnly (0x200) is intentionally omitted: that flag
// returns versions[0], which is often a pre-release that
// `code --install-extension --force` cannot install.
const (
	vscodeGalleryIncludeVersions          = 0x1
	vscodeGalleryIncludeVersionProperties = 0x10
	vscodeGalleryIncludeLatestVersionOnly = 0x200
	vscodeGalleryFilterExtensionName      = 7
	vscodeGalleryQueryChunk               = 100
	vscodePreReleaseProperty              = "Microsoft.VisualStudio.Code.PreRelease"
	vscodeDefaultGalleryBase              = "https://marketplace.visualstudio.com/_apis/public/gallery"
	vscodeGalleryUserAgent                = "genv (https://github.com/ks1686/genv)"
	vscodeGalleryAccept                   = "application/json;api-version=3.0-preview.1"
)

// vscodeGalleryBase is the extensionsGallery.serviceUrl used for outdated
// queries. Empty means auto-detect from the resolved editor CLI's
// product.json (so Cursor hits marketplace.cursorapi.com), falling back
// to the public VS Code marketplace. Tests point it at an httptest server.
var vscodeGalleryBase string

type vscodeGalleryQuery struct {
	Filters []vscodeGalleryFilter `json:"filters"`
	Flags   int                   `json:"flags"`
}

type vscodeGalleryFilter struct {
	Criteria   []vscodeGalleryCriterion `json:"criteria"`
	PageNumber int                      `json:"pageNumber"`
	PageSize   int                      `json:"pageSize"`
}

type vscodeGalleryCriterion struct {
	FilterType int    `json:"filterType"`
	Value      string `json:"value"`
}

type vscodeGalleryResponse struct {
	Results []vscodeGalleryResult `json:"results"`
}

type vscodeGalleryResult struct {
	Extensions []vscodeGalleryExtension `json:"extensions"`
}

type vscodeGalleryExtension struct {
	ExtensionName string                 `json:"extensionName"`
	Publisher     vscodeGalleryPublisher `json:"publisher"`
	Versions      []vscodeGalleryVersion `json:"versions"`
}

type vscodeGalleryPublisher struct {
	PublisherName string `json:"publisherName"`
}

func (e vscodeGalleryExtension) id() string {
	return strings.ToLower(e.Publisher.PublisherName + "." + e.ExtensionName)
}

type vscodeGalleryVersion struct {
	Version    string                  `json:"version"`
	Properties []vscodeGalleryProperty `json:"properties"`
}

type vscodeGalleryProperty struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

func vscodeVersionIsPreRelease(v vscodeGalleryVersion) bool {
	for _, p := range v.Properties {
		if p.Key == vscodePreReleaseProperty && strings.EqualFold(p.Value, "true") {
			return true
		}
	}
	return false
}

// vscodeLatestStableFromVersions returns the newest stable version. The
// gallery lists versions newest-first, including pre-release builds; skipping
// PreRelease=true yields the version `code --install-extension --force`
// actually installs.
func vscodeLatestStableFromVersions(versions []vscodeGalleryVersion) string {
	for _, v := range versions {
		if v.Version == "" || vscodeVersionIsPreRelease(v) {
			continue
		}
		return v.Version
	}
	return ""
}

func vscodeGalleryServiceURL() string {
	if vscodeGalleryBase != "" {
		return vscodeGalleryBase
	}
	if cliPath, err := lookPath(vscodeCLIName()); err == nil {
		if product := vscodeFindProductJSON(cliPath); product != "" {
			if raw, err := os.ReadFile(product); err == nil {
				if u := vscodeGalleryURLFromProductJSON(raw); u != "" {
					return u
				}
			}
		}
	}
	return vscodeDefaultGalleryBase
}

func vscodeGalleryURLFromProductJSON(raw []byte) string {
	var product struct {
		ExtensionsGallery struct {
			ServiceURL string `json:"serviceUrl"`
		} `json:"extensionsGallery"`
	}
	if err := json.Unmarshal(raw, &product); err != nil {
		return ""
	}
	return strings.TrimSpace(product.ExtensionsGallery.ServiceURL)
}

// vscodeFindProductJSON locates product.json relative to the resolved
// editor CLI so Cursor, VS Code, and VSCodium each hit their own gallery.
func vscodeFindProductJSON(codePath string) string {
	resolved, err := filepath.EvalSymlinks(codePath)
	if err != nil {
		resolved = codePath
	}
	dir := filepath.Dir(resolved)
	candidates := []string{
		filepath.Join(dir, "..", "product.json"),
		filepath.Join(dir, "product.json"),
		filepath.Join(dir, "..", "resources", "app", "product.json"),
		filepath.Join(dir, "..", "..", "resources", "app", "product.json"),
		filepath.Join(dir, "..", "..", "Resources", "app", "product.json"),
	}
	for _, c := range candidates {
		c = filepath.Clean(c)
		if info, err := os.Stat(c); err == nil && !info.IsDir() {
			return c
		}
	}
	return ""
}

func fetchVscodeLatestStableVersions(ids []string) (map[string]string, error) {
	unique := uniqueLowerIDs(ids)
	if len(unique) == 0 {
		return nil, nil
	}

	out := make(map[string]string, len(unique))
	for start := 0; start < len(unique); start += vscodeGalleryQueryChunk {
		end := start + vscodeGalleryQueryChunk
		if end > len(unique) {
			end = len(unique)
		}
		chunk, err := fetchVscodeLatestStableChunk(unique[start:end])
		if err != nil {
			return nil, err
		}
		for id, version := range chunk {
			out[id] = version
		}
	}
	return out, nil
}

func uniqueLowerIDs(ids []string) []string {
	seen := make(map[string]bool, len(ids))
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		id = strings.ToLower(strings.TrimSpace(id))
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out
}

func fetchVscodeLatestStableChunk(ids []string) (map[string]string, error) {
	criteria := make([]vscodeGalleryCriterion, 0, len(ids))
	for _, id := range ids {
		criteria = append(criteria, vscodeGalleryCriterion{
			FilterType: vscodeGalleryFilterExtensionName,
			Value:      id,
		})
	}
	query := vscodeGalleryQuery{
		Filters: []vscodeGalleryFilter{{
			Criteria:   criteria,
			PageNumber: 1,
			PageSize:   len(criteria),
		}},
		Flags: vscodeGalleryIncludeVersions | vscodeGalleryIncludeVersionProperties,
	}
	body, err := json.Marshal(query)
	if err != nil {
		return nil, err
	}

	endpoint := strings.TrimRight(vscodeGalleryServiceURL(), "/") + "/extensionquery"
	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", vscodeGalleryAccept)
	req.Header.Set("User-Agent", vscodeGalleryUserAgent)

	resp, err := registryHTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("vscode gallery: %s returned %s", endpoint, resp.Status)
	}
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("vscode gallery: read %s: %w", endpoint, err)
	}
	var payload vscodeGalleryResponse
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, fmt.Errorf("vscode gallery: decode: %w", err)
	}

	out := make(map[string]string)
	for _, result := range payload.Results {
		for _, ext := range result.Extensions {
			if latest := vscodeLatestStableFromVersions(ext.Versions); latest != "" {
				out[ext.id()] = latest
			}
		}
	}
	return out, nil
}
