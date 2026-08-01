package complete

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ks1686/genv/internal/genvfile"
)

const CacheTTL = 14 * 24 * time.Hour

// CacheDir returns the directory for completion name dumps.
func CacheDir() (string, error) {
	base, err := genvfile.DefaultDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "cache", "completions"), nil
}

// DumpFilename returns the cache file name for a manager dump.
func DumpFilename(manager string) string {
	return manager + ".txt"
}

func validateManager(manager string) error {
	if manager == "" {
		return errors.New("empty manager name")
	}
	if strings.Contains(manager, "/") || strings.Contains(manager, "..") {
		return fmt.Errorf("invalid manager name %q", manager)
	}
	if filepath.Base(manager) != manager {
		return fmt.Errorf("invalid manager name %q", manager)
	}
	return nil
}

func dumpPath(manager string) (string, error) {
	if err := validateManager(manager); err != nil {
		return "", err
	}
	dir, err := CacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, DumpFilename(manager)), nil
}

// ReadDump returns cached package names for manager when a fresh dump exists.
// Missing, expired, unreadable, or invalid manager names are cache misses.
func ReadDump(manager string) ([]string, bool) {
	path, err := dumpPath(manager)
	if err != nil {
		return nil, false
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, false
	}
	if time.Since(info.ModTime()) > CacheTTL {
		return nil, false
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, false
	}
	defer f.Close()

	var names []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if line != "" {
			names = append(names, line)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, false
	}
	return names, true
}

// WriteDump stores package names for manager as one name per line.
func WriteDump(manager string, names []string) error {
	path, err := dumpPath(manager)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create cache dir: %w", err)
	}
	content := strings.Join(names, "\n")
	if len(names) > 0 {
		content += "\n"
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		return fmt.Errorf("write dump: %w", err)
	}
	return nil
}
