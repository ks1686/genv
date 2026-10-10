package resolver

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	"github.com/ks1686/genv/internal/testutil"
)

func TestRunCommand_UvCacheLockIsNotAnApplyError(t *testing.T) {
	testutil.InstallFakeBinary(t, "uv", `
echo "error: Timeout (1s) when waiting for lock on /tmp/uv" >&2
exit 2
`)
	var stderr bytes.Buffer
	err := RunCommand(context.Background(), []string{"uv", "cache", "clean"}, nil, io.Discard, &stderr)
	if err != nil {
		t.Fatalf("locked uv cache = %v, want nil; stderr=%s", err, stderr.String())
	}
	if !strings.Contains(stderr.String(), "cache is in use") {
		t.Fatalf("stderr = %q, want a skip notice", stderr.String())
	}
}

func TestRunCommand_UvCacheCleanFailureIsReturned(t *testing.T) {
	testutil.InstallFakeBinary(t, "uv", `
echo "error: permission denied" >&2
exit 2
`)
	err := RunCommand(context.Background(), []string{"uv", "cache", "clean"}, nil, io.Discard, io.Discard)
	if err == nil {
		t.Fatal("permission denied cache clean returned nil")
	}
}
