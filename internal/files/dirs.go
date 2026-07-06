package files

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ks1686/genv/internal/schema"
)

func applyDir(ctx context.Context, d schema.FileDir, opts ApplyOptions, res *ApplyResult) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	target, err := expandPath(d.Target)
	if err != nil {
		return fmt.Errorf("dir %q: %w", d.Target, err)
	}
	target = filepath.Clean(target)

	fi, err := os.Lstat(target)
	if err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("dir %s: %w", target, err)
		}
		if opts.DryRun {
			res.Created = append(res.Created, target)
			return nil
		}
		if err := os.MkdirAll(target, 0o755); err != nil {
			return fmt.Errorf("dir %s: %w", target, err)
		}
		res.Created = append(res.Created, target)
		return nil
	}

	if fi.IsDir() {
		res.Skipped = append(res.Skipped, target)
		return nil
	}

	if opts.DryRun {
		res.Updated = append(res.Updated, target)
		return nil
	}
	if !opts.Force {
		res.Mismatched = append(res.Mismatched, target)
		return nil
	}

	if opts.Backup {
		if err := backupExisting(target); err != nil {
			return err
		}
	} else {
		if err := os.RemoveAll(target); err != nil {
			return fmt.Errorf("dir %s: remove existing: %w", target, err)
		}
	}
	if err := os.MkdirAll(target, 0o755); err != nil {
		return fmt.Errorf("dir %s: %w", target, err)
	}
	res.Updated = append(res.Updated, target)
	return nil
}
