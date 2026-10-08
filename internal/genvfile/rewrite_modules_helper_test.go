package genvfile

import (
	"strings"

	"github.com/ks1686/genv/internal/schema"
)

func schemaParse(data string) (*schema.GenvFile, []schema.ValidationError, error) {
	return schema.ParseAndValidate([]byte(data))
}

func packageWithID(id string) schema.Package {
	return schema.Package{ID: id}
}

func specWithSelection(names []string) *schema.GenvFile {
	return &schema.GenvFile{
		SchemaVersion: schema.Version10,
		Defaults:      &schema.TargetBundle{UseModules: names},
		Targets:       map[string]*schema.TargetBundle{"macos": {}},
	}
}

func containsAll(haystack string, needles ...string) bool {
	for _, n := range needles {
		if !strings.Contains(haystack, n) {
			return false
		}
	}
	return true
}
