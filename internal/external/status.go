package external

import (
	"context"

	"github.com/ks1686/genv/internal/genvfile"
	"github.com/ks1686/genv/internal/schema"
)

// LocalState is the detected local state of a recipe-backed external package.
type LocalState struct {
	Present bool
	Version string
	Drift   bool
}

// InspectLocal detects a package and compares its recipe and owned paths with the receipt.
func InspectLocal(ctx context.Context, pkg schema.Package, locked *genvfile.LockedPackage) LocalState {
	if pkg.External == nil {
		return LocalState{}
	}
	version, err := detectVersion(ctx, pkg.External.Detect)
	if err != nil {
		return LocalState{}
	}
	state := LocalState{Present: true, Version: version}
	if locked == nil || locked.External == nil {
		return state
	}
	if version != locked.InstalledVersion {
		state.Drift = true
	}
	recipeDigest, err := hashRecipe(pkg.External)
	if err != nil || recipeDigest != locked.External.RecipeSHA256 {
		state.Drift = true
	}
	for _, pathReceipt := range locked.External.Paths {
		digest, err := fileSHA256(pathReceipt.Path)
		if err != nil {
			state.Drift = true
			continue
		}
		if err := VerifySHA256(digest, pathReceipt.SHA256); err != nil {
			state.Drift = true
		}
	}
	return state
}
