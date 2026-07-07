package files

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"

	"github.com/ks1686/genv/internal/schema"
)

// symlink creates a symlink like os.Symlink, but on Windows augments a
// failure with an actionable privilege hint. The original error is
// preserved for errors.Is / errors.As.
func symlink(source, target string) error {
	return windowsSymlinkHint(runtime.GOOS, os.Symlink(source, target))
}

// windowsSymlinkHint wraps a Windows symlink failure with a hint about
// enabling Developer Mode or running as Administrator, since unprivileged
// Windows processes cannot create symlinks. On other platforms, or for a
// nil error, it returns err unchanged so non-Windows behavior is identical.
func windowsSymlinkHint(goos string, err error) error {
	if err == nil || goos != "windows" {
		return err
	}
	return fmt.Errorf("%w (on Windows, creating symlinks requires enabling Developer Mode or running as Administrator)", err)
}

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

	if l.Mode == "merge-dir" {
		return applyMergeDir(ctx, source, target, l, opts, res)
	}

	return applyLinkAt(target, source, l.Mode == "managed-link", opts.Backup || l.Backup, opts, res)
}

// applyMergeDir walks source (which must be a directory) and, for every
// regular file found under it, ensures target/<relative path> is a symlink
// back to that file. Every file is treated with managed-link's self-healing
// semantics: a wrong or dangling symlink there is silently relinked without
// needing --force. This lets multiple merge-dir records target the same
// directory (e.g. one host-filtered, one not) and layer — a later record's
// files silently override an earlier record's same-named files, while a
// hand-authored real file at that path still requires --force to replace.
func applyMergeDir(ctx context.Context, sourceDir, targetDir string, l schema.FileLink, opts ApplyOptions, res *ApplyResult) error {
	info, err := os.Stat(sourceDir)
	if err != nil {
		return fmt.Errorf("merge-dir source %q: %w", l.Source, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("merge-dir source %q: not a directory", l.Source)
	}

	return filepath.WalkDir(sourceDir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(sourceDir, path)
		if err != nil {
			return fmt.Errorf("merge-dir %s: %w", path, err)
		}
		return applyLinkAt(filepath.Join(targetDir, rel), path, true, opts.Backup || l.Backup, opts, res)
	})
}

// applyLinkAt ensures target is a symlink to source. When managed is true,
// a wrong or dangling existing symlink is relinked without requiring
// --force (mirrors "managed-link" and every file under a "merge-dir").
func applyLinkAt(target, source string, managed, backup bool, opts ApplyOptions, res *ApplyResult) error {
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
		if err := symlink(source, target); err != nil {
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
		if managed || opts.Force {
			return replaceLinkAt(target, source, backup, opts, res)
		}
		res.Mismatched = append(res.Mismatched, target)
		return nil
	}

	// Target is a real file or directory.
	if !opts.Force {
		res.Mismatched = append(res.Mismatched, target)
		return nil
	}
	return replaceLinkAt(target, source, backup, opts, res)
}

func replaceLinkAt(target, source string, backup bool, opts ApplyOptions, res *ApplyResult) error {
	if opts.DryRun {
		res.Updated = append(res.Updated, target)
		return nil
	}
	if err := ensureParentDir(target); err != nil {
		return err
	}
	if backup {
		if err := backupExisting(target); err != nil {
			return err
		}
	} else {
		if err := os.RemoveAll(target); err != nil {
			return fmt.Errorf("link %s: remove existing: %w", target, err)
		}
	}
	if err := symlink(source, target); err != nil {
		return fmt.Errorf("link %s: %w", target, err)
	}
	res.Updated = append(res.Updated, target)
	return nil
}
