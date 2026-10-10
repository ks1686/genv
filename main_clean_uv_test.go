package main

import (
	"strings"
	"testing"

	"github.com/ks1686/genv/internal/adapter"
	"github.com/ks1686/genv/internal/testutil"
)

func TestCleanCmd_UvCacheInUseIsSkipped(t *testing.T) {
	original := adapter.All
	adapter.All = []adapter.Adapter{adapter.Uv{}}
	t.Cleanup(func() { adapter.All = original })
	testutil.InstallFakeBinary(t, "uv", `
echo "Cache is currently in-use, waiting for other uv processes to finish" >&2
echo "error: Timeout (1s) when waiting for lock on /tmp/uv" >&2
exit 2
`)

	var code int
	stderr := captureStderr(t, func() { code = run([]string{"clean"}) })
	if code != exitOK {
		t.Fatalf("clean = %d, want 0; stderr=%s", code, stderr)
	}
	if !strings.Contains(stderr, "cache is in use") {
		t.Fatalf("stderr = %q, want a skip notice", stderr)
	}
}

func TestCleanCmd_UvCacheCleanFailureStillFails(t *testing.T) {
	original := adapter.All
	adapter.All = []adapter.Adapter{adapter.Uv{}}
	t.Cleanup(func() { adapter.All = original })
	testutil.InstallFakeBinary(t, "uv", `
echo "error: permission denied" >&2
exit 2
`)

	var code int
	stderr := captureStderr(t, func() { code = run([]string{"clean"}) })
	if code != exitLogic {
		t.Fatalf("clean = %d, want %d; stderr=%s", code, exitLogic, stderr)
	}
}

func TestCleanCmd_EmptyPipCacheIsSuccess(t *testing.T) {
	original := adapter.All
	adapter.All = []adapter.Adapter{adapter.PipUser{}}
	t.Cleanup(func() { adapter.All = original })
	testutil.InstallFakeBinary(t, "python3", `
case "$*" in
  *--version*) echo "pip 24.0"; exit 0 ;;
  *"cache purge"*) echo "ERROR: No matching packages" >&2; exit 1 ;;
  *) echo "unexpected $*" >&2; exit 1 ;;
esac
`)

	var code int
	stderr := captureStderr(t, func() { code = run([]string{"clean"}) })
	if code != exitOK {
		t.Fatalf("clean = %d, want 0; stderr=%s", code, stderr)
	}
	if !strings.Contains(stderr, "cache already empty") {
		t.Fatalf("stderr = %q, want an empty-cache notice", stderr)
	}
}

func TestCleanCmd_UvCacheCleanUsesShortLockTimeout(t *testing.T) {
	original := adapter.All
	adapter.All = []adapter.Adapter{adapter.Uv{}}
	t.Cleanup(func() { adapter.All = original })
	t.Setenv("UV_LOCK_TIMEOUT", "")
	testutil.InstallFakeBinary(t, "uv", `
echo "timeout=$UV_LOCK_TIMEOUT"
exit 0
`)

	var code int
	stdout := captureStdout(t, func() { code = run([]string{"clean"}) })
	if code != exitOK {
		t.Fatalf("clean = %d, stdout=%s", code, stdout)
	}
	if !strings.Contains(stdout, "timeout=1") {
		t.Fatalf("stdout = %q, want UV_LOCK_TIMEOUT=1", stdout)
	}
}
