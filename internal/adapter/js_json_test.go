package adapter

import (
	"maps"
	"slices"
	"testing"
)

func TestParseJSListJSON_whenNpmObjectIncludesScopedPackage(t *testing.T) {
	data := []byte(`{
		"dependencies": {
			"@colbymchenry/codegraph": {"version": "1.0.1"},
			"typescript": {"version": "5.9.2"}
		}
	}`)

	got, err := parseJSListJSON(data)

	if err != nil {
		t.Fatalf("parseJSListJSON npm object: %v", err)
	}
	want := []jsPackageEntry{{name: "@colbymchenry/codegraph", version: "1.0.1"}, {name: "typescript", version: "5.9.2"}}
	if !slices.Equal(got, want) {
		t.Errorf("parseJSListJSON npm object = %v, want %v", got, want)
	}
}

func TestParseJSListJSON_whenPnpmArrayIncludesScopedPackage(t *testing.T) {
	data := []byte(`[{
		"dependencies": {
			"@scope/tool": {"version": "2.3.4"},
			"eslint": {"version": "9.0.0"}
		}
	}]`)

	got, err := parseJSListJSON(data)

	if err != nil {
		t.Fatalf("parseJSListJSON pnpm array: %v", err)
	}
	wantVersions := map[string]string{"@scope/tool": "2.3.4", "eslint": "9.0.0"}
	if !maps.Equal(entriesVersions(got), wantVersions) {
		t.Errorf("parseJSListJSON pnpm array versions = %v, want %v", entriesVersions(got), wantVersions)
	}
}

func TestParseJSListJSON_whenMalformedReturnsError(t *testing.T) {
	_, err := parseJSListJSON([]byte(`not-json`))

	if err == nil {
		t.Fatal("parseJSListJSON malformed: expected error")
	}
}
