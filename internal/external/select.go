package external

import (
	"fmt"
	"net/url"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"github.com/ks1686/genv/internal/schema"
)

// Host contains facts used to select one platform recipe.
type Host struct {
	OS   string
	Arch string
	Libc string
}

// CurrentHost returns normalized release-selection facts for the running host.
func CurrentHost() Host {
	host := Host{OS: runtime.GOOS, Arch: runtime.GOARCH}
	if runtime.GOOS == "linux" {
		host.Libc = "glibc"
		if matches, _ := filepath.Glob("/lib/ld-musl-*.so.1"); len(matches) > 0 {
			host.Libc = "musl"
		}
	}
	return host
}

// SelectPlatform requires exactly one platform recipe to match host.
func SelectPlatform(platforms []schema.ExternalPlatform, host Host) (schema.ExternalPlatform, error) {
	var matches []schema.ExternalPlatform
	for _, platform := range platforms {
		if contains(platform.OS, host.OS) && contains(platform.Arch, host.Arch) && (len(platform.Libc) == 0 || contains(platform.Libc, host.Libc)) {
			matches = append(matches, platform)
		}
	}
	switch len(matches) {
	case 0:
		return schema.ExternalPlatform{}, fmt.Errorf("no external platform matches %s/%s", host.OS, host.Arch)
	case 1:
		return matches[0], nil
	default:
		return schema.ExternalPlatform{}, fmt.Errorf("multiple external platforms match %s/%s", host.OS, host.Arch)
	}
}

// SelectAsset requires a platform expression to match exactly one release asset.
func SelectAsset(release Release, platform schema.ExternalPlatform) (Asset, error) {
	re, err := regexp.Compile(platform.AssetRegex)
	if err != nil {
		return Asset{}, fmt.Errorf("compile asset regex: %w", err)
	}
	var matches []Asset
	for _, asset := range release.Assets {
		if re.MatchString(asset.Name) {
			matches = append(matches, asset)
		}
	}
	switch len(matches) {
	case 0:
		return Asset{}, fmt.Errorf("no release asset matches %q", platform.AssetRegex)
	case 1:
		return matches[0], nil
	default:
		return Asset{}, fmt.Errorf("multiple release assets match %q", platform.AssetRegex)
	}
}

// ExpandArtifactURL substitutes documented release and host placeholders.
func ExpandArtifactURL(template string, release Release, host Host) (string, error) {
	values := map[string]string{"version": release.Version, "tag": release.Tag, "os": host.OS, "arch": host.Arch}
	re := regexp.MustCompile(`\{([A-Za-z][A-Za-z0-9]*)\}`)
	var unknown string
	result := re.ReplaceAllStringFunc(template, func(match string) string {
		name := match[1 : len(match)-1]
		value, ok := values[name]
		if !ok {
			unknown = name
			return match
		}
		return url.PathEscape(value)
	})
	if unknown != "" {
		return "", fmt.Errorf("unknown artifact URL placeholder %q", unknown)
	}
	if strings.Contains(result, "{") || strings.Contains(result, "}") {
		return "", fmt.Errorf("invalid artifact URL template")
	}
	return result, nil
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
