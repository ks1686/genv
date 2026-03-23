package version

import "testing"

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
