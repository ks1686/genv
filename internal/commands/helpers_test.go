package commands

import (
	"strings"
	"testing"

	"github.com/ks1686/genv/internal/schema"
)

func TestKnownManagerList(t *testing.T) {
	result := KnownManagerList()

	if len(schema.KnownManagers) == 0 {
		if result != "" {
			t.Errorf("expected empty string when no known managers, got %q", result)
		}
		return
	}

	if result == "" {
		t.Fatalf("KnownManagerList() returned empty string but schema has %d managers", len(schema.KnownManagers))
	}

	managers := strings.Split(result, ", ")
	if len(managers) != len(schema.KnownManagers) {
		t.Errorf("expected %d managers, got %d", len(schema.KnownManagers), len(managers))
	}

	for i := 1; i < len(managers); i++ {
		if managers[i-1] >= managers[i] {
			t.Errorf("list is not sorted: %q comes before %q", managers[i-1], managers[i])
		}
	}

	for _, mgr := range managers {
		if _, ok := schema.KnownManagers[mgr]; !ok {
			t.Errorf("manager %q in result is not in schema.KnownManagers", mgr)
		}
	}
}

func TestRedactValue(t *testing.T) {
	tests := []struct {
		name      string
		value     string
		sensitive bool
		want      string
	}{
		{
			name:      "sensitive non-empty",
			value:     "secret123",
			sensitive: true,
			want:      "[redacted]",
		},
		{
			name:      "sensitive empty",
			value:     "",
			sensitive: true,
			want:      "",
		},
		{
			name:      "non-sensitive non-empty",
			value:     "public123",
			sensitive: false,
			want:      "public123",
		},
		{
			name:      "non-sensitive empty",
			value:     "",
			sensitive: false,
			want:      "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := RedactValue(tt.value, tt.sensitive); got != tt.want {
				t.Errorf("RedactValue() = %v, want %v", got, tt.want)
			}
		})
	}
}
