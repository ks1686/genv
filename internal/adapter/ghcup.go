package adapter

import (
	"slices"
	"strings"
)

// Ghcup manages explicitly tracked Haskell tool versions via ghcup. IDs are
// namespaced as "<tool>:<version>" where tool is one of ghc, cabal, hls, stack.
// Bare or unknown formats produce an inert, non-mutating command.
type Ghcup struct{}

func (Ghcup) Name() string { return "ghcup" }

func (Ghcup) Available() bool {
	_, err := lookPath("ghcup")
	return err == nil
}

func (Ghcup) NormalizeID(id string, managers map[string]string) (string, bool) {
	return normalizeID("ghcup", id, managers)
}

func (Ghcup) PlanInstall(pkgName string) []string {
	spec, ok := parseGhcupID(pkgName)
	if !ok {
		return ghcupInvalidCommand(pkgName)
	}
	return []string{"ghcup", "install", spec.tool, spec.version}
}

func (Ghcup) PlanUninstall(pkgName string) []string {
	spec, ok := parseGhcupID(pkgName)
	if !ok {
		return ghcupInvalidCommand(pkgName)
	}
	return []string{"ghcup", "rm", spec.tool, spec.version}
}

func (Ghcup) PlanUpgrade(pkgName string) []string {
	spec, ok := parseGhcupID(pkgName)
	if !ok {
		return ghcupInvalidCommand(pkgName)
	}
	return []string{"ghcup", "install", spec.tool, spec.version}
}

func (Ghcup) PlanClean() [][]string { return nil }

func (g Ghcup) Query(pkgName string) (bool, error) {
	spec, ok := parseGhcupID(pkgName)
	if !ok {
		return false, nil
	}
	installed, err := ghcupInstalledVersions(spec.tool)
	if err != nil {
		return false, err
	}
	return slices.Contains(installed, spec.version), nil
}

func (Ghcup) ListInstalled() ([]string, error) { return nil, nil }

func (Ghcup) QueryVersion(pkgName string) (string, error) {
	spec, ok := parseGhcupID(pkgName)
	if !ok {
		return "", nil
	}
	installed, err := ghcupInstalledVersions(spec.tool)
	if err != nil {
		return "", err
	}
	for _, v := range installed {
		if v == spec.version {
			return v, nil
		}
	}
	return "", nil
}

type ghcupSpec struct {
	tool    string
	version string
}

var ghcupTools = map[string]bool{"ghc": true, "cabal": true, "hls": true, "stack": true}

func parseGhcupID(id string) (ghcupSpec, bool) {
	tool, version, ok := strings.Cut(id, ":")
	if !ok || version == "" || !ghcupTools[tool] {
		return ghcupSpec{}, false
	}
	return ghcupSpec{tool: tool, version: version}, true
}

// ghcupInstalledVersions parses `ghcup list -t <tool> -c installed -r` output.
// The raw (-r) format is space-separated columns beginning with the tool and
// version, e.g. "ghc 9.4.8 ...".
func ghcupInstalledVersions(tool string) ([]string, error) {
	lines, err := runListOutput("ghcup", "list", "-t", tool, "-c", "installed", "-r")
	if err != nil {
		return nil, err
	}
	versions := make([]string, 0, len(lines))
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[0] == tool {
			versions = append(versions, fields[1])
		}
	}
	return versions, nil
}

func ghcupInvalidCommand(pkgName string) []string {
	return []string{"sh", "-c", "printf '%s\n' 'genv: ghcup requires a <tool>:<version> id where tool is ghc, cabal, hls, or stack' >&2; exit 1", "genv-ghcup-invalid", pkgName}
}
