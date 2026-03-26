package commands

import (
	"testing"
)

func TestRedactValue(t *testing.T) {
	tests := []struct {
		name      string
		value     string
		sensitive bool
		want      string
	}{
		{
			name:      "not sensitive, empty value",
			value:     "",
			sensitive: false,
			want:      "",
		},
		{
			name:      "not sensitive, non-empty value",
			value:     "secret-123",
			sensitive: false,
			want:      "secret-123",
		},
		{
			name:      "sensitive, empty value",
			value:     "",
			sensitive: true,
			want:      "",
		},
		{
			name:      "sensitive, non-empty value",
			value:     "secret-123",
			sensitive: true,
			want:      "[redacted]",
		},
		{
			name:      "sensitive, single space",
			value:     " ",
			sensitive: true,
			want:      "[redacted]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := RedactValue(tt.value, tt.sensitive); got != tt.want {
				t.Errorf("RedactValue(%q, %v) = %q, want %q", tt.value, tt.sensitive, got, tt.want)
			}
		})
	}
}
