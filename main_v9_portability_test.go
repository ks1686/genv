package main

import (
	"testing"

	"github.com/ks1686/genv/internal/schema"
)

func TestResolveEffectiveSpecMaterializesVersion9Target(t *testing.T) {
	f := &schema.GenvFile{
		SchemaVersion: schema.Version9,
		Targets: map[string]*schema.TargetBundle{
			"linux": {Packages: []schema.Package{{ID: "tool", Prefer: "external"}}},
		},
	}

	effective, targetID, err := resolveEffectiveSpec(f, "linux", "linux")
	if err != nil {
		t.Fatalf("resolveEffectiveSpec() error: %v", err)
	}
	if targetID != "linux" || len(effective.Packages) != 1 || effective.Packages[0].ID != "tool" {
		t.Fatalf("target=%q effective=%+v", targetID, effective)
	}
}

func TestResolveMutationTargetUsesVersion9Target(t *testing.T) {
	f := &schema.GenvFile{SchemaVersion: schema.Version9, Targets: map[string]*schema.TargetBundle{"linux": {}}}
	targetID, comp, code := resolveMutationTarget("add", "genv.json", f, "linux")
	if code != exitOK || targetID != "linux" {
		t.Fatalf("resolveMutationTarget() = (%q, %d), want (linux, %d)", targetID, code, exitOK)
	}
	// v9 has no modules, so there is no composition to guard mutations with.
	if comp != nil {
		t.Errorf("resolveMutationTarget() composition = %+v, want nil for v9", comp)
	}
}
