package adapter

import (
	"encoding/json"
	"errors"
	"os/exec"
	"strings"
)

// Composer manages globally required Composer packages via `composer global`.
// Project-local Composer operations are intentionally out of scope; genv never
// runs a bare `composer update` against a project.
type Composer struct{}

func (Composer) Name() string { return "composer" }

func (Composer) Available() bool {
	_, err := lookPath("composer")
	return err == nil
}

func (Composer) NormalizeID(id string, managers map[string]string) (string, bool) {
	return normalizeID("composer", id, managers)
}

func (Composer) PlanInstall(pkgName string) []string {
	return []string{"composer", "global", "require", pkgName}
}

func (Composer) PlanUninstall(pkgName string) []string {
	return []string{"composer", "global", "remove", composerBaseName(pkgName)}
}

func (Composer) PlanUpgrade(pkgName string) []string {
	return []string{"composer", "global", "require", pkgName}
}

func (Composer) PlanClean() [][]string {
	return [][]string{{"composer", "clear-cache"}}
}

func (c Composer) Query(pkgName string) (bool, error) {
	entries, err := c.listEntries()
	if err != nil {
		return false, err
	}
	_, ok := findComposerEntry(entries, pkgName)
	return ok, nil
}

func (c Composer) ListInstalled() ([]string, error) {
	entries, err := c.listEntries()
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.name)
	}
	return names, nil
}

func (c Composer) QueryVersion(pkgName string) (string, error) {
	entries, err := c.listEntries()
	if err != nil {
		return "", err
	}
	entry, ok := findComposerEntry(entries, pkgName)
	if !ok {
		return "", nil
	}
	return entry.version, nil
}

func (c Composer) ListInstalledVersions() (map[string]string, error) {
	entries, err := c.listEntries()
	if err != nil {
		return nil, err
	}
	versions := make(map[string]string, len(entries))
	for _, entry := range entries {
		versions[entry.name] = entry.version
	}
	return versions, nil
}

type composerEntry struct {
	name    string
	version string
}

func (Composer) listEntries() ([]composerEntry, error) {
	out, err := exec.Command("composer", "global", "show", "--format=json").Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return nil, nil
		}
		return nil, err
	}
	return parseComposerShowJSON(out)
}

// composerBaseName strips a Composer version constraint suffix (":^1.2") while
// preserving the vendor/package prefix (the single "/" separator is kept).
func composerBaseName(pkgName string) string {
	if before, _, ok := strings.Cut(pkgName, ":"); ok {
		return before
	}
	return pkgName
}

func findComposerEntry(entries []composerEntry, pkgName string) (composerEntry, bool) {
	base := composerBaseName(pkgName)
	for _, entry := range entries {
		if entry.name == base {
			return entry, true
		}
	}
	return composerEntry{}, false
}

// parseComposerShowJSON parses `composer global show --format=json` output,
// which has an "installed" array of {name, version} objects.
func parseComposerShowJSON(data []byte) ([]composerEntry, error) {
	if len(strings.TrimSpace(string(data))) == 0 {
		return nil, nil
	}
	var root struct {
		Installed []struct {
			Name    string `json:"name"`
			Version string `json:"version"`
		} `json:"installed"`
	}
	if err := json.Unmarshal(data, &root); err != nil {
		return nil, err
	}
	entries := make([]composerEntry, 0, len(root.Installed))
	for _, item := range root.Installed {
		if item.Name == "" {
			continue
		}
		entries = append(entries, composerEntry{name: item.Name, version: strings.TrimPrefix(item.Version, "v")})
	}
	return entries, nil
}
