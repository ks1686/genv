package files

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/ks1686/genv/internal/schema"
)

// AdoptOptions controls how Adopt seeds a source and links a live target.
type AdoptOptions struct {
	DryRun bool
}

// AdoptResult reports the planned or executed steps of Adopt.
type AdoptResult struct {
	Source     string
	Target     string
	BackupPath string
	Seeded     bool
	Steps      []string
}

// Adopt copies the live target to source when source is missing, backs up
// the live target, and creates a symlink from target to source.
func Adopt(source, target string, opts AdoptOptions) (*AdoptResult, error) {
	source = filepath.Clean(source)
	target = filepath.Clean(target)
	res := &AdoptResult{Source: source, Target: target}

	_, srcErr := os.Lstat(source)
	sourceMissing := os.IsNotExist(srcErr)
	if srcErr != nil && !sourceMissing {
		return nil, fmt.Errorf("adopt source %s: %w", source, srcErr)
	}

	fi, err := os.Lstat(target)
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("adopt %s: %w", target, err)
		}
		if sourceMissing {
			return nil, fmt.Errorf("adopt %s: target does not exist and source %s is missing", target, source)
		}
		res.Steps = append(res.Steps, fmt.Sprintf("link %s -> %s", source, target))
		if opts.DryRun {
			return res, nil
		}
		if err := ensureParentDir(target); err != nil {
			return nil, err
		}
		if err := symlink(source, target); err != nil {
			return nil, fmt.Errorf("link %s: %w", target, err)
		}
		return res, nil
	}

	if fi.Mode()&os.ModeSymlink != 0 {
		cur, err := os.Readlink(target)
		if err != nil {
			return nil, fmt.Errorf("adopt %s: readlink: %w", target, err)
		}
		if cur == source {
			return res, nil
		}
		return nil, fmt.Errorf("adopt %s: target is a symlink to %s, not a regular file", target, cur)
	}
	if fi.IsDir() {
		return nil, fmt.Errorf("adopt %s: target is a directory", target)
	}

	if sourceMissing {
		res.Seeded = true
		res.Steps = append(res.Steps, fmt.Sprintf("copy %s -> %s", target, source))
	}
	backupPath := backupPathFor(target)
	res.BackupPath = backupPath
	res.Steps = append(res.Steps, fmt.Sprintf("backup %s -> %s", target, backupPath))
	res.Steps = append(res.Steps, fmt.Sprintf("link %s -> %s", source, target))
	if opts.DryRun {
		return res, nil
	}

	if sourceMissing {
		if err := copyFile(target, source); err != nil {
			return nil, fmt.Errorf("adopt seed %s: %w", source, err)
		}
	}
	actualBackup, err := backupExistingTo(target)
	if err != nil {
		return nil, err
	}
	res.BackupPath = actualBackup
	if err := ensureParentDir(target); err != nil {
		return nil, err
	}
	if err := symlink(source, target); err != nil {
		return nil, fmt.Errorf("link %s: %w", target, err)
	}
	return res, nil
}

// FindLinkByTarget returns the unique files.links entry whose target matches
// want after ~/$VAR expansion. Matching the spec's raw target string also
// succeeds so callers can pass the declared path unchanged.
func FindLinkByTarget(cfg *schema.FilesConfig, want string) (schema.FileLink, error) {
	if cfg == nil || len(cfg.Links) == 0 {
		return schema.FileLink{}, fmt.Errorf("no files.links entry for %s", want)
	}
	expandedWant, err := expandPath(want)
	if err != nil {
		return schema.FileLink{}, err
	}
	expandedWant = filepath.Clean(expandedWant)

	var matches []schema.FileLink
	seen := map[string]bool{}
	for _, l := range cfg.Links {
		key := l.Source + "\x00" + l.Target + "\x00" + l.Mode
		if l.Target == want || filepath.Clean(l.Target) == expandedWant {
			if !seen[key] {
				matches = append(matches, l)
				seen[key] = true
			}
			continue
		}
		expanded, err := expandPath(l.Target)
		if err != nil {
			continue
		}
		if filepath.Clean(expanded) == expandedWant && !seen[key] {
			matches = append(matches, l)
			seen[key] = true
		}
	}
	if len(matches) == 0 {
		return schema.FileLink{}, fmt.Errorf("no files.links entry for %s", want)
	}
	if len(matches) > 1 {
		return schema.FileLink{}, fmt.Errorf("ambiguous files.links target %s", want)
	}
	return matches[0], nil
}

func copyFile(src, dst string) error {
	if err := ensureParentDir(dst); err != nil {
		return err
	}
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	mode := os.FileMode(0o644)
	if info, err := os.Stat(src); err == nil {
		mode = info.Mode().Perm()
	}
	return os.WriteFile(dst, data, mode)
}
