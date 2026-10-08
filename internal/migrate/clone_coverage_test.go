package migrate

import (
	"testing"

	"github.com/ks1686/genv/internal/schema"
)

func TestCloneGenvFileIsDeepCopy(t *testing.T) {
	env := schema.EnvVar{Value: "one"}
	in := &schema.GenvFile{
		SchemaVersion: schema.Version8,
		Targets: map[string]*schema.TargetBundle{
			"arch": {
				Packages: []schema.Package{{ID: "git"}},
				Env:      map[string]*schema.EnvVar{"X": &env},
			},
		},
	}
	out, err := cloneGenvFile(in)
	if err != nil {
		t.Fatal(err)
	}
	out.Targets["arch"].Packages[0].ID = "curl"
	outEnv := schema.EnvVar{Value: "two"}
	out.Targets["arch"].Env["X"] = &outEnv
	if in.Targets["arch"].Packages[0].ID != "git" || in.Targets["arch"].Env["X"].Value != "one" {
		t.Fatalf("input mutated: %+v", in)
	}
}
