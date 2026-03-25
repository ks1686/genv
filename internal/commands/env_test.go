package commands

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/ks1686/genv/internal/schema"
)

// ─── EnvSet ──────────────────────────────────────────────────────────────────

func TestEnvSet_New(t *testing.T) {
	f := &schema.GenvFile{SchemaVersion: schema.Version, Packages: []schema.Package{}}
	if err := EnvSet(f, "MY_VAR", "hello", false); err != nil {
		t.Fatalf("EnvSet: %v", err)
	}
	if f.SchemaVersion != schema.Version2 {
		t.Errorf("expected schemaVersion %q, got %q", schema.Version2, f.SchemaVersion)
	}
	ev, ok := f.Env["MY_VAR"]
	if !ok {
		t.Fatal("expected MY_VAR in env map")
	}
	if ev.Value != "hello" {
		t.Errorf("expected value %q, got %q", "hello", ev.Value)
	}
	if ev.Sensitive {
		t.Error("expected Sensitive=false")
	}
}

func TestEnvSet_Sensitive(t *testing.T) {
	f := &schema.GenvFile{SchemaVersion: schema.Version, Packages: []schema.Package{}}
	if err := EnvSet(f, "SECRET", "s3cr3t", true); err != nil {
		t.Fatalf("EnvSet: %v", err)
	}
	if !f.Env["SECRET"].Sensitive {
		t.Error("expected Sensitive=true")
	}
}

func TestEnvSet_Update(t *testing.T) {
	f := &schema.GenvFile{
		SchemaVersion: schema.Version2,
		Packages:      []schema.Package{},
		Env:           map[string]schema.EnvVar{"X": {Value: "old"}},
	}
	if err := EnvSet(f, "X", "new", false); err != nil {
		t.Fatalf("EnvSet update: %v", err)
	}
	if f.Env["X"].Value != "new" {
		t.Errorf("expected updated value %q, got %q", "new", f.Env["X"].Value)
	}
}

func TestEnvSet_InvalidName(t *testing.T) {
	f := &schema.GenvFile{SchemaVersion: schema.Version, Packages: []schema.Package{}}
	cases := []string{"1invalid", "has space", "has-dash", ""}
	for _, name := range cases {
		if err := EnvSet(f, name, "val", false); err == nil {
			t.Errorf("EnvSet(%q): expected error, got nil", name)
		}
	}
}

func TestEnvSet_ValidNames(t *testing.T) {
	f := &schema.GenvFile{SchemaVersion: schema.Version, Packages: []schema.Package{}}
	cases := []string{"FOO", "_BAR", "baz_123", "X", "_"}
	for _, name := range cases {
		f.Env = nil
		if err := EnvSet(f, name, "v", false); err != nil {
			t.Errorf("EnvSet(%q): unexpected error: %v", name, err)
		}
	}
}

// ─── EnvUnset ────────────────────────────────────────────────────────────────

func TestEnvUnset_OK(t *testing.T) {
	f := &schema.GenvFile{
		SchemaVersion: schema.Version2,
		Packages:      []schema.Package{},
		Env:           map[string]schema.EnvVar{"FOO": {Value: "bar"}},
	}
	if err := EnvUnset(f, "FOO"); err != nil {
		t.Fatalf("EnvUnset: %v", err)
	}
	if _, ok := f.Env["FOO"]; ok {
		t.Error("expected FOO to be removed")
	}
}

func TestEnvUnset_NotFound(t *testing.T) {
	f := &schema.GenvFile{SchemaVersion: schema.Version2, Packages: []schema.Package{}}
	err := EnvUnset(f, "MISSING")
	if err == nil {
		t.Fatal("expected error for missing var")
	}
	if !errors.Is(err, ErrEnvNotFound) {
		t.Errorf("expected ErrEnvNotFound, got %v", err)
	}
}

// ─── EnvList ─────────────────────────────────────────────────────────────────

func TestEnvList_Empty(t *testing.T) {
	f := &schema.GenvFile{SchemaVersion: schema.Version, Packages: []schema.Package{}}
	var buf bytes.Buffer
	EnvList(f, &buf)
	if !strings.Contains(buf.String(), "no env variables") {
		t.Errorf("expected 'no env variables' message, got: %q", buf.String())
	}
}

func TestEnvList_ShowsVars(t *testing.T) {
	f := &schema.GenvFile{
		SchemaVersion: schema.Version2,
		Packages:      []schema.Package{},
		Env: map[string]schema.EnvVar{
			"FOO":    {Value: "bar"},
			"SECRET": {Value: "s3cr3t", Sensitive: true},
		},
	}
	var buf bytes.Buffer
	EnvList(f, &buf)
	out := buf.String()
	if !strings.Contains(out, "FOO") {
		t.Error("expected FOO in output")
	}
	if !strings.Contains(out, "bar") {
		t.Error("expected value 'bar' in output")
	}
	if !strings.Contains(out, "SECRET") {
		t.Error("expected SECRET in output")
	}
	// Sensitive value must be redacted.
	if strings.Contains(out, "s3cr3t") {
		t.Error("sensitive value must not appear in output")
	}
	if !strings.Contains(out, "[redacted]") {
		t.Error("expected [redacted] for sensitive value")
	}
}
