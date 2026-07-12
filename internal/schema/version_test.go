package schema

import "testing"

func TestAtLeastVersion(t *testing.T) {
	cases := []struct {
		name    string
		current string
		min     string
		want    string
	}{
		{"raises v1 to v2", Version, Version2, Version2},
		{"keeps newer v5 over v2", Version5, Version2, Version5},
		{"keeps newer v6 over v3", Version6, Version3, Version6},
		{"keeps newer v6 over v4", Version6, Version4, Version6},
		{"equal stays", Version4, Version4, Version4},
		{"unknown current upgraded to min", "99", Version2, Version2},
		{"empty current upgraded to min", "", Version3, Version3},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := AtLeastVersion(tc.current, tc.min); got != tc.want {
				t.Errorf("AtLeastVersion(%q, %q) = %q, want %q", tc.current, tc.min, got, tc.want)
			}
		})
	}
}
