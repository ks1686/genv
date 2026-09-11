package external

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path"
	"strconv"
	"strings"

	"github.com/klauspost/compress/zstd"
	"github.com/ks1686/genv/internal/genvfile"
	"github.com/ks1686/genv/internal/schema"
	"github.com/ulikunitz/xz"
)

const maxArchiveFileBytes int64 = 512 << 20

type stagedArchiveFile struct {
	path string
	spec schema.ExternalInstallFile
}

func installArchive(archivePath, assetName string, install schema.ExternalInstall, policy installPolicy) ([]genvfile.ExternalPathReceipt, func(), func() error, error) {
	staging, err := os.MkdirTemp("", "genv-archive-*")
	if err != nil {
		return nil, nil, nil, err
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.RemoveAll(staging)
		}
	}()
	wanted, err := wantedArchiveFiles(install.Files)
	if err != nil {
		return nil, nil, nil, err
	}
	var staged []stagedArchiveFile
	switch {
	case strings.HasSuffix(strings.ToLower(assetName), ".zip"):
		staged, err = stageZip(archivePath, staging, install.StripComponents, wanted)
	case strings.HasSuffix(strings.ToLower(assetName), ".tar.gz"), strings.HasSuffix(strings.ToLower(assetName), ".tgz"):
		staged, err = stageTar(archivePath, staging, install.StripComponents, wanted, "gzip")
	case strings.HasSuffix(strings.ToLower(assetName), ".tar.xz"), strings.HasSuffix(strings.ToLower(assetName), ".txz"):
		staged, err = stageTar(archivePath, staging, install.StripComponents, wanted, "xz")
	case strings.HasSuffix(strings.ToLower(assetName), ".tar.zst"), strings.HasSuffix(strings.ToLower(assetName), ".tzst"):
		staged, err = stageTar(archivePath, staging, install.StripComponents, wanted, "zstd")
	case strings.HasSuffix(strings.ToLower(assetName), ".tar"):
		staged, err = stageTar(archivePath, staging, install.StripComponents, wanted, "")
	default:
		return nil, nil, nil, fmt.Errorf("unsupported archive format for %q", assetName)
	}
	if err != nil {
		return nil, nil, nil, err
	}
	if len(staged) != len(wanted) {
		return nil, nil, nil, fmt.Errorf("archive is missing one or more declared files")
	}

	type transaction struct {
		restore func()
		finish  func() error
	}
	var transactions []transaction
	rollback := func() {
		for i := len(transactions) - 1; i >= 0; i-- {
			transactions[i].restore()
		}
		_ = os.RemoveAll(staging)
	}
	var receipts []genvfile.ExternalPathReceipt
	for _, item := range staged {
		destination, err := expandDestination(item.spec.To)
		if err != nil {
			rollback()
			return nil, nil, nil, err
		}
		restore, finish, err := installDirectWithMode(item.path, destination, archiveMode(item.spec.Mode), policy)
		if err != nil {
			rollback()
			return nil, nil, nil, wrapSystemScopeError(policy.Scope, destination, err)
		}
		transactions = append(transactions, transaction{restore: restore, finish: finish})
		digest, err := fileSHA256(destination)
		if err != nil {
			rollback()
			return nil, nil, nil, err
		}
		receipts = append(receipts, genvfile.ExternalPathReceipt{Path: destination, SHA256: digest})
	}
	cleanup = false
	finish := func() error {
		for _, tx := range transactions {
			if err := tx.finish(); err != nil {
				return err
			}
		}
		return os.RemoveAll(staging)
	}
	return receipts, rollback, finish, nil
}

func wantedArchiveFiles(files []schema.ExternalInstallFile) (map[string]schema.ExternalInstallFile, error) {
	wanted := make(map[string]schema.ExternalInstallFile, len(files))
	for _, file := range files {
		name, err := safeArchiveName(file.From)
		if err != nil {
			return nil, err
		}
		if _, exists := wanted[name]; exists {
			return nil, fmt.Errorf("duplicate archive source %q", name)
		}
		wanted[name] = file
	}
	return wanted, nil
}

func stageZip(archivePath, staging string, strip int, wanted map[string]schema.ExternalInstallFile) ([]stagedArchiveFile, error) {
	zr, err := zip.OpenReader(archivePath)
	if err != nil {
		return nil, err
	}
	defer zr.Close()
	var out []stagedArchiveFile
	seen := make(map[string]bool)
	for _, entry := range zr.File {
		name, err := strippedArchiveName(entry.Name, strip)
		if err != nil {
			return nil, err
		}
		if entry.Mode()&os.ModeSymlink != 0 || !entry.Mode().IsRegular() && !entry.FileInfo().IsDir() {
			return nil, fmt.Errorf("archive entry %q is not a regular file or directory", entry.Name)
		}
		spec, selected := wanted[name]
		if !selected || entry.FileInfo().IsDir() {
			continue
		}
		if seen[name] {
			return nil, fmt.Errorf("archive contains duplicate entry %q", name)
		}
		seen[name] = true
		r, err := entry.Open()
		if err != nil {
			return nil, err
		}
		staged, err := stageReader(staging, r)
		_ = r.Close()
		if err != nil {
			return nil, err
		}
		out = append(out, stagedArchiveFile{path: staged, spec: spec})
	}
	return out, nil
}

func stageTar(archivePath, staging string, strip int, wanted map[string]schema.ExternalInstallFile, compression string) ([]stagedArchiveFile, error) {
	f, err := os.Open(archivePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var reader io.Reader = f
	switch compression {
	case "gzip":
		gz, err := gzip.NewReader(f)
		if err != nil {
			return nil, err
		}
		defer gz.Close()
		reader = gz
	case "xz":
		xzReader, err := xz.NewReader(f)
		if err != nil {
			return nil, err
		}
		reader = xzReader
	case "zstd":
		zstdReader, err := zstd.NewReader(f)
		if err != nil {
			return nil, err
		}
		defer zstdReader.Close()
		reader = zstdReader
	}
	tr := tar.NewReader(reader)
	var out []stagedArchiveFile
	seen := make(map[string]bool)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		name, err := strippedArchiveName(header.Name, strip)
		if err != nil {
			return nil, err
		}
		if header.Typeflag != tar.TypeReg && header.Typeflag != tar.TypeRegA && header.Typeflag != tar.TypeDir {
			return nil, fmt.Errorf("archive entry %q has unsafe type", header.Name)
		}
		spec, selected := wanted[name]
		if !selected || header.Typeflag == tar.TypeDir {
			continue
		}
		if seen[name] {
			return nil, fmt.Errorf("archive contains duplicate entry %q", name)
		}
		seen[name] = true
		staged, err := stageReader(staging, tr)
		if err != nil {
			return nil, err
		}
		out = append(out, stagedArchiveFile{path: staged, spec: spec})
	}
	return out, nil
}

func stageReader(staging string, reader io.Reader) (string, error) {
	f, err := os.CreateTemp(staging, "file-*")
	if err != nil {
		return "", err
	}
	name := f.Name()
	written, copyErr := io.Copy(f, io.LimitReader(reader, maxArchiveFileBytes+1))
	closeErr := f.Close()
	if copyErr != nil {
		return "", copyErr
	}
	if closeErr != nil {
		return "", closeErr
	}
	if written > maxArchiveFileBytes {
		return "", fmt.Errorf("archive file exceeds %d bytes", maxArchiveFileBytes)
	}
	return name, nil
}

func strippedArchiveName(name string, strip int) (string, error) {
	clean, err := safeArchiveName(name)
	if err != nil {
		return "", err
	}
	parts := strings.Split(clean, "/")
	if strip >= len(parts) {
		return "", nil
	}
	return strings.Join(parts[strip:], "/"), nil
}

func safeArchiveName(name string) (string, error) {
	if name == "" || strings.Contains(name, "\\") || strings.HasPrefix(name, "/") {
		return "", fmt.Errorf("unsafe archive path %q", name)
	}
	clean := path.Clean(name)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("unsafe archive path %q", name)
	}
	return clean, nil
}

func archiveMode(value string) os.FileMode {
	if value == "" {
		return 0o755
	}
	parsed, err := strconv.ParseUint(value, 8, 32)
	if err != nil {
		return 0o755
	}
	return os.FileMode(parsed)
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
