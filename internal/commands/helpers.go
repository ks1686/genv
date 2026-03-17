package commands

import (
	"sort"
	"strings"

	"github.com/ks1686/gpm/internal/schema"
)

// KnownManagerList returns a sorted, comma-separated string of all known manager names.
func KnownManagerList() string {
	names := make([]string, 0, len(schema.KnownManagers))
	for k := range schema.KnownManagers {
		names = append(names, k)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}
