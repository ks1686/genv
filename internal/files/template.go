// Package files applies schema v5 file entries to the filesystem.
package files

import (
	"errors"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
)

// RenderOptions controls how RenderTemplate reconciles a template against the
// filesystem.
type RenderOptions struct {
	Force  bool
	Backup bool
	DryRun bool
}

// ErrMismatch is returned by RenderTemplate when the destination exists with
// different rendered content and Force is false.
var ErrMismatch = errors.New("target exists with different content")

// RenderString replaces the supported envsubst-style placeholders in input.
// Supported placeholders are __HOME__, __USER__, __HOST__, __OS__, and __ARCH__.
// Unknown __*__ tokens are left literal.
func RenderString(input string) (string, error) {
	ctx, err := defaultContext()
	if err != nil {
		return "", err
	}
	return renderWithContext(input, ctx), nil
}

// RenderTemplate reads srcPath, renders placeholders, and writes the result to
// dstPath. It expands ~ and $HOME in dstPath. The destination file is created
// with the same permissions as the source, falling back to 0o644. If dstPath
// already exists with identical rendered content, it is left untouched. If it
// exists with different content and Force is false, ErrMismatch is returned.
// With Force true, the old target is backed up when Backup is true, then
// replaced. DryRun reports the intended change without writing.
func RenderTemplate(srcPath, dstPath string, host string, opts RenderOptions) error {
	srcPath = filepath.Clean(srcPath)

	expandedDst, err := expandPath(dstPath)
	if err != nil {
		return fmt.Errorf("expand destination %q: %w", dstPath, err)
	}
	dstPath = filepath.Clean(expandedDst)

	srcData, err := os.ReadFile(srcPath)
	if err != nil {
		return fmt.Errorf("read source %s: %w", srcPath, err)
	}

	ctx, err := defaultContext()
	if err != nil {
		return fmt.Errorf("build render context: %w", err)
	}
	if host != "" {
		ctx.host = host
	}
	rendered := renderWithContext(string(srcData), ctx)

	srcInfo, err := os.Stat(srcPath)
	if err != nil {
		return fmt.Errorf("stat source %s: %w", srcPath, err)
	}
	mode := srcInfo.Mode().Perm()
	if mode == 0 {
		mode = 0o644
	}

	dstInfo, err := os.Stat(dstPath)
	if err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("stat target %s: %w", dstPath, err)
		}
		if opts.DryRun {
			return nil
		}
		if err := ensureParentDir(dstPath); err != nil {
			return err
		}
		if err := atomicWrite(dstPath, []byte(rendered), mode); err != nil {
			return fmt.Errorf("write target %s: %w", dstPath, err)
		}
		return nil
	}

	if dstInfo.IsDir() {
		return fmt.Errorf("target %s is a directory", dstPath)
	}

	existing, err := os.ReadFile(dstPath)
	if err != nil {
		return fmt.Errorf("read target %s: %w", dstPath, err)
	}
	if string(existing) == rendered {
		return nil
	}

	if !opts.Force {
		return ErrMismatch
	}

	if opts.DryRun {
		return nil
	}

	if opts.Backup {
		if err := backupExisting(dstPath); err != nil {
			return err
		}
	} else {
		if err := os.Remove(dstPath); err != nil {
			return fmt.Errorf("remove target %s: %w", dstPath, err)
		}
	}

	if err := atomicWrite(dstPath, []byte(rendered), mode); err != nil {
		return fmt.Errorf("write target %s: %w", dstPath, err)
	}
	return nil
}

type templateContext struct {
	home string
	user string
	host string
	os   string
	arch string
}

func defaultContext() (templateContext, error) {
	var ctx templateContext

	home, err := os.UserHomeDir()
	if err != nil {
		return ctx, fmt.Errorf("resolve __HOME__: %w", err)
	}
	ctx.home = home

	u, err := currentUser()
	if err != nil {
		return ctx, fmt.Errorf("resolve __USER__: %w", err)
	}
	ctx.user = u

	host, err := os.Hostname()
	if err != nil {
		return ctx, fmt.Errorf("resolve __HOST__: %w", err)
	}
	if i := strings.Index(host, "."); i >= 0 {
		host = host[:i]
	}
	ctx.host = host

	ctx.os = runtime.GOOS
	ctx.arch = runtime.GOARCH

	return ctx, nil
}

func renderWithContext(input string, ctx templateContext) string {
	out := input
	out = strings.ReplaceAll(out, "__HOME__", ctx.home)
	out = strings.ReplaceAll(out, "__USER__", ctx.user)
	out = strings.ReplaceAll(out, "__HOST__", ctx.host)
	out = strings.ReplaceAll(out, "__OS__", ctx.os)
	out = strings.ReplaceAll(out, "__ARCH__", ctx.arch)
	return out
}

func atomicWrite(path string, data []byte, mode os.FileMode) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, mode); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func currentUser() (string, error) {
	u, err := user.Current()
	if err != nil {
		return "", err
	}
	if u.Username != "" {
		return u.Username, nil
	}
	return u.Name, nil
}
