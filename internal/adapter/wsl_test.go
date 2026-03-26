package adapter

import (
	"strings"
	"testing"
)

// ---- isWindowsMountPath -----------------------------------------------------

func TestIsWindowsMountPath(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		{"/mnt/c", true},
		{"/mnt/c/", true},
		{"/mnt/c/Windows/System32", true},
		{"/mnt/d", true},
		{"/mnt/z/Program Files", true},
		// Not Windows mounts
		{"/mnt/data", false}, // more than one char after /mnt/
		{"/mnt/C", false},    // uppercase drive letter
		{"/mnt/1", false},    // digit, not a letter
		{"/usr/bin", false},
		{"/mnt", false},
		{"", false},
	}

	for _, tc := range cases {
		got := isWindowsMountPath(tc.path)
		if got != tc.want {
			t.Errorf("isWindowsMountPath(%q) = %v, want %v", tc.path, got, tc.want)
		}
	}
}

// ---- sanitizePathForWSL -----------------------------------------------------

func TestSanitizePathForWSL(t *testing.T) {
	input := strings.Join([]string{
		"/usr/local/sbin",
		"/usr/local/bin",
		"/mnt/c/Windows/System32",
		"/usr/bin",
		"/mnt/c/Program Files/Git/cmd",
		"/mnt/d/tools",
		"/home/user/.local/bin",
	}, ":")

	got := sanitizePathForWSL(input)
	parts := strings.Split(got, ":")

	wantPresent := []string{"/usr/local/sbin", "/usr/local/bin", "/usr/bin", "/home/user/.local/bin"}
	wantAbsent := []string{"/mnt/c/Windows/System32", "/mnt/c/Program Files/Git/cmd", "/mnt/d/tools"}

	for _, p := range wantPresent {
		found := false
		for _, part := range parts {
			if part == p {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("sanitizePathForWSL: expected %q to be present in result %q", p, got)
		}
	}

	for _, p := range wantAbsent {
		for _, part := range parts {
			if part == p {
				t.Errorf("sanitizePathForWSL: expected %q to be removed from result %q", p, got)
			}
		}
	}
}

func TestSanitizePathForWSL_EmptyPath(t *testing.T) {
	if got := sanitizePathForWSL(""); got != "" {
		t.Errorf("sanitizePathForWSL(\"\") = %q, want \"\"", got)
	}
}

func TestSanitizePathForWSL_NoWindowsPaths(t *testing.T) {
	input := "/usr/bin:/usr/local/bin:/home/user/.local/bin"
	got := sanitizePathForWSL(input)
	if got != input {
		t.Errorf("sanitizePathForWSL with no Windows paths modified the PATH: got %q, want %q", got, input)
	}
}

func BenchmarkIsWSL(b *testing.B) {
	for i := 0; i < b.N; i++ {
		isWSL()
	}
}
