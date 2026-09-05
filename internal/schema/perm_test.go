package schema

import (
	"strings"
	"testing"
)

func TestParseFilePerm(t *testing.T) {
	tests := []struct {
		name    string
		perm    string
		want    uint32
		wantErr string
	}{
		{name: "unset", perm: "", want: 0},
		{name: "three digits", perm: "644", want: 0o644},
		{name: "leading zero", perm: "0600", want: 0o600},
		{name: "directory", perm: "0700", want: 0o700},
		{name: "letters", perm: "rwx", wantErr: "octal"},
		{name: "digit 8", perm: "0800", wantErr: "octal"},
		{name: "0o prefix", perm: "0o700", wantErr: "octal"},
		{name: "too short", perm: "70", wantErr: "octal"},
		{name: "too long", perm: "00700", wantErr: "octal"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseFilePerm(tc.perm)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("ParseFilePerm(%q) err = %v, want %q", tc.perm, err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseFilePerm(%q) err = %v, want nil", tc.perm, err)
			}
			if uint32(got) != tc.want {
				t.Fatalf("ParseFilePerm(%q) = %04o, want %04o", tc.perm, got, tc.want)
			}
		})
	}
}
