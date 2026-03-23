// Package version provides constraint-satisfaction logic for package versions.
// genv uses simple string-based version constraints:
//   - empty or "*": always satisfied
//   - "x.y.*": prefix wildcard (installed must start with "x.y.")
//   - "x.y.z": exact match
package version

import "strings"

// Satisfies reports whether installed satisfies the version constraint.
// An empty or "*" constraint is always satisfied.
// An empty installed string never satisfies a non-wildcard, non-empty constraint.
func Satisfies(constraint, installed string) bool {
	if constraint == "" || constraint == "*" {
		return true
	}
	if installed == "" {
		return false
	}
	if strings.HasSuffix(constraint, ".*") {
		prefix := strings.TrimSuffix(constraint, "*")
		return strings.HasPrefix(installed, prefix)
	}
	return installed == constraint
}
