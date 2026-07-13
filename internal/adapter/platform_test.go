package adapter

import "testing"

func TestAutomaticOnGOOS(t *testing.T) {
	tests := []struct {
		name    string
		manager string
		goos    string
		want    bool
	}{
		{name: "Darwin accepts Homebrew", manager: "brew", goos: "darwin", want: true},
		{name: "Darwin rejects Linuxbrew", manager: "linuxbrew", goos: "darwin", want: false},
		{name: "Linux rejects Homebrew", manager: "brew", goos: "linux", want: false},
		{name: "Linux accepts Linuxbrew", manager: "linuxbrew", goos: "linux", want: true},
		{name: "unrelated manager on Darwin", manager: "pacman", goos: "darwin", want: true},
		{name: "unrelated manager on Linux", manager: "pacman", goos: "linux", want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := AutomaticOnGOOS(tt.manager, tt.goos); got != tt.want {
				t.Errorf("AutomaticOnGOOS(%q, %q) = %v, want %v", tt.manager, tt.goos, got, tt.want)
			}
		})
	}
}
