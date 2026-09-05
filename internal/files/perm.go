package files

import (
	"fmt"
	"io/fs"
	"os"

	"github.com/ks1686/genv/internal/schema"
)

const kindPermMismatch = "perm-mismatch"

// applyPermIfNeeded chmods path to perm when the live mode differs.
// Empty perm is a no-op. Dry-run reports whether a chmod would happen.
func applyPermIfNeeded(path, perm string, opts ApplyOptions) (changed bool, err error) {
	if perm == "" {
		return false, nil
	}
	want, err := schema.ParseFilePerm(perm)
	if err != nil {
		return false, fmt.Errorf("%s: %w", path, err)
	}
	got, err := currentPerm(path)
	if err != nil {
		return false, err
	}
	if got == want.Perm() {
		return false, nil
	}
	if opts.DryRun {
		return true, nil
	}
	if err := os.Chmod(path, want); err != nil {
		return false, fmt.Errorf("chmod %s: %w", path, err)
	}
	return true, nil
}

func currentPerm(path string) (fs.FileMode, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return 0, fmt.Errorf("stat %s: %w", path, err)
	}
	return fi.Mode().Perm(), nil
}

func recordPermChange(res *ApplyResult, resultPath string, created bool, changed bool) {
	if !changed || created {
		return
	}
	res.Skipped = removePath(res.Skipped, resultPath)
	if !containsPath(res.Updated, resultPath) {
		res.Updated = append(res.Updated, resultPath)
	}
}

func containsPath(list []string, path string) bool {
	for _, p := range list {
		if p == path {
			return true
		}
	}
	return false
}

func removePath(list []string, path string) []string {
	out := list[:0]
	for _, p := range list {
		if p != path {
			out = append(out, p)
		}
	}
	return out
}

func statusPerm(entry StatusEntry, path, perm string) (StatusEntry, error) {
	if perm == "" || entry.Kind != "ok" {
		return entry, nil
	}
	want, err := schema.ParseFilePerm(perm)
	if err != nil {
		return entry, fmt.Errorf("%s: %w", path, err)
	}
	got, err := currentPerm(path)
	if err != nil {
		return entry, err
	}
	if got != want.Perm() {
		entry.Kind = kindPermMismatch
	}
	return entry, nil
}
