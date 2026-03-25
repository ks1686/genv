package commands

import (
	"sort"
	"strings"

	"github.com/ks1686/genv/internal/schema"
)

// knownManagerList is computed once at init from the constant KnownManagers map.
var knownManagerList = func() string {
	names := make([]string, 0, len(schema.KnownManagers))
	for k := range schema.KnownManagers {
		names = append(names, k)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}()

// KnownManagerList returns a sorted, comma-separated string of all known manager names.
func KnownManagerList() string { return knownManagerList }

// RedactValue returns value unchanged unless sensitive is true, in which case
// it returns "[redacted]". Use for any user-facing output that may expose secrets.
func RedactValue(value string, sensitive bool) string {
	if sensitive && value != "" {
		return "[redacted]"
	}
	return value
}
