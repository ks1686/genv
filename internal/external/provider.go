// Package external resolves and manages release-backed external packages.
package external

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/ks1686/genv/internal/schema"
)

const defaultMaxMetadataBytes int64 = 1 << 20

// Asset is one downloadable file attached to a release.
type Asset struct {
	Name   string
	URL    string
	Digest string
}

// Release is normalized metadata returned by a release provider.
type Release struct {
	ID       string
	Tag      string
	Version  string
	Assets   []Asset
	Metadata json.RawMessage
}

// Client resolves external release metadata through an injected HTTP client.
type Client struct {
	HTTPClient       *http.Client
	GitHubToken      string
	MaxMetadataBytes int64
}

// Resolve returns the latest release selected by source.
func (c Client) Resolve(ctx context.Context, source schema.ExternalSource, allowInsecure bool) (Release, error) {
	switch source.Type {
	case "githubRelease":
		return c.resolveGitHub(ctx, source, allowInsecure)
	case "httpRelease":
		return c.resolveHTTP(ctx, source, allowInsecure)
	default:
		return Release{}, fmt.Errorf("unsupported external source type %q", source.Type)
	}
}

func (c Client) httpClient() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}
	return &http.Client{Timeout: 15 * time.Second}
}

func (c Client) maxMetadataBytes() int64 {
	if c.MaxMetadataBytes > 0 {
		return c.MaxMetadataBytes
	}
	return defaultMaxMetadataBytes
}

func (c Client) get(ctx context.Context, endpoint string, allowInsecure bool, headers http.Header) ([]byte, string, error) {
	u, err := url.Parse(endpoint)
	if err != nil || u.Host == "" || (u.Scheme != "https" && !(allowInsecure && u.Scheme == "http")) {
		return nil, "", fmt.Errorf("external metadata URL must use HTTPS")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, "", fmt.Errorf("build external metadata request: %w", err)
	}
	for key, values := range headers {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("fetch external metadata from %s: %w", u.Host, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, "", fmt.Errorf("fetch external metadata from %s: HTTP %d", u.Host, resp.StatusCode)
	}
	limit := c.maxMetadataBytes()
	body, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, "", fmt.Errorf("read external metadata from %s: %w", u.Host, err)
	}
	if int64(len(body)) > limit {
		return nil, "", fmt.Errorf("external metadata from %s exceeds %d bytes", u.Host, limit)
	}
	return body, resp.Header.Get("Content-Type"), nil
}

func captureVersion(value, expression string) (string, error) {
	if expression == "" {
		return value, nil
	}
	re, err := regexp.Compile(expression)
	if err != nil {
		return "", fmt.Errorf("compile version regex: %w", err)
	}
	match := re.FindStringSubmatch(value)
	if len(match) != 2 || match[1] == "" {
		return "", fmt.Errorf("value %q does not match version regex", value)
	}
	return match[1], nil
}

func jsonPointer(raw []byte, pointer string) (string, error) {
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", fmt.Errorf("parse external JSON metadata: %w", err)
	}
	if pointer == "" || pointer[0] != '/' {
		return "", fmt.Errorf("invalid JSON pointer %q", pointer)
	}
	current := value
	for _, token := range strings.Split(pointer[1:], "/") {
		token = strings.ReplaceAll(strings.ReplaceAll(token, "~1", "/"), "~0", "~")
		switch node := current.(type) {
		case map[string]any:
			var ok bool
			current, ok = node[token]
			if !ok {
				return "", fmt.Errorf("JSON pointer %q does not exist", pointer)
			}
		case []any:
			index, err := strconv.Atoi(token)
			if err != nil || index < 0 || index >= len(node) {
				return "", fmt.Errorf("JSON pointer %q does not exist", pointer)
			}
			current = node[index]
		default:
			return "", fmt.Errorf("JSON pointer %q traverses a scalar", pointer)
		}
	}
	version, ok := current.(string)
	if !ok || version == "" {
		return "", fmt.Errorf("JSON pointer %q must select a non-empty string", pointer)
	}
	return version, nil
}
