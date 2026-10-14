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

// GitHub's /releases/latest/download/<asset> looks the asset up on the
// current release. A guide that still names genv_4.1.0_... 404s once 4.7.4
// is latest. Version pins in the install guides must match the release
// README calls current.
func TestInstallDocsMatchCurrentReleaseAssets(t *testing.T) {
	t.Parallel()

	readme, err := os.ReadFile("README.md")
	if err != nil {
		t.Fatal(err)
	}
	current := regexp.MustCompile(`genv_(\d+\.\d+\.\d+)_`).FindStringSubmatch(string(readme))
	if current == nil {
		t.Fatal("README.md does not name a genv_<version>_ release asset")
	}
	version := current[1]
	asset := regexp.MustCompile(`genv_(\d+\.\d+\.\d+)_`)
	pin := regexp.MustCompile(`(?m)^version=(\d+\.\d+\.\d+)\s*$`)
	for _, path := range []string{"README.md", "docs/linux-install.md", "docs/windows-install.md", "docs/wsl2-install.md"} {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		text := string(body)
		for _, match := range asset.FindAllStringSubmatch(text, -1) {
			if match[1] != version {
				t.Errorf("%s names genv_%s_ but the current release asset is genv_%s_", path, match[1], version)
			}
		}
		if got := pin.FindStringSubmatch(text); got != nil && got[1] != version {
			t.Errorf("%s pins version=%s, current release is %s", path, got[1], version)
		}
	}
}

func TestReadmeAndAgentsGoVersionMatchModule(t *testing.T) {
	t.Parallel()

	mod, err := os.ReadFile("go.mod")
	if err != nil {
		t.Fatal(err)
	}
	var goVer string
	for _, line := range strings.Split(string(mod), "\n") {
		if strings.HasPrefix(line, "go ") {
			goVer = strings.TrimSpace(strings.TrimPrefix(line, "go "))
			break
		}
	}
	if goVer == "" {
		t.Fatal("go.mod has no go version")
	}
	want := "Go " + goVer
	for _, path := range []string{"README.md", "AGENTS.md"} {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(body), want) {
			t.Errorf("%s does not mention %s from go.mod", path, want)
		}
	}
}
