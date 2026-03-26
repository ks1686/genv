package adapter

import (
	"strings"
	"unicode/utf8"
)

// containsFold reports whether substr is within s, case-insensitively.
// It is a drop-in replacement for strings.Contains(strings.ToLower(s), strings.ToLower(substr))
// that avoids allocations for ASCII-only strings.
func containsFold(s, substr string) bool {
	if substr == "" {
		return true
	}

	// Fast path for ASCII-only strings.
	// Check if substr is purely ASCII.
	for i := 0; i < len(substr); i++ {
		if substr[i] >= utf8.RuneSelf {
			goto fallback
		}
	}

	if len(s) < len(substr) {
		return false
	}

	for i := 0; i <= len(s)-len(substr); i++ {
		match := true
		for j := 0; j < len(substr); j++ {
			c1 := s[i+j]
			c2 := substr[j]

			if c1 >= utf8.RuneSelf {
				// Found non-ASCII in s, fallback to slow path.
				goto fallback
			}

			if c1 == c2 {
				continue
			}

			// ASCII case folding
			if 'A' <= c1 && c1 <= 'Z' {
				c1 += 'a' - 'A'
			}
			if 'A' <= c2 && c2 <= 'Z' {
				c2 += 'a' - 'A'
			}

			if c1 != c2 {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false

fallback:
	// Slow path for non-ASCII strings.
	return strings.Contains(strings.ToLower(s), strings.ToLower(substr))
}
