package external

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/ks1686/genv/internal/schema"
)

type githubRelease struct {
	ID         int64  `json:"id"`
	Tag        string `json:"tag_name"`
	Draft      bool   `json:"draft"`
	Prerelease bool   `json:"prerelease"`
	Assets     []struct {
		Name   string `json:"name"`
		URL    string `json:"browser_download_url"`
		Digest string `json:"digest"`
	} `json:"assets"`
}

func (c Client) resolveGitHub(ctx context.Context, source schema.ExternalSource, allowInsecure bool) (Release, error) {
	base := strings.TrimRight(source.APIBase, "/")
	if base == "" {
		base = "https://api.github.com"
	}
	parts := strings.Split(source.Repository, "/")
	if len(parts) != 2 {
		return Release{}, fmt.Errorf("invalid GitHub repository %q", source.Repository)
	}
	endpoint := base + "/repos/" + url.PathEscape(parts[0]) + "/" + url.PathEscape(parts[1]) + "/releases?per_page=100"
	headers := make(http.Header)
	headers.Set("Accept", "application/vnd.github+json")
	headers.Set("X-GitHub-Api-Version", "2022-11-28")
	if c.GitHubToken != "" {
		headers.Set("Authorization", "Bearer "+c.GitHubToken)
	}
	body, _, err := c.get(ctx, endpoint, allowInsecure, headers)
	if err != nil {
		return Release{}, err
	}
	var releases []githubRelease
	if err := json.Unmarshal(body, &releases); err != nil {
		return Release{}, fmt.Errorf("parse GitHub releases for %s: %w", source.Repository, err)
	}
	channel := source.Release
	if channel == "" {
		channel = "stable"
	}
	for _, candidate := range releases {
		if candidate.Draft || (channel == "stable" && candidate.Prerelease) || (channel == "prerelease" && !candidate.Prerelease) {
			continue
		}
		version, err := captureVersion(candidate.Tag, source.TagRegex)
		if err != nil {
			return Release{}, err
		}
		release := Release{ID: fmt.Sprint(candidate.ID), Tag: candidate.Tag, Version: version, Metadata: append(json.RawMessage(nil), body...)}
		for _, asset := range candidate.Assets {
			release.Assets = append(release.Assets, Asset{Name: asset.Name, URL: asset.URL, Digest: asset.Digest})
		}
		return release, nil
	}
	return Release{}, fmt.Errorf("no %s GitHub release found for %s", channel, source.Repository)
}
