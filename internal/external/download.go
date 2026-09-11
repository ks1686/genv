package external

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
)

const defaultMaxArtifactBytes int64 = 512 << 20

// DownloadedArtifact is a privately staged and hashed payload.
type DownloadedArtifact struct {
	Path   string
	SHA256 string
	Size   int64
}

// Download fetches one payload into dir while enforcing transport and size policy.
func (c Client) Download(ctx context.Context, endpoint string, allowInsecure bool, dir string, maxBytes int64) (DownloadedArtifact, error) {
	u, err := url.Parse(endpoint)
	if err != nil || u.Host == "" || (u.Scheme != "https" && !(allowInsecure && u.Scheme == "http")) {
		return DownloadedArtifact{}, fmt.Errorf("external artifact URL must use HTTPS")
	}
	if maxBytes <= 0 {
		maxBytes = defaultMaxArtifactBytes
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return DownloadedArtifact{}, fmt.Errorf("build artifact request: %w", err)
	}
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return DownloadedArtifact{}, fmt.Errorf("download external artifact from %s: %w", u.Host, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return DownloadedArtifact{}, fmt.Errorf("download external artifact from %s: HTTP %d", u.Host, resp.StatusCode)
	}
	if resp.Request.URL.Scheme != "https" && !allowInsecure {
		return DownloadedArtifact{}, fmt.Errorf("external artifact redirected to insecure transport")
	}
	file, err := os.CreateTemp(dir, ".genv-download-*")
	if err != nil {
		return DownloadedArtifact{}, fmt.Errorf("create artifact staging file: %w", err)
	}
	path := file.Name()
	ok := false
	defer func() {
		_ = file.Close()
		if !ok {
			_ = os.Remove(path)
		}
	}()
	if err := file.Chmod(0o600); err != nil {
		return DownloadedArtifact{}, fmt.Errorf("secure artifact staging file: %w", err)
	}
	hash := sha256.New()
	written, err := io.Copy(io.MultiWriter(file, hash), io.LimitReader(resp.Body, maxBytes+1))
	if err != nil {
		return DownloadedArtifact{}, fmt.Errorf("write artifact staging file: %w", err)
	}
	if written > maxBytes {
		return DownloadedArtifact{}, fmt.Errorf("external artifact exceeds %d bytes", maxBytes)
	}
	if err := file.Sync(); err != nil {
		return DownloadedArtifact{}, fmt.Errorf("sync artifact staging file: %w", err)
	}
	if err := file.Close(); err != nil {
		return DownloadedArtifact{}, fmt.Errorf("close artifact staging file: %w", err)
	}
	ok = true
	return DownloadedArtifact{Path: filepath.Clean(path), SHA256: hex.EncodeToString(hash.Sum(nil)), Size: written}, nil
}
