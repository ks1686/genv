package adapter

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

// npmRegistryBase is the npm registry endpoint used to look up the latest
// published version of a package. It is a var so tests can point it at a local
// httptest server.
var npmRegistryBase = "https://registry.npmjs.org"

// pypiRegistryBase is the PyPI endpoint used to look up the latest published
// version of a package. It is a var so tests can point it at a local httptest
// server.
var pypiRegistryBase = "https://pypi.org"

// npmHTTPClient is the client used for registry lookups. The short timeout keeps
// an hourly update check from blocking on a slow or unreachable network.
var npmHTTPClient = &http.Client{Timeout: 5 * time.Second}

// npmLatestVersion resolves the "latest" dist-tag version of an npm package.
// It is a package-level var so tests can inject canned versions without any
// network access, mirroring the lookPath seam in adapter.go.
var npmLatestVersion = fetchNpmLatest

// pypiLatestVersion resolves the latest published version of a PyPI package.
// It is a package-level var so tests can inject canned versions without any
// network access.
var pypiLatestVersion = fetchPypiLatest

// fetchNpmLatest queries the npm registry for the latest published version of
// name. Scoped names (@scope/pkg) are URL-encoded (the "/" becomes %2F). Returns
// "" with no error when the package is unknown (404); network and transport
// failures are returned as errors so callers can fall back conservatively.
func fetchNpmLatest(name string) (string, error) {
	endpoint := npmRegistryBase + "/" + url.PathEscape(name) + "/latest"
	resp, err := npmHTTPClient.Get(endpoint)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return "", nil
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("npm registry: %s returned %s", name, resp.Status)
	}
	var payload struct {
		Version string `json:"version"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", fmt.Errorf("npm registry: decode %s: %w", name, err)
	}
	return payload.Version, nil
}

// fetchPypiLatest queries PyPI for the latest published version of name.
// Returns "" with no error when the package is unknown (404); network and
// transport failures are returned as errors so callers can fall back
// conservatively.
func fetchPypiLatest(name string) (string, error) {
	endpoint := pypiRegistryBase + "/pypi/" + url.PathEscape(name) + "/json"
	resp, err := npmHTTPClient.Get(endpoint)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return "", nil
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("PyPI registry: %s returned %s", name, resp.Status)
	}
	var payload struct {
		Info struct {
			Version string `json:"version"`
		} `json:"info"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", fmt.Errorf("PyPI registry: decode %s: %w", name, err)
	}
	return payload.Info.Version, nil
}
