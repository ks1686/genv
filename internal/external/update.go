package external

import (
	"context"
	"fmt"

	"github.com/ks1686/genv/internal/schema"
)

// LatestVersion resolves release metadata without downloading the artifact.
func (e Engine) LatestVersion(ctx context.Context, pkg schema.Package) (string, error) {
	if pkg.External == nil {
		return "", fmt.Errorf("external recipe is required")
	}
	release, err := e.Client.Resolve(ctx, pkg.External.Source, pkg.External.AllowInsecureHTTP)
	if err != nil {
		return "", err
	}
	if _, err := SelectPlatform(pkg.External.Platforms, e.Host); err != nil {
		return "", err
	}
	return release.Version, nil
}
