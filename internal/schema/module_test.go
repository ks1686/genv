package schema

import (
	"strings"
	"testing"
)

func TestParseAndValidateModule_accepts_v10_document(t *testing.T) {
	doc := []byte(`{
	  "schemaVersion": "10",
	  "defaults": { "packages": [{ "id": "ripgrep", "prefer": "brew" }] },
	  "targets": { "macos": { "packages": [{ "id": "mas-cli", "prefer": "mas" }] } }
	}`)

	mod, errs, parseErr := ParseAndValidateModule(doc)
	if parseErr != nil {
		t.Fatalf("ParseAndValidateModule() parse error: %v", parseErr)
	}
	if len(errs) > 0 {
		t.Fatalf("ParseAndValidateModule() unexpected errors: %v", errs)
	}
	if mod.SchemaVersion != Version10 {
		t.Errorf("SchemaVersion = %q, want %q", mod.SchemaVersion, Version10)
	}
	if mod.Defaults == nil || len(mod.Defaults.Packages) != 1 || mod.Defaults.Packages[0].ID != "ripgrep" {
		t.Errorf("defaults.packages not decoded: %+v", mod.Defaults)
	}
	if mod.Targets["macos"] == nil || len(mod.Targets["macos"].Packages) != 1 {
		t.Errorf("targets.macos.packages not decoded: %+v", mod.Targets["macos"])
	}
}

func TestModule_v8_schemaVersion_rejected(t *testing.T) {
	doc := []byte(`{"schemaVersion":"9","defaults":{"packages":[{"id":"git"}]}}`)
	_, errs, parseErr := ParseAndValidateModule(doc)
	if parseErr != nil {
		t.Fatalf("parse error: %v", parseErr)
	}
	if !hasValidationFor(errs, "schemaVersion") {
		t.Fatalf("expected schemaVersion error, got %v", errs)
	}
}

func TestModule_rejects_nested_modules(t *testing.T) {
	cases := map[string]string{
		"nested modules registry": `{"schemaVersion":"10","defaults":{},"modules":{"x":"x.json"}}`,
		"nested useModules":       `{"schemaVersion":"10","defaults":{"useModules":["x"]}}`,
		"nested updates":          `{"schemaVersion":"10","defaults":{},"updates":{"enabled":true}}`,
		"nested adapters":         `{"schemaVersion":"10","defaults":{},"adapters":{}}`,
		"nested repo":             `{"schemaVersion":"10","defaults":{},"repo":{"url":"https://example.com/x"}}`,
	}
	for name, doc := range cases {
		t.Run(name, func(t *testing.T) {
			_, errs, parseErr := ParseAndValidateModule([]byte(doc))
			if parseErr != nil {
				t.Fatalf("parse error: %v", parseErr)
			}
			if len(errs) == 0 {
				t.Fatalf("expected validation error for %s", name)
			}
		})
	}
}

func TestModule_requires_defaults_or_targets(t *testing.T) {
	_, errs, parseErr := ParseAndValidateModule([]byte(`{"schemaVersion":"10"}`))
	if parseErr != nil {
		t.Fatalf("parse error: %v", parseErr)
	}
	if len(errs) == 0 {
		t.Fatal("expected error for module without defaults or targets")
	}
}

func TestModule_rejects_unknown_target(t *testing.T) {
	doc := []byte(`{"schemaVersion":"10","targets":{"plan9":{"packages":[{"id":"x"}]}}}`)
	_, errs, parseErr := ParseAndValidateModule(doc)
	if parseErr != nil {
		t.Fatalf("parse error: %v", parseErr)
	}
	if !hasValidationFor(errs, "targets.plan9") {
		t.Fatalf("expected unknown target error, got %v", errs)
	}
}

func TestModule_requiresModules_known_names_only(t *testing.T) {
	doc := []byte(`{"schemaVersion":"10","requiresModules":["Not-Kebab"],"defaults":{}}`)
	_, errs, _ := ParseAndValidateModule(doc)
	if !hasValidationFor(errs, "requiresModules[0]") {
		t.Fatalf("expected invalid module name error, got %v", errs)
	}
}

func TestModule_rejects_unknown_field(t *testing.T) {
	doc := []byte(`{"schemaVersion":"10","defaults":{},"modules":{"a":"a.json"}}`)
	_, errs, _ := ParseAndValidateModule(doc)
	if !hasValidationFor(errs, "modules") {
		t.Fatalf("expected unknown field error for modules, got %v", errs)
	}
}

func TestValidateModuleName(t *testing.T) {
	valid := []string{"proxy-service", "search-service", "a", "agent-harnesses", "x1"}
	for _, name := range valid {
		if !ValidateModuleName(name) {
			t.Errorf("ValidateModuleName(%q) = false, want true", name)
		}
	}
	invalid := []string{"", "Proxy", "proxy_service", "-proxy", "proxy-", "with space", "brew", strings.Repeat("a", 65)}
	for _, name := range invalid {
		if ValidateModuleName(name) {
			t.Errorf("ValidateModuleName(%q) = true, want false", name)
		}
	}
}

func TestValidateModulePath(t *testing.T) {
	valid := []string{
		"modules/proxy.json",
		"proxy.json",
		"a/b/c.json",
		"agent-modules/search-service.json",
	}
	for _, p := range valid {
		if !ValidateModulePath(p) {
			t.Errorf("ValidateModulePath(%q) = false, want true", p)
		}
	}
	invalid := []string{
		"",
		"/etc/passwd",
		"../outside.json",
		"modules/../../outside.json",
		"~/modules/proxy.json",
		"$HOME/modules/proxy.json",
		"modules/proxy.json/",
		".",
		"..",
	}
	for _, p := range invalid {
		if ValidateModulePath(p) {
			t.Errorf("ValidateModulePath(%q) = true, want false", p)
		}
	}
}

func TestPortable_v10_requires_useModules_version(t *testing.T) {
	doc := []byte(`{"schemaVersion":"9","targets":{"macos":{"useModules":["proxy"]}}}`)
	_, errs, parseErr := ParseAndValidate(doc)
	if parseErr != nil {
		t.Fatalf("parse error: %v", parseErr)
	}
	if !hasValidationFor(errs, "targets.macos.useModules") {
		t.Fatalf("expected useModules version error, got %v", errs)
	}
}

func TestPortable_v10_requires_modules_version(t *testing.T) {
	doc := []byte(`{"schemaVersion":"9","modules":{"proxy":"modules/proxy.json"},"targets":{"macos":{}}}`)
	_, errs, parseErr := ParseAndValidate(doc)
	if parseErr != nil {
		t.Fatalf("parse error: %v", parseErr)
	}
	if !hasValidationFor(errs, "modules") {
		t.Fatalf("expected modules version error, got %v", errs)
	}
}

func TestPortable_v10_root_accepts_modules_and_useModules(t *testing.T) {
	doc := []byte(`{
	  "schemaVersion": "10",
	  "modules": { "proxy-service": "modules/proxy-service.json" },
	  "defaults": { "useModules": ["proxy-service"] },
	  "targets": { "macos": { "packages": [{ "id": "git" }] } }
	}`)
	f, errs, parseErr := ParseAndValidate(doc)
	if parseErr != nil {
		t.Fatalf("parse error: %v", parseErr)
	}
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if f.Modules["proxy-service"] != "modules/proxy-service.json" {
		t.Errorf("Modules = %v", f.Modules)
	}
	if len(f.Defaults.UseModules) != 1 || f.Defaults.UseModules[0] != "proxy-service" {
		t.Errorf("Defaults.UseModules = %v", f.Defaults.UseModules)
	}
}

func TestPortable_v10_rejects_invalid_module_registration(t *testing.T) {
	cases := map[string]string{
		"unknown field":  `{"schemaVersion":"10","modules":{"proxy":"m.json","bad name":"m.json"},"targets":{"macos":{}}}`,
		"absolute path":  `{"schemaVersion":"10","modules":{"proxy":"/etc/m.json"},"targets":{"macos":{}}}`,
		"traversal path": `{"schemaVersion":"10","modules":{"proxy":"../m.json"},"targets":{"macos":{}}}`,
		"unknown module": `{"schemaVersion":"10","targets":{"macos":{"useModules":["ghost"]}}}`,
	}
	for name, doc := range cases {
		t.Run(name, func(t *testing.T) {
			_, errs, parseErr := ParseAndValidate([]byte(doc))
			if parseErr != nil {
				t.Fatalf("parse error: %v", parseErr)
			}
			if len(errs) == 0 {
				t.Fatalf("expected validation error for %s", name)
			}
		})
	}
}

func TestRootModules_preserved_by_marshal(t *testing.T) {
	f := &GenvFile{
		SchemaVersion: Version10,
		Modules:       map[string]string{"proxy-service": "modules/proxy-service.json"},
		Defaults:      &TargetBundle{UseModules: []string{"proxy-service"}},
		Targets:       map[string]*TargetBundle{"macos": {}},
	}
	data, err := f.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON() error: %v", err)
	}
	got := string(data)
	for _, want := range []string{`"modules"`, `modules/proxy-service.json`, `"useModules"`} {
		if !strings.Contains(got, want) {
			t.Errorf("marshaled output missing %s: %s", want, got)
		}
	}
	round, errs, parseErr := ParseAndValidate(data)
	if parseErr != nil || len(errs) > 0 {
		t.Fatalf("round trip failed: parseErr=%v errs=%v", parseErr, errs)
	}
	if round.Modules["proxy-service"] != "modules/proxy-service.json" {
		t.Errorf("round trip lost modules: %v", round.Modules)
	}
	if len(round.Defaults.UseModules) != 1 {
		t.Errorf("round trip lost useModules: %v", round.Defaults.UseModules)
	}
}

func TestAtLeastVersion_v10(t *testing.T) {
	if got := AtLeastVersion("9", Version10); got != Version10 {
		t.Errorf("AtLeastVersion(%q, %q) = %q, want %q", "9", Version10, got, Version10)
	}
	if got := AtLeastVersion(Version10, "9"); got != Version10 {
		t.Errorf("AtLeastVersion(%q, %q) = %q, want %q", Version10, "9", got, Version10)
	}
}

func TestIsPortableVersion_includes_v10(t *testing.T) {
	for _, v := range []string{Version8, Version9, Version10} {
		if !IsPortableVersion(v) {
			t.Errorf("IsPortableVersion(%q) = false, want true", v)
		}
	}
	for _, v := range []string{Version, Version7, "", "99"} {
		if IsPortableVersion(v) {
			t.Errorf("IsPortableVersion(%q) = true, want false", v)
		}
	}
}

func hasValidationFor(errs []ValidationError, field string) bool {
	for _, e := range errs {
		if e.Field == field {
			return true
		}
	}
	return false
}

func TestPortable_v10_keeps_v9_external_recipe_surface(t *testing.T) {
	doc := []byte(`{
	  "schemaVersion": "10",
	  "targets": { "macos": { "packages": [{
	    "id": "example",
	    "prefer": "external",
	    "external": {
	      "detect": { "command": ["example", "--version"], "versionRegex": "([0-9.]+)" },
	      "source": { "type": "githubRelease", "repository": "acme/example" },
	      "platforms": [ { "os": ["darwin"], "arch": ["arm64"], "assetRegex": "example-.*", "install": { "type": "direct", "destination": "/usr/local/bin/example" } } ],
	      "verify": [ { "type": "githubDigest", "assetRegex": "example-.*" } ]
	    }
	  } ] } }
	}`)
	_, errs, parseErr := ParseAndValidate(doc)
	if parseErr != nil {
		t.Fatalf("parse error: %v", parseErr)
	}
	if len(errs) > 0 {
		t.Fatalf("v10 must keep the v9 external recipe surface, got: %v", errs)
	}
}

func TestPortable_host_message_reports_actual_version(t *testing.T) {
	// The message must name the version the file actually declares, or a v10
	// author is told to fix a "schemaVersion 8" error.
	doc := []byte(`{"schemaVersion":"10","defaults":{"packages":[{"id":"jq","host":"macos"}]},"targets":{"macos":{}}}`)
	_, errs, parseErr := ParseAndValidate(doc)
	if parseErr != nil {
		t.Fatalf("parse error: %v", parseErr)
	}
	if len(errs) == 0 {
		t.Fatal("expected a host-predicate error")
	}
	if !strings.Contains(errs[0].Message, `schemaVersion "10"`) {
		t.Errorf("message = %q, want it to name schemaVersion 10", errs[0].Message)
	}
}
