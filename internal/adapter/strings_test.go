package adapter

import (
	"strings"
	"testing"
)

func TestContainsFold(t *testing.T) {
	tests := []struct {
		s      string
		substr string
		want   bool
	}{
		{"abc", "b", true},
		{"ABC", "b", true},
		{"abc", "B", true},
		{"ABC", "B", true},
		{"hello world", "WORLD", true},
		{"", "a", false},
		{"a", "", true},
		{"", "", true},
		{"Hello ß", "ß", true},
		{"A ß C", "a ß c", true},
		{"ÄÖÜ", "äöü", true},
	}
	for _, tc := range tests {
		got := containsFold(tc.s, tc.substr)
		if got != tc.want {
			t.Errorf("containsFold(%q, %q) = %v; want %v", tc.s, tc.substr, got, tc.want)
		}
	}
}

func BenchmarkContainsFold(b *testing.B) {
	lines := make([]string, 1000)
	for i := 0; i < 1000; i++ {
		lines[i] = "Some-Package-Name-With-Mixed-Case"
	}
	query := "MIXED"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var count int
		for _, line := range lines {
			if containsFold(line, query) {
				count++
			}
		}
	}
}

func BenchmarkContainsToLower(b *testing.B) {
	lines := make([]string, 1000)
	for i := 0; i < 1000; i++ {
		lines[i] = "Some-Package-Name-With-Mixed-Case"
	}
	query := "MIXED"
	q := strings.ToLower(query)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var count int
		for _, line := range lines {
			if strings.Contains(strings.ToLower(line), q) {
				count++
			}
		}
	}
}
