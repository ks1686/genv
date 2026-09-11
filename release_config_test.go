package main

import (
	"os"
	"strings"
	"testing"
)

func TestReleaseConfigDoesNotPublishSnap(t *testing.T) {
	t.Parallel()

	checks := []struct {
		path      string
		forbidden []string
	}{
		{
			path: ".goreleaser.yml",
			forbidden: []string{
				"snapcrafts:",
			},
		},
		{
			path: ".github/workflows/release.yml",
			forbidden: []string{
				"Install snapcraft",
				"SNAPCRAFT_STORE_CREDENTIALS",
			},
		},
	}

	for _, check := range checks {
		data, err := os.ReadFile(check.path)
		if err != nil {
			t.Fatalf("read %s: %v", check.path, err)
		}
		for _, forbidden := range check.forbidden {
			if strings.Contains(string(data), forbidden) {
				t.Errorf("%s still contains discontinued Snap publication setting %q", check.path, forbidden)
			}
		}
	}
}
