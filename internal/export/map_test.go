package export

import (
	"reflect"
	"strings"
	"testing"

	"github.com/ks1686/genv/internal/schema"
)

func TestMapSuggestsUbuntuMappingForMacOSOnlyMASPackageWithoutMutation(t *testing.T) {
	f := &schema.GenvFile{
		SchemaVersion: schema.Version8,
		Targets: map[string]*schema.TargetBundle{
			"macos": {
				Packages: []schema.Package{{
					ID:     "numbers",
					Prefer: "mas",
					Managers: map[string]string{
						"mas": "409203825",
					},
				}},
			},
		},
	}
	before := cloneGenvFileForMapTest(t, f)

	report := Suggest(f, "ubuntu")

	if !reflect.DeepEqual(f, before) {
		t.Fatalf("Suggest mutated input:\n got: %+v\nwant: %+v", f, before)
	}
	item := findReportItem(report, "manager-mapping-suggested", "numbers")
	if item == nil {
		t.Fatalf("missing mapping suggestion in report: %+v", report)
	}
	if item.Class != ClassSuggestion {
		t.Fatalf("suggestion class = %q, want %q", item.Class, ClassSuggestion)
	}
	if !strings.Contains(item.Message, "ubuntu") ||
		!strings.Contains(item.Message, "snap") ||
		!strings.Contains(item.Message, "linuxbrew") ||
		!strings.Contains(item.Message, "mas") {
		t.Fatalf("suggestion message does not mention expected managers/target: %q", item.Message)
	}
}

func cloneGenvFileForMapTest(t *testing.T, f *schema.GenvFile) *schema.GenvFile {
	t.Helper()
	return &schema.GenvFile{
		SchemaVersion: f.SchemaVersion,
		Targets: map[string]*schema.TargetBundle{
			"macos": {
				Packages: []schema.Package{{
					ID:       f.Targets["macos"].Packages[0].ID,
					Prefer:   f.Targets["macos"].Packages[0].Prefer,
					Managers: map[string]string{"mas": f.Targets["macos"].Packages[0].Managers["mas"]},
				}},
			},
		},
	}
}

func findReportItem(report []ReportItem, code, packageID string) *ReportItem {
	for i := range report {
		if report[i].Code == code && report[i].PackageID == packageID {
			return &report[i]
		}
	}
	return nil
}
