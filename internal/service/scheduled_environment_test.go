package service

import (
	"strings"
	"testing"
)

func TestScheduledPath(t *testing.T) {
	tests := []struct {
		name      string
		pathValue string
		goos      string
		want      []string
	}{
		{
			name:      "darwin strips Homebrew shims and retains absolute entries",
			pathValue: "/opt/homebrew/Library/Homebrew/shims/shared:/custom/bin::relative:/usr/bin:/custom/bin:/another/bin",
			goos:      "darwin",
			want: []string{
				"/custom/bin", "/usr/bin", "/another/bin",
				"/opt/homebrew/bin", "/opt/homebrew/sbin", "/usr/local/bin", "/usr/local/sbin",
				"/bin", "/usr/sbin", "/sbin",
			},
		},
		{
			name:      "darwin retains absolute entries and appends defaults",
			pathValue: "/custom/bin::relative:/usr/bin:/custom/bin:/another/bin",
			goos:      "darwin",
			want: []string{
				"/custom/bin", "/usr/bin", "/another/bin",
				"/opt/homebrew/bin", "/opt/homebrew/sbin", "/usr/local/bin", "/usr/local/sbin",
				"/bin", "/usr/sbin", "/sbin",
			},
		},
		{
			name:      "linux falls back to platform defaults",
			pathValue: "relative::also-relative",
			goos:      "linux",
			want: []string{
				"/home/linuxbrew/.linuxbrew/bin", "/home/linuxbrew/.linuxbrew/sbin",
				"/usr/local/bin", "/usr/local/sbin", "/usr/bin", "/bin", "/usr/sbin", "/sbin",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := strings.Split(ScheduledPath(tt.pathValue, tt.goos), ":"); !equalStrings(got, tt.want) {
				t.Fatalf("ScheduledPath(%q, %q) = %v, want %v", tt.pathValue, tt.goos, got, tt.want)
			}
		})
	}
}

func equalStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
