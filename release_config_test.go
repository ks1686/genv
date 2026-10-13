package main

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

func TestReleaseConfigDoesNotPublishSnap(t *testing.T) {
	t.Parallel()

	if _, err := os.Stat("snap/snapcraft.yaml"); !os.IsNotExist(err) {
		if err == nil {
			t.Error("snap/snapcraft.yaml remains after the Snap distribution channel was discontinued")
		} else {
			t.Fatalf("stat snap/snapcraft.yaml: %v", err)
		}
	}

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

// The release chain holds tokens that can publish binaries, the Homebrew cask,
// and the Scoop manifest, so every third-party action it runs is pinned to a
// full commit SHA rather than a movable tag.
func TestReleaseWorkflowsPinActionsToCommitSHAs(t *testing.T) {
	t.Parallel()

	pinned := regexp.MustCompile(`^[^@\s]+@[0-9a-f]{40}(\s+#.*)?$`)
	for _, path := range []string{".github/workflows/release.yml", ".github/workflows/test.yml"} {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		for i, line := range strings.Split(string(data), "\n") {
			trimmed := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "- "))
			if !strings.HasPrefix(trimmed, "uses:") {
				continue
			}
			ref := strings.TrimSpace(strings.TrimPrefix(trimmed, "uses:"))
			if strings.HasPrefix(ref, "./") {
				continue // local reusable workflow
			}
			if !pinned.MatchString(ref) {
				t.Errorf("%s:%d: action %q is not pinned to a commit SHA", path, i+1, ref)
			}
		}
	}
}
