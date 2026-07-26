package hooks

import (
	"bytes"
	"context"
	"io"
	"runtime"
	"testing"

	"github.com/ks1686/genv/internal/schema"
)

func TestNewExecutorDefaultsNilWritersAndRunsHook(t *testing.T) {
	e := NewExecutor(nil, nil)
	if e.Stdout != io.Discard || e.Stderr != io.Discard {
		t.Fatal("NewExecutor nil writers were not replaced with io.Discard")
	}
	if e.goos != runtime.GOOS {
		t.Errorf("NewExecutor goos = %q, want %q", e.goos, runtime.GOOS)
	}
	if err := e.PostApply(context.Background(), []schema.Hook{{Command: "true"}}, "any", false); err != nil {
		t.Fatalf("PostApply with exec runner: %v", err)
	}
}

func TestNewExecutorPreservesSuppliedWriters(t *testing.T) {
	var stdout, stderr bytes.Buffer
	e := NewExecutor(&stdout, &stderr)
	if e.Stdout != &stdout || e.Stderr != &stderr {
		t.Fatal("NewExecutor did not preserve supplied writers")
	}
}
