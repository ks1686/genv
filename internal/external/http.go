package external

import (
	"context"
	"fmt"
	"mime"
	"regexp"
	"strings"

	"github.com/ks1686/genv/internal/schema"
)

func (c Client) resolveHTTP(ctx context.Context, source schema.ExternalSource, allowInsecure bool) (Release, error) {
	body, contentType, err := c.get(ctx, source.VersionURL, allowInsecure, nil)
	if err != nil {
		return Release{}, err
	}
	mediaType, _, _ := mime.ParseMediaType(contentType)
	if mediaType == "text/html" || mediaType == "application/xhtml+xml" {
		return Release{}, fmt.Errorf("external HTTP release endpoint returned HTML")
	}
	var version string
	switch source.Format {
	case "json":
		version, err = jsonPointer(body, source.VersionPointer)
	case "text":
		re, compileErr := regexp.Compile(source.VersionRegex)
		if compileErr != nil {
			return Release{}, fmt.Errorf("compile HTTP version regex: %w", compileErr)
		}
		match := re.FindSubmatch(body)
		if len(match) != 2 {
			return Release{}, fmt.Errorf("HTTP release metadata does not match version regex")
		}
		version = strings.TrimSpace(string(match[1]))
	default:
		return Release{}, fmt.Errorf("unsupported HTTP release format %q", source.Format)
	}
	if err != nil {
		return Release{}, err
	}
	if version == "" {
		return Release{}, fmt.Errorf("external HTTP release version is empty")
	}
	return Release{Version: version, Metadata: append([]byte(nil), body...)}, nil
}
