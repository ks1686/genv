package main

import (
	"path/filepath"
	"slices"
	"testing"
)

func TestHookEnv_includes_spec_and_lock_paths(t *testing.T) {
	spec := filepath.Join(t.TempDir(), "genv.json")
	lock := filepath.Join(t.TempDir(), "genv.lock.json")

	env := hookEnv(hookContext{
		Event:    "apply",
		Phase:    "post-apply",
		Host:     "ci",
		SpecFile: spec,
		SpecDir:  filepath.Dir(spec),
		LockFile: lock,
	})

	want := []string{
		"GENV_SPEC_FILE=" + spec,
		"GENV_SPEC_DIR=" + filepath.Dir(spec),
		"GENV_LOCK_FILE=" + lock,
	}
	for _, item := range want {
		if !slices.Contains(env, item) {
			t.Fatalf("hookEnv() = %v, missing %q", env, item)
		}
	}
	if !slices.Contains(env, "GENV_EVENT=apply") || !slices.Contains(env, "GENV_PHASE=post-apply") {
		t.Fatalf("hookEnv() dropped existing context vars: %v", env)
	}
}
