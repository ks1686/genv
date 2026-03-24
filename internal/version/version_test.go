package version

import (
	"strings"
	"testing"
)

func TestSatisfies(t *testing.T) {
	tests := []struct {
		constraint string
		installed  string
		want       bool
	}{
		// empty/wildcard constraints are always satisfied
		{"", "1.0.0", true},
		{"", "", true},
		{"*", "1.0.0", true},
		{"*", "", true},
		// exact match
		{"1.0.0", "1.0.0", true},
		{"1.0.0", "1.0.1", false},
		{"1.0.0", "2.0.0", false},
		// prefix wildcard
		{"0.10.*", "0.10.0", true},
		{"0.10.*", "0.10.99", true},
		{"0.10.*", "0.10.1-1", true},
		{"0.10.*", "0.11.0", false},
		{"0.10.*", "1.10.0", false},
		// empty installed with non-trivial constraint
		{"1.0.0", "", false},
		{"1.*", "", false},
	}
	for _, tc := range tests {
		got := Satisfies(tc.constraint, tc.installed)
		if got != tc.want {
			t.Errorf("Satisfies(%q, %q) = %v, want %v", tc.constraint, tc.installed, got, tc.want)
		}
	}
}

// FuzzSatisfies checks semantic invariants that must hold for all inputs:
//
//  1. Empty or "*" constraint is always satisfied, regardless of installed.
//  2. Empty installed never satisfies a non-trivial, non-empty constraint.
//  3. Satisfies never panics.
//  4. Prefix wildcard constraints (ending in ".*") are satisfied iff the
//     installed string starts with the prefix (sans trailing "*").
func FuzzSatisfies(f *testing.F) {
	// Seed with known interesting inputs so the fuzzer has a head-start.
	seeds := []struct{ c, i string }{
		{"", "1.0.0"},
		{"*", ""},
		{"1.0.0", "1.0.0"},
		{"1.0.0", "1.0.1"},
		{"0.10.*", "0.10.5"},
		{"0.10.*", "0.11.0"},
		{"1.*", ""},
		{"", ""},
	}
	for _, s := range seeds {
		f.Add(s.c, s.i)
	}

	f.Fuzz(func(t *testing.T, constraint, installed string) {
		// Must never panic.
		got := Satisfies(constraint, installed)

		// Invariant 1: empty or wildcard-only constraint is always satisfied.
		if constraint == "" || constraint == "*" {
			if !got {
				t.Errorf("Satisfies(%q, %q) = false; empty/wildcard constraint must always be true", constraint, installed)
			}
		}

		// Invariant 2: empty installed string never satisfies a non-trivial constraint.
		if installed == "" && constraint != "" && constraint != "*" {
			if got {
				t.Errorf("Satisfies(%q, %q) = true; empty installed must not satisfy non-wildcard constraint", constraint, installed)
			}
		}

		// Invariant 3: prefix wildcard consistency.
		if strings.HasSuffix(constraint, ".*") {
			prefix := strings.TrimSuffix(constraint, "*")
			wantTrue := strings.HasPrefix(installed, prefix)
			if got != wantTrue {
				t.Errorf("Satisfies(%q, %q) = %v; prefix wildcard disagrees with strings.HasPrefix (want %v)", constraint, installed, got, wantTrue)
			}
		}
	})
}
