package adapter

import (
	"os/exec"
	"strings"
	"testing"
)

func TestConfigureCacheClean_CapsUnsetUvTimeout(t *testing.T) {
	t.Setenv("UV_LOCK_TIMEOUT", "")
	cmd := exec.Command("uv", "cache", "clean")
	ConfigureCacheClean(cmd, []string{"uv", "cache", "clean"})
	got := envValue(cmd.Env, "UV_LOCK_TIMEOUT")
	if got != "1" {
		t.Fatalf("UV_LOCK_TIMEOUT = %q, want 1", got)
	}
}

func TestConfigureCacheClean_HonorsCallerTimeout(t *testing.T) {
	t.Setenv("UV_LOCK_TIMEOUT", "15")
	cmd := exec.Command("uv", "cache", "clean")
	ConfigureCacheClean(cmd, []string{"uv", "cache", "clean"})
	if got := envValue(cmd.Env, "UV_LOCK_TIMEOUT"); got != "15" {
		t.Fatalf("UV_LOCK_TIMEOUT = %q, want the caller's 15", got)
	}
}

func TestCacheCleanWarning(t *testing.T) {
	uv := []string{"uv", "cache", "clean"}
	if msg, ok := CacheCleanWarning(uv, "error: Timeout (1s) when waiting for lock on /tmp/uv"); !ok || msg != "cache is in use; skipped" {
		t.Fatalf("lock timeout = %q %v", msg, ok)
	}
	if _, ok := CacheCleanWarning(uv, "error: permission denied"); ok {
		t.Fatal("permission denied is a real uv failure")
	}
	pip := []string{"python3", "-m", "pip", "cache", "purge"}
	if msg, ok := CacheCleanWarning(pip, "ERROR: No matching packages"); !ok || msg != "cache already empty" {
		t.Fatalf("empty pip cache = %q %v", msg, ok)
	}
	if _, ok := CacheCleanWarning(pip, "ERROR: No module named pip"); ok {
		t.Fatal("a missing pip module is a real failure")
	}
	if _, ok := CacheCleanWarning([]string{"brew", "cleanup"}, "waiting for lock"); ok {
		t.Fatal("brew cleanup is not classified as a warning")
	}
}

func envValue(env []string, key string) string {
	prefix := key + "="
	for _, entry := range env {
		if strings.HasPrefix(entry, prefix) {
			return strings.TrimPrefix(entry, prefix)
		}
	}
	return ""
}
