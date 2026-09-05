// Package files applies schema v5 file entries to the filesystem.
package files

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ks1686/genv/internal/host"
	"github.com/ks1686/genv/internal/schema"
)

// ApplyOptions controls how Apply reconciles file entries.
type ApplyOptions struct {
	Force      bool
	DryRun     bool
	Backup     bool
	SourceRoot string // base directory for relative Source paths
}

// ApplyResult reports what Apply did or would do.
type ApplyResult struct {
	Created    []string
	Updated    []string
	Skipped    []string
	Mismatched []string
	Errors     []error
}

// Apply reconciles cfg against the filesystem on the named host.
// It filters links, templates, and dirs by host, expands paths, creates missing
// entries, and reports mismatches. With Force and Backup it preserves existing
// targets.
func Apply(ctx context.Context, cfg *schema.FilesConfig, hostName string, opts ApplyOptions) (*ApplyResult, error) {
	res := &ApplyResult{}
	if cfg == nil {
		return res, nil
	}
	if err := ctx.Err(); err != nil {
		return res, err
	}

	filtered := filter(cfg, hostName)
	for _, d := range filtered.Dirs {
		if err := applyDir(ctx, d, opts, res); err != nil {
			res.Errors = append(res.Errors, err)
		}
	}
	for _, l := range filtered.Links {
		if err := applyLink(ctx, l, opts, res); err != nil {
			res.Errors = append(res.Errors, err)
		}
	}
	for _, tmpl := range filtered.Templates {
		if err := applyTemplate(ctx, tmpl, hostName, opts, res); err != nil {
			res.Errors = append(res.Errors, err)
		}
	}

	if len(res.Errors) > 0 || len(res.Mismatched) > 0 {
		return res, summarize(res)
	}
	return res, nil
}

func filter(cfg *schema.FilesConfig, hostName string) *schema.FilesConfig {
	out := &schema.FilesConfig{}
	for _, d := range cfg.Dirs {
		if host.Match(d.Host, hostName) {
			out.Dirs = append(out.Dirs, d)
		}
	}
	for _, l := range cfg.Links {
		if host.Match(l.Host, hostName) {
			out.Links = append(out.Links, l)
		}
	}
	for _, tmpl := range cfg.Templates {
		if host.Match(tmpl.Host, hostName) {
			out.Templates = append(out.Templates, tmpl)
		}
	}
	return out
}

func applyTemplate(ctx context.Context, tmpl schema.FileTemplate, hostName string, opts ApplyOptions, res *ApplyResult) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	target, err := expandPath(tmpl.Target)
	if err != nil {
		return fmt.Errorf("template target %q: %w", tmpl.Target, err)
	}
	target = filepath.Clean(target)

	source, err := resolveSource(opts.SourceRoot, tmpl.Source)
	if err != nil {
		return fmt.Errorf("template source %q: %w", tmpl.Source, err)
	}

	rendered, err := renderedTemplate(source, hostName)
	if err != nil {
		return fmt.Errorf("template render %s: %w", source, err)
	}

	renderOpts := RenderOptions{
		Force:  opts.Force,
		Backup: opts.Backup || tmpl.Backup,
		DryRun: opts.DryRun,
	}

	fi, err := os.Lstat(target)
	if err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("template %s: %w", target, err)
		}
		if opts.DryRun {
			res.Created = append(res.Created, target)
			return nil
		}
		if err := RenderTemplate(source, target, hostName, renderOpts); err != nil {
			return fmt.Errorf("template %s: %w", target, err)
		}
		if _, err := applyPermIfNeeded(target, tmpl.Perm, opts); err != nil {
			return fmt.Errorf("template %s: %w", target, err)
		}
		res.Created = append(res.Created, target)
		return nil
	}

	if fi.IsDir() {
		return fmt.Errorf("template %s: target is a directory", target)
	}

	existing, err := os.ReadFile(target)
	if err != nil {
		return fmt.Errorf("template %s: read target: %w", target, err)
	}
	if string(existing) == string(rendered) {
		changed, err := applyPermIfNeeded(target, tmpl.Perm, opts)
		if err != nil {
			return fmt.Errorf("template %s: %w", target, err)
		}
		if changed {
			res.Updated = append(res.Updated, target)
			return nil
		}
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

	if err := RenderTemplate(source, target, hostName, renderOpts); err != nil {
		return fmt.Errorf("template %s: %w", target, err)
	}
	if _, err := applyPermIfNeeded(target, tmpl.Perm, opts); err != nil {
		return fmt.Errorf("template %s: %w", target, err)
	}
	res.Updated = append(res.Updated, target)
	return nil
}

// summaryError reports the aggregate mismatch/error counts as a
// human-readable string via Error, while Unwrap exposes the underlying
// collected errors so callers can errors.Is / errors.As past the summary.
type summaryError struct {
	summary string
	errs    []error
}

func (e *summaryError) Error() string { return e.summary }

func (e *summaryError) Unwrap() []error { return e.errs }

func summarize(res *ApplyResult) error {
	var parts []string
	if n := len(res.Mismatched); n > 0 {
		parts = append(parts, fmt.Sprintf("%d mismatch(es)", n))
	}
	if n := len(res.Errors); n > 0 {
		parts = append(parts, fmt.Sprintf("%d error(s)", n))
	}
	return &summaryError{summary: strings.Join(parts, ", "), errs: res.Errors}
}
