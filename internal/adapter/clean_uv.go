package adapter

import (
	"os"
	"os/exec"
	"strings"
)

// uvCacheLockTimeout is how long a genv-spawned `uv cache clean` waits for
// another uv to release the cache lock. uv's own default is 300s, which made
// `genv clean` look hung on a machine where uv was already running, then fail
// the whole clean when the wait expired.
const uvCacheLockTimeout = "1"

// ConfigureCacheClean adjusts a cache-clean command before it runs.
// A `uv cache clean` inherits UV_LOCK_TIMEOUT when the caller set one.
// Otherwise the wait is capped at one second.
func ConfigureCacheClean(cmd *exec.Cmd, argv []string) {
	if !isUvCacheClean(argv) {
		return
	}
	if cmd.Env == nil {
		cmd.Env = os.Environ()
	}
	filtered := make([]string, 0, len(cmd.Env)+1)
	for _, entry := range cmd.Env {
		name, value, ok := strings.Cut(entry, "=")
		if ok && name == "UV_LOCK_TIMEOUT" {
			if value != "" {
				return
			}
			continue
		}
		filtered = append(filtered, entry)
	}
	cmd.Env = append(filtered, "UV_LOCK_TIMEOUT="+uvCacheLockTimeout)
}

// CacheCleanWarning reports a cache-clean failure that is not a broken
// manager. The string is the notice callers should print. uv's cache lock
// is left alone (`--force` would delete a cache a live uv is writing).
// pip exits 1 with "No matching packages" when `cache purge` finds nothing,
// which is an empty cache, not a failed purge.
func CacheCleanWarning(argv []string, output string) (string, bool) {
	text := strings.ToLower(output)
	if isUvCacheClean(argv) && (strings.Contains(text, "waiting for lock") || strings.Contains(text, "currently in-use")) {
		return "cache is in use; skipped", true
	}
	if isPipCachePurge(argv) && strings.Contains(text, "no matching packages") {
		return "cache already empty", true
	}
	return "", false
}

func isPipCachePurge(argv []string) bool {
	// python3 -m pip cache purge
	if len(argv) < 5 || argv[1] != "-m" || argv[2] != "pip" || argv[3] != "cache" || argv[4] != "purge" {
		return false
	}
	return true
}

func isUvCacheClean(argv []string) bool {
	return len(argv) >= 3 && argv[0] == "uv" && argv[1] == "cache" && argv[2] == "clean"
}
