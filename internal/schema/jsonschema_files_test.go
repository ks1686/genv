package schema_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/ks1686/genv/internal/schema"
)

// The JSON Schema files under schema/v* are published for editors and CI
// checkouts. Nothing in the Go code reads them, so they drift silently: a new
// manager, a removed manager, or a renamed field leaves them advertising
// something the validator refuses (or hiding something it accepts). These
// tests read the same registry the validator does, so the files fail the build
// when they disagree.

const schemaRoot = "../../schema"

// adapterNamePattern is the shape schema.ValidAdapterName accepts. A portable
// spec may reference a custom adapter by name, so the published schemas cannot
// close the manager set to the built-ins alone.
const adapterNamePattern = `^[a-z][a-z0-9-]*$`

func knownManagersSorted() []string {
	names := make([]string, 0, len(schema.KnownManagers))
	for name := range schema.KnownManagers {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

func loadPublishedSchema(t *testing.T, version string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(schemaRoot, "v"+version, "genv.json"))
	if err != nil {
		t.Fatalf("read published schema v%s: %v", version, err)
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("published schema v%s is not valid JSON: %v", version, err)
	}
	return doc
}

// lookup walks a document by key path, returning nil when any segment is
// absent or not an object.
func lookup(t *testing.T, doc map[string]any, path ...string) any {
	t.Helper()
	var node any = doc
	for _, segment := range path {
		object, ok := node.(map[string]any)
		if !ok {
			return nil
		}
		node, ok = object[segment]
		if !ok {
			return nil
		}
	}
	return node
}

func enumAt(t *testing.T, node any) []string {
	t.Helper()
	object, ok := node.(map[string]any)
	if !ok {
		return nil
	}
	raw, ok := object["enum"].([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		text, ok := item.(string)
		if !ok {
			t.Fatalf("manager enum contains a non-string entry %#v", item)
		}
		out = append(out, text)
	}
	return out
}

// firstAnyOf returns the first alternative of an anyOf node, or nil. It reads
// through map[string]any without asserting so a schema that has not been
// updated yet reports a difference instead of panicking.
func firstAnyOf(node any) any {
	object, ok := node.(map[string]any)
	if !ok {
		return nil
	}
	alternatives, ok := object["anyOf"].([]any)
	if !ok || len(alternatives) == 0 {
		return nil
	}
	return alternatives[0]
}

func TestPublishedSchemasIdentifyTheirVersion(t *testing.T) {
	for _, version := range []string{"1", "8", "9", "10"} {
		doc := loadPublishedSchema(t, version)
		if got := lookup(t, doc, "properties", "schemaVersion", "const"); got != version {
			t.Errorf("schema v%s: schemaVersion const = %#v, want %q", version, got, version)
		}
		wantID := fmt.Sprintf("https://github.com/ks1686/genv/schema/v%s/genv.json", version)
		if got := lookup(t, doc, "$id"); got != wantID {
			t.Errorf("schema v%s: $id = %#v, want %q", version, got, wantID)
		}
		if got := lookup(t, doc, "$schema"); got != "http://json-schema.org/draft-07/schema#" {
			t.Errorf("schema v%s: $schema = %#v, want draft-07", version, got)
		}
	}
}

// TestPublishedSchemasListEveryManager is the regression test for the drift
// found in the readiness audit: schema/v1 listed managers genv does not ship
// (flatpak) and missed the ones it does, while v8-v10 listed no managers at all.
func TestPublishedSchemasListEveryManager(t *testing.T) {
	want := knownManagersSorted()
	for _, version := range []string{"1", "8", "9", "10"} {
		t.Run("v"+version, func(t *testing.T) {
			doc := loadPublishedSchema(t, version)
			packageProps, ok := lookup(t, doc, "$defs", "package", "properties").(map[string]any)
			if !ok {
				t.Fatalf("schema v%s: no $defs.package.properties", version)
			}

			// prefer: v1 has no adapters block, so a closed enum is exact.
			// v8+ accept a declared adapter name too, so the enum is the
			// first anyOf branch and the second branch carries the pattern.
			prefer := packageProps["prefer"]
			got := enumAt(t, prefer)
			if got == nil {
				got = enumAt(t, firstAnyOf(prefer))
			}
			if !slices.Equal(got, want) {
				t.Errorf("schema v%s: prefer managers = %v\nwant %v\n(differs: %s)",
					version, got, want, describeManagerDiff(got, want))
			}

			managers, ok := packageProps["managers"].(map[string]any)
			if !ok {
				t.Fatalf("schema v%s: no package.managers object", version)
			}
			names := enumAt(t, managers["propertyNames"])
			if names == nil {
				names = enumAt(t, firstAnyOf(managers["propertyNames"]))
			}
			if len(names) != 0 && !slices.Equal(names, want) {
				t.Errorf("schema v%s: managers keys = %v\nwant %v\n(differs: %s)",
					version, names, want, describeManagerDiff(names, want))
			}

			if version != "1" {
				// The adapters escape hatch must stay open, or the schema
				// refuses a spec the validator accepts.
				for field, node := range map[string]any{
					"prefer":                 prefer,
					"managers.propertyNames": managers["propertyNames"],
				} {
					if !mentionsAdapterPattern(node) {
						t.Errorf("schema v%s: %s does not accept a custom adapter name (pattern %s)",
							version, field, adapterNamePattern)
					}
				}
			}
		})
	}
}

func mentionsAdapterPattern(node any) bool {
	switch typed := node.(type) {
	case map[string]any:
		if pattern, ok := typed["pattern"].(string); ok && pattern == adapterNamePattern {
			return true
		}
		for _, child := range typed {
			if mentionsAdapterPattern(child) {
				return true
			}
		}
	case []any:
		for _, child := range typed {
			if mentionsAdapterPattern(child) {
				return true
			}
		}
	}
	return false
}

func describeManagerDiff(got, want []string) string {
	var missing, extra []string
	for _, name := range want {
		if !slices.Contains(got, name) {
			missing = append(missing, name)
		}
	}
	for _, name := range got {
		if !slices.Contains(want, name) {
			extra = append(extra, name)
		}
	}
	return strings.Join([]string{
		"missing: " + strings.Join(missing, ", "),
		"not a known manager: " + strings.Join(extra, ", "),
	}, "; ")
}