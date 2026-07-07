package files

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/ks1686/genv/internal/schema"
)

// StatusEntry describes one file parity result.
type StatusEntry struct {
	Source string
	Target string
	Mode   string
	Kind   string
}

// StatusResult is the files-only parity result for genv status --files.
type StatusResult struct {
	Entries []StatusEntry
	OK      bool
}

// Status compares cfg against the live filesystem for hostName.
// Todo 10 replaces this stub with the real comparator.
func Status(cfg *schema.FilesConfig, hostName string) (*StatusResult, error) {
	res := &StatusResult{OK: true}
	if cfg == nil {
		return res, nil
	}

	filtered := filter(cfg, hostName)
	for _, d := range filtered.Dirs {
		entry, err := statusDir(d)
		if err != nil {
			return res, err
		}
		res.add(entry)
	}
	for _, l := range filtered.Links {
		if l.Mode == "merge-dir" {
			entries, err := statusMergeDir(l)
			if err != nil {
				return res, err
			}
			for _, entry := range entries {
				res.add(entry)
			}
			continue
		}
		entry, err := statusLink(l)
		if err != nil {
			return res, err
		}
		res.add(entry)
	}
	for _, tmpl := range filtered.Templates {
		entry, err := statusTemplate(tmpl, hostName)
		if err != nil {
			return res, err
		}
		res.add(entry)
	}
	return res, nil
}

func (r *StatusResult) add(entry StatusEntry) {
	r.Entries = append(r.Entries, entry)
	if entry.Kind != "ok" {
		r.OK = false
	}
}

func statusDir(d schema.FileDir) (StatusEntry, error) {
	target, err := statusTarget(d.Target)
	if err != nil {
		return StatusEntry{}, fmt.Errorf("dir target %q: %w", d.Target, err)
	}
	entry := StatusEntry{Target: target, Mode: "dir"}
	fi, err := os.Lstat(target)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			entry.Kind = "missing"
			return entry, nil
		}
		return entry, fmt.Errorf("dir %s: %w", target, err)
	}
	if fi.IsDir() {
		entry.Kind = "ok"
	} else {
		entry.Kind = "wrong-type"
	}
	return entry, nil
}

func statusLink(l schema.FileLink) (StatusEntry, error) {
	target, err := statusTarget(l.Target)
	if err != nil {
		return StatusEntry{}, fmt.Errorf("link target %q: %w", l.Target, err)
	}
	source, err := resolveSource("", l.Source)
	if err != nil {
		return StatusEntry{}, fmt.Errorf("link source %q: %w", l.Source, err)
	}
	mode := l.Mode
	if mode == "" {
		mode = "link"
	}
	return statusLinkAt(target, source, mode)
}

// statusMergeDir reports one StatusEntry per file found under l's source
// directory, checking each against target/<relative path> exactly like
// statusLink checks a single file.
func statusMergeDir(l schema.FileLink) ([]StatusEntry, error) {
	sourceDir, err := resolveSource("", l.Source)
	if err != nil {
		return nil, fmt.Errorf("merge-dir source %q: %w", l.Source, err)
	}
	targetDir, err := statusTarget(l.Target)
	if err != nil {
		return nil, fmt.Errorf("merge-dir target %q: %w", l.Target, err)
	}

	info, err := os.Stat(sourceDir)
	if err != nil {
		return nil, fmt.Errorf("merge-dir source %q: %w", l.Source, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("merge-dir source %q: not a directory", l.Source)
	}

	var entries []StatusEntry
	walkErr := filepath.WalkDir(sourceDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(sourceDir, path)
		if err != nil {
			return fmt.Errorf("merge-dir %s: %w", path, err)
		}
		entry, err := statusLinkAt(filepath.Join(targetDir, rel), path, "merge-dir")
		if err != nil {
			return err
		}
		entries = append(entries, entry)
		return nil
	})
	if walkErr != nil {
		return nil, walkErr
	}
	return entries, nil
}

// statusLinkAt is the single-file check shared by statusLink and
// statusMergeDir: is target a symlink pointing at source?
func statusLinkAt(target, source, mode string) (StatusEntry, error) {
	entry := StatusEntry{Source: source, Target: target, Mode: mode}
	fi, err := os.Lstat(target)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			entry.Kind = "missing"
			return entry, nil
		}
		return entry, fmt.Errorf("link %s: %w", target, err)
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		entry.Kind = "wrong-type"
		return entry, nil
	}
	cur, err := os.Readlink(target)
	if err != nil {
		return entry, fmt.Errorf("link %s: readlink: %w", target, err)
	}
	if cur == source {
		entry.Kind = "ok"
	} else {
		entry.Kind = "mismatch"
	}
	return entry, nil
}

func statusTemplate(tmpl schema.FileTemplate, hostName string) (StatusEntry, error) {
	target, err := statusTarget(tmpl.Target)
	if err != nil {
		return StatusEntry{}, fmt.Errorf("template target %q: %w", tmpl.Target, err)
	}
	source, err := resolveSource("", tmpl.Source)
	if err != nil {
		return StatusEntry{}, fmt.Errorf("template source %q: %w", tmpl.Source, err)
	}
	entry := StatusEntry{Source: source, Target: target, Mode: "copy"}
	fi, err := os.Lstat(target)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			entry.Kind = "missing"
			return entry, nil
		}
		return entry, fmt.Errorf("template %s: %w", target, err)
	}
	if fi.IsDir() || fi.Mode()&os.ModeSymlink != 0 {
		entry.Kind = "wrong-type"
		return entry, nil
	}
	rendered, err := renderedTemplate(source, hostName)
	if err != nil {
		return entry, err
	}
	existing, err := os.ReadFile(target)
	if err != nil {
		return entry, fmt.Errorf("template %s: read target: %w", target, err)
	}
	if bytes.Equal(existing, rendered) {
		entry.Kind = "ok"
	} else {
		entry.Kind = "mismatch"
	}
	return entry, nil
}

func statusTarget(target string) (string, error) {
	expanded, err := expandPath(target)
	if err != nil {
		return "", err
	}
	return filepath.Clean(expanded), nil
}

func renderedTemplate(source, hostName string) ([]byte, error) {
	srcData, err := os.ReadFile(source)
	if err != nil {
		return nil, fmt.Errorf("template source %s: %w", source, err)
	}
	ctx, err := defaultContext()
	if err != nil {
		return nil, fmt.Errorf("build render context: %w", err)
	}
	if hostName != "" {
		ctx.host = hostName
	}
	return []byte(renderWithContext(string(srcData), ctx)), nil
}
