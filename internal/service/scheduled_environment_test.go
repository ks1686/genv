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
		{
			name:      "windows keeps scoop and winget shims and uses semicolon separator",
			pathValue: `C:\Users\qa\scoop\shims;relative;C:\Users\qa\scoop\shims;C:\custom\bin`,
			goos:      "windows",
			want: []string{
				`C:\Users\qa\scoop\shims`, `C:\custom\bin`,
				`C:\Windows\System32`, `C:\Windows`, `C:\Windows\System32\Wbem`,
				`C:\Windows\System32\WindowsPowerShell\v1.0`, `C:\Windows\System32\OpenSSH`,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sep := ":"
			if tt.goos == "windows" {
				sep = ";"
			}
			if got := strings.Split(ScheduledPath(tt.pathValue, tt.goos), sep); !equalStrings(got, tt.want) {
				t.Fatalf("ScheduledPath(%q, %q) = %v, want %v", tt.pathValue, tt.goos, got, tt.want)
			}
		})
	}
}

func TestScheduledWindowsPath_appends_user_shims(t *testing.T) {
	got := ScheduledWindowsPath(
		`C:\custom\bin`,
		`C:\Users\qa`,
		`C:\Users\qa\AppData\Local`,
		`D:\scoop`,
	)
	want := strings.Join([]string{
		`C:\custom\bin`,
		`C:\Windows\System32`,
		`C:\Windows`,
		`C:\Windows\System32\Wbem`,
		`C:\Windows\System32\WindowsPowerShell\v1.0`,
		`C:\Windows\System32\OpenSSH`,
		`D:\scoop\shims`,
		`C:\Users\qa\scoop\shims`,
		`C:\Users\qa\AppData\Local\Microsoft\WindowsApps`,
		`C:\Users\qa\AppData\Local\Microsoft\WinGet\Links`,
	}, ";")
	if got != want {
		t.Fatalf("ScheduledWindowsPath = %q, want %q", got, want)
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
