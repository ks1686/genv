package env

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ks1686/genv/internal/genvfile"
	"github.com/ks1686/genv/internal/schema"
)

// ─── shellQuote/shellUnquote roundtrip ───────────────────────────────────────

func TestShellQuoteRoundtrip(t *testing.T) {
	cases := []string{
		"simple",
		"with spaces",
		"has'single'quote",
		`has"double"quote`,
		"dollar$sign",
		"back`tick`here",
		"",
		"it's a test",
	}
	for _, v := range cases {
		quoted := shellQuote(v)
		got := shellUnquote(quoted)
		if got != v {
			t.Errorf("roundtrip(%q): got %q, want %q (quoted: %q)", v, got, v, quoted)
		}
		// Quoted form must start and end with single quote.
		if !strings.HasPrefix(quoted, "'") || !strings.HasSuffix(quoted, "'") {
			t.Errorf("shellQuote(%q) = %q: not single-quoted", v, quoted)
		}
	}
}

// ─── WriteFragment / ReadFragment ───────────────────────────────────────────

func TestWriteFragment_Basic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "env.sh")

	vars := map[string]schema.EnvVar{
		"FOO": {Value: "bar"},
		"BAZ": {Value: "hello world"},
	}
	if err := WriteFragment(path, vars); err != nil {
		t.Fatalf("WriteFragment: %v", err)
	}

	got, err := ReadFragment(path)
	if err != nil {
		t.Fatalf("ReadFragment: %v", err)
	}

	want := map[string]string{
		"FOO": "bar",
		"BAZ": "hello world",
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("var %s: got %q, want %q", k, got[k], v)
		}
	}
}

func TestWriteFragment_SpecialChars(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "env.sh")

	vars := map[string]schema.EnvVar{
		"TRICKY": {Value: "it's a $test with `backticks`"},
	}
	if err := WriteFragment(path, vars); err != nil {
		t.Fatalf("WriteFragment: %v", err)
	}

	got, err := ReadFragment(path)
	if err != nil {
		t.Fatalf("ReadFragment: %v", err)
	}
	if got["TRICKY"] != vars["TRICKY"].Value {
		t.Errorf("got %q, want %q", got["TRICKY"], vars["TRICKY"].Value)
	}
}

func TestWriteFragment_Empty_RemovesFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "env.sh")

	// Create fragment first.
	if err := WriteFragment(path, map[string]schema.EnvVar{"X": {Value: "1"}}); err != nil {
		t.Fatalf("WriteFragment: %v", err)
	}
	// Write empty — should remove.
	if err := WriteFragment(path, nil); err != nil {
		t.Fatalf("WriteFragment(empty): %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("expected fragment to be removed")
	}
}

func TestReadFragment_NonExistent(t *testing.T) {
	dir := t.TempDir()
	got, err := ReadFragment(filepath.Join(dir, "missing.sh"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty map, got %v", got)
	}
}

func TestWriteFragment_Deterministic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "env.sh")

	vars := map[string]schema.EnvVar{
		"ZZZ": {Value: "last"},
		"AAA": {Value: "first"},
		"MMM": {Value: "middle"},
	}
	if err := WriteFragment(path, vars); err != nil {
		t.Fatalf("WriteFragment: %v", err)
	}
	data, _ := os.ReadFile(path)
	content := string(data)

	// AAA must appear before MMM, MMM before ZZZ (sorted output).
	iAAA := strings.Index(content, "export AAA=")
	iMMM := strings.Index(content, "export MMM=")
	iZZZ := strings.Index(content, "export ZZZ=")
	if !(iAAA < iMMM && iMMM < iZZZ) {
		t.Errorf("fragment not sorted: AAA@%d MMM@%d ZZZ@%d\n%s", iAAA, iMMM, iZZZ, content)
	}
}

// ─── InjectSourceLine ────────────────────────────────────────────────────────

func TestInjectSourceLine_AddsOnce(t *testing.T) {
	dir := t.TempDir()
	rc := filepath.Join(dir, ".bashrc")
	frag := filepath.Join(dir, "env.sh")

	// First injection.
	if err := InjectSourceLine(rc, frag); err != nil {
		t.Fatalf("InjectSourceLine: %v", err)
	}
	data, _ := os.ReadFile(rc)
	if !strings.Contains(string(data), frag) {
		t.Error("expected fragment path in rc file")
	}

	// Second injection — should not duplicate.
	if err := InjectSourceLine(rc, frag); err != nil {
		t.Fatalf("InjectSourceLine (2nd): %v", err)
	}
	data2, _ := os.ReadFile(rc)
	count := strings.Count(string(data2), frag)
	if count != 1 {
		t.Errorf("expected fragment referenced once, found %d times", count)
	}
}

func TestInjectSourceLine_CreatesRcFile(t *testing.T) {
	dir := t.TempDir()
	rc := filepath.Join(dir, "nonexistent.rc")
	frag := "/some/path/env.sh"

	if err := InjectSourceLine(rc, frag); err != nil {
		t.Fatalf("InjectSourceLine: %v", err)
	}
	if _, err := os.Stat(rc); err != nil {
		t.Errorf("expected rc file to be created: %v", err)
	}
}

// ─── EnvStatus ───────────────────────────────────────────────────────────────

func TestEnvStatus_AllOK(t *testing.T) {
	spec := map[string]schema.EnvVar{
		"FOO": {Value: "bar"},
		"BAZ": {Value: "qux"},
	}
	lock := []genvfile.LockedEnvVar{
		{Name: "FOO", Value: "bar"},
		{Name: "BAZ", Value: "qux"},
	}
	entries := EnvStatus(spec, lock)
	for _, e := range entries {
		if e.Kind != EnvStatusOK {
			t.Errorf("%s: expected ok, got %s", e.Name, e.Kind)
		}
	}
}

func TestEnvStatus_Missing(t *testing.T) {
	spec := map[string]schema.EnvVar{
		"FOO": {Value: "bar"},
	}
	entries := EnvStatus(spec, nil)
	if len(entries) != 1 || entries[0].Kind != EnvStatusMissing {
		t.Errorf("expected 1 missing entry, got %v", entries)
	}
}

func TestEnvStatus_Extra(t *testing.T) {
	lock := []genvfile.LockedEnvVar{
		{Name: "ORPHAN", Value: "x"},
	}
	entries := EnvStatus(nil, lock)
	if len(entries) != 1 || entries[0].Kind != EnvStatusExtra {
		t.Errorf("expected 1 extra entry, got %v", entries)
	}
}

func TestEnvStatus_Modified(t *testing.T) {
	spec := map[string]schema.EnvVar{
		"FOO": {Value: "new"},
	}
	lock := []genvfile.LockedEnvVar{
		{Name: "FOO", Value: "old"},
	}
	entries := EnvStatus(spec, lock)
	if len(entries) != 1 || entries[0].Kind != EnvStatusModified {
		t.Errorf("expected 1 modified entry, got %v", entries)
	}
}

// ─── ApplyEnv ───────────────────────────────────────────────────────────────

func TestApplyEnv_Basic(t *testing.T) {
	dir := t.TempDir()
	frag := filepath.Join(dir, "env.sh")
	rc1 := filepath.Join(dir, ".bashrc")
	rc2 := filepath.Join(dir, ".zshrc")

	vars := map[string]schema.EnvVar{
		"FOO": {Value: "bar"},
	}

	if err := ApplyEnv(frag, vars, []string{rc1, rc2}); err != nil {
		t.Fatalf("ApplyEnv: %v", err)
	}

	// Verify fragment
	got, err := ReadFragment(frag)
	if err != nil {
		t.Fatalf("ReadFragment: %v", err)
	}
	if got["FOO"] != "bar" {
		t.Errorf("got %q, want %q", got["FOO"], "bar")
	}

	// Verify rc injection
	for _, rc := range []string{rc1, rc2} {
		data, err := os.ReadFile(rc)
		if err != nil {
			t.Fatalf("reading %s: %v", rc, err)
		}
		if !strings.Contains(string(data), frag) {
			t.Errorf("expected fragment path in %s", rc)
		}
	}
}

func TestApplyEnv_EmptyVars(t *testing.T) {
	dir := t.TempDir()
	frag := filepath.Join(dir, "env.sh")
	rc := filepath.Join(dir, ".bashrc")

	// Pre-create fragment
	if err := WriteFragment(frag, map[string]schema.EnvVar{"X": {Value: "1"}}); err != nil {
		t.Fatalf("WriteFragment: %v", err)
	}

	if err := ApplyEnv(frag, nil, []string{rc}); err != nil {
		t.Fatalf("ApplyEnv: %v", err)
	}

	// Fragment should be removed
	if _, err := os.Stat(frag); !os.IsNotExist(err) {
		t.Error("expected fragment to be removed")
	}

	// RC file should not be created/modified
	if _, err := os.Stat(rc); !os.IsNotExist(err) {
		t.Error("expected rc file to not be created")
	}
}

func TestApplyEnv_WriteError(t *testing.T) {
	dir := t.TempDir()
	// Create a directory where the fragment should be, causing a write error
	frag := filepath.Join(dir, "env.sh")
	if err := os.MkdirAll(frag, 0700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	vars := map[string]schema.EnvVar{
		"FOO": {Value: "bar"},
	}

	err := ApplyEnv(frag, vars, nil)
	if err == nil {
		t.Error("expected error from WriteFragment, got nil")
	}
}

func TestApplyEnv_InjectErrorNonFatal(t *testing.T) {
	dir := t.TempDir()
	frag := filepath.Join(dir, "env.sh")
	rc := filepath.Join(dir, ".bashrc")

	// Create a directory where the rc file should be, causing an inject error
	if err := os.MkdirAll(rc, 0700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	vars := map[string]schema.EnvVar{
		"FOO": {Value: "bar"},
	}

	// Should not return an error despite inject failure
	if err := ApplyEnv(frag, vars, []string{rc}); err != nil {
		t.Fatalf("ApplyEnv: %v", err)
	}

	// Check fragment was still written
	got, err := ReadFragment(frag)
	if err != nil {
		t.Fatalf("ReadFragment: %v", err)
	}
	if got["FOO"] != "bar" {
		t.Errorf("got %q, want %q", got["FOO"], "bar")
	}
}
