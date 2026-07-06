package files

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ks1686/genv/internal/schema"
)

func applyLink(ctx context.Context, l schema.FileLink, opts ApplyOptions, res *ApplyResult) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	target, err := expandPath(l.Target)
	if err != nil {
		return fmt.Errorf("link target %q: %w", l.Target, err)
	}
	target = filepath.Clean(target)

	source, err := resolveSource(opts.SourceRoot, l.Source)
	if err != nil {
		return fmt.Errorf("link source %q: %w", l.Source, err)
	}

	fi, err := os.Lstat(target)
	if err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("link %s: %w", target, err)
		}
		if opts.DryRun {
			res.Created = append(res.Created, target)
			return nil
		}
		if err := ensureParentDir(target); err != nil {
			return err
		}
		if err := os.Symlink(source, target); err != nil {
			return fmt.Errorf("link %s: %w", target, err)
		}
		res.Created = append(res.Created, target)
		return nil
	}

	if fi.Mode()&os.ModeSymlink != 0 {
		cur, err := os.Readlink(target)
		if err != nil {
			return fmt.Errorf("link %s: readlink: %w", target, err)
		}
		if cur == source {
			res.Skipped = append(res.Skipped, target)
			return nil
		}
		// Wrong or dangling symlink.
		if l.Mode == "managed-link" || opts.Force {
			return replaceLink(target, source, opts, l, res)
		}
		res.Mismatched = append(res.Mismatched, target)
		return nil
	}

	// Target is a real file or directory.
	if l.Mode == "managed-link" && !opts.Force {
		res.Mismatched = append(res.Mismatched, target)
		return nil
	}
	if !opts.Force {
		res.Mismatched = append(res.Mismatched, target)
		return nil
	}
	return replaceLink(target, source, opts, l, res)
}

func replaceLink(target, source string, opts ApplyOptions, l schema.FileLink, res *ApplyResult) error {
	if opts.DryRun {
		res.Updated = append(res.Updated, target)
		return nil
	}
	if err := ensureParentDir(target); err != nil {
		return err
	}
	if opts.Backup || l.Backup {
		if err := backupExisting(target); err != nil {
			return err
		}
	} else {
		if err := os.RemoveAll(target); err != nil {
			return fmt.Errorf("link %s: remove existing: %w", target, err)
		}
	}
	if err := os.Symlink(source, target); err != nil {
		return fmt.Errorf("link %s: %w", target, err)
	}
	res.Updated = append(res.Updated, target)
	return nil
}
