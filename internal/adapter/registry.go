package adapter

import (
	"encoding/json"
	"fmt"
	"io"
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

// cratesRegistryBase is the crates.io endpoint used to look up the latest
// published version of a crate. It is a var so tests can point it at a local
// httptest server.
var cratesRegistryBase = "https://crates.io"

// registryHTTPClient is the client used for package-registry lookups. The short
// timeout keeps an hourly update check from blocking on a slow or unreachable
// network.
var registryHTTPClient = &http.Client{Timeout: 5 * time.Second}

// npmLatestVersion resolves the "latest" dist-tag version of an npm package.
// It is a package-level var so tests can inject canned versions without any
// network access, mirroring the lookPath seam in adapter.go.
var npmLatestVersion = fetchNpmLatest

// pypiLatestVersion resolves the latest published version of a PyPI package.
// It is a package-level var so tests can inject canned versions without any
// network access.
var pypiLatestVersion = fetchPypiLatest

// cratesLatestVersion resolves the latest published version of a crates.io crate.
// It is a package-level var so tests can inject canned versions without any
// network access.
var cratesLatestVersion = fetchCratesLatest

const cratesUserAgent = "genv (https://github.com/ks1686/genv)"

// fetchRegistryLatest GETs endpoint, treating 404 as unknown ("" , nil) and other
// non-200 statuses as errors. extract pulls the version string from a 200 body.
func fetchRegistryLatest(endpoint, label string, header http.Header, extract func([]byte) (string, error)) (string, error) {
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return "", err
	}
	for k, vals := range header {
		for _, v := range vals {
			req.Header.Add(k, v)
		}
	}
	resp, err := registryHTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return "", nil
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("%s: %s returned %s", label, endpoint, resp.Status)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("%s: read %s: %w", label, endpoint, err)
	}
	version, err := extract(body)
	if err != nil {
		return "", fmt.Errorf("%s: decode: %w", label, err)
	}
	return version, nil
}

// fetchNpmLatest queries the npm registry for the latest published version of
// name. Scoped names (@scope/pkg) are URL-encoded (the "/" becomes %2F). Returns
// "" with no error when the package is unknown (404); network and transport
// failures are returned as errors so callers can fall back conservatively.
func fetchNpmLatest(name string) (string, error) {
	endpoint := npmRegistryBase + "/" + url.PathEscape(name) + "/latest"
	return fetchRegistryLatest(endpoint, "npm registry", nil, func(body []byte) (string, error) {
		var payload struct {
			Version string `json:"version"`
		}
		if err := json.Unmarshal(body, &payload); err != nil {
			return "", err
		}
		return payload.Version, nil
	})
}

// fetchPypiLatest queries PyPI for the latest published version of name.
// Returns "" with no error when the package is unknown (404); network and
// transport failures are returned as errors so callers can fall back
// conservatively.
func fetchPypiLatest(name string) (string, error) {
	endpoint := pypiRegistryBase + "/pypi/" + url.PathEscape(name) + "/json"
	return fetchRegistryLatest(endpoint, "PyPI registry", nil, func(body []byte) (string, error) {
		var payload struct {
			Info struct {
				Version string `json:"version"`
			} `json:"info"`
		}
		if err := json.Unmarshal(body, &payload); err != nil {
			return "", err
		}
		return payload.Info.Version, nil
	})
}

// fetchCratesLatest queries crates.io for the latest published version of name.
// Returns "" with no error when the crate is unknown (404); network and
// transport failures are returned as errors so callers can fall back
// conservatively.
func fetchCratesLatest(name string) (string, error) {
	endpoint := cratesRegistryBase + "/api/v1/crates/" + url.PathEscape(name)
	header := http.Header{"User-Agent": {cratesUserAgent}}
	return fetchRegistryLatest(endpoint, "crates.io registry", header, func(body []byte) (string, error) {
		var payload struct {
			Crate struct {
				MaxVersion string `json:"max_version"`
			} `json:"crate"`
		}
		if err := json.Unmarshal(body, &payload); err != nil {
			return "", err
		}
		return payload.Crate.MaxVersion, nil
	})
}
