package schema

import "strings"

// MaxVersionConstraintBytes bounds a package version constraint before it is
// passed to adapters. genv supports only an exact version, "*", or a prefix
// wildcard such as "1.2.*".
const MaxVersionConstraintBytes = 128

// ValidVersionConstraint reports whether constraint is one of genv's supported
// constraint forms. It deliberately does not parse semantic versions: package
// managers use many version syntaxes, and genv compares these constraints as
// strings in internal/version.
func ValidVersionConstraint(constraint string) bool {
	if constraint == "" || constraint == "*" {
		return true
	}
	if len(constraint) > MaxVersionConstraintBytes || strings.HasPrefix(constraint, "-") {
		return false
	}
	for _, r := range constraint {
		if r <= 0x1f || r == 0x7f || r == ' ' || r == '\t' {
			return false
		}
	}
	if strings.Contains(constraint, "*") {
		return strings.HasSuffix(constraint, ".*") && strings.Count(constraint, "*") == 1 && len(constraint) > 2
	}
	return true
}
