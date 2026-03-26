package adapter

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
)

var (
	wslOnce sync.Once
	wslMemo bool
)

// isWSL reports whether the current process is running inside a WSL (Windows
// Subsystem for Linux) environment. It checks /proc/version for the
// "microsoft" string that the WSL kernel injects into that file.
// Returns false on any non-Linux host or when the file cannot be read.
func isWSL() bool {
	wslOnce.Do(func() {
		data, err := os.ReadFile("/proc/version")
		if err == nil {
			wslMemo = containsFold(string(data), "microsoft")
		}
	})
	return wslMemo
}

// sanitizePathForWSL removes Windows-host path entries from a PATH string.
// On WSL2, Windows drives are mounted under /mnt/<letter>/ and the Windows
// PATH is appended to the Linux PATH by default. This means Windows binaries
// (e.g. a stray brew.exe) can shadow Linux-native package managers.
//
// sanitizePathForWSL drops any path component that starts with /mnt/ followed
// by a single lowercase letter and a separator (e.g. /mnt/c/, /mnt/d/).
// All other entries are preserved unchanged.
func sanitizePathForWSL(pathEnv string) string {
	entries := filepath.SplitList(pathEnv)
	filtered := entries[:0]
	for _, e := range entries {
		if isWindowsMountPath(e) {
			continue
		}
		filtered = append(filtered, e)
	}
	return strings.Join(filtered, string(filepath.ListSeparator))
}

// isWindowsMountPath reports whether p looks like a WSL Windows-drive mount
// point: /mnt/<single-lowercase-letter> optionally followed by a slash.
func isWindowsMountPath(p string) bool {
	// Minimum: /mnt/c  (6 chars)
	if len(p) < 6 {
		return false
	}
	if !strings.HasPrefix(p, "/mnt/") {
		return false
	}
	rest := p[5:] // everything after /mnt/
	if len(rest) == 0 {
		return false
	}
	driveLetter := rest[0]
	if driveLetter < 'a' || driveLetter > 'z' {
		return false
	}
	// Accept exactly /mnt/c or /mnt/c/...
	return len(rest) == 1 || rest[1] == '/'
}
