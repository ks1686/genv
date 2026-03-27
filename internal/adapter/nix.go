package adapter

import "strings"

// Nix is the adapter for the Nix package manager (NixOS and any Linux/macOS
// host with Nix installed). Operations target the current user's profile via
// nix-env, so no sudo is needed.
type Nix struct{}

func (Nix) Name() string { return "nix" }

func (Nix) Available() bool {
	_, err := lookPath("nix-env")
	return err == nil
}

func (Nix) NormalizeID(id string, managers map[string]string) (string, bool) {
	return normalizeID("nix", id, managers)
}

// PlanInstall uses the nixpkgs attribute path: nix-env resolves packages via
// attribute paths, not bare names, so the "nixpkgs." prefix is required.
func (Nix) PlanInstall(pkgName string) []string {
	return []string{"nix-env", "-iA", "nixpkgs." + pkgName}
}

func (Nix) PlanUninstall(pkgName string) []string {
	return []string{"nix-env", "-e", pkgName}
}

func (Nix) PlanUpgrade(pkgName string) []string {
	return []string{"nix-env", "-u", pkgName}
}

func (Nix) PlanClean() [][]string {
	// nix-collect-garbage -d deletes all generations and removes unreachable
	// store paths, which is the closest nix equivalent to a cache clean.
	return [][]string{
		{"nix-collect-garbage", "-d"},
	}
}

// nixEnvQuery returns lines from "nix-env -q ^pkgName-", used by both Query
// and QueryVersion to avoid duplicating the subprocess invocation.
// The anchored regex restricts output to packages whose name starts with
// pkgName- (e.g. "^git-" matches "git-2.43.0" but also "git-lfs-3.4.0");
// callers must confirm the exact name via trimVersionSuffix.
func nixEnvQuery(pkgName string) ([]string, error) {
	return runListOutput("nix-env", "-q", "--installed", "^"+pkgName+"-")
}

// Query reports whether pkgName is installed in the user's nix profile.
// The loop is necessary: the regex "^git-" also matches "git-lfs", so we
// confirm each result with trimVersionSuffix to avoid false positives.
func (Nix) Query(pkgName string) (bool, error) {
	lines, err := nixEnvQuery(pkgName)
	if err != nil {
		return false, err
	}
	for _, line := range lines {
		if trimVersionSuffix(line) == pkgName {
			return true, nil
		}
	}
	return false, nil
}

// ListInstalled returns the names of all packages in the user's nix profile.
func (Nix) ListInstalled() ([]string, error) {
	lines, err := runListOutput("nix-env", "-q")
	if err != nil || len(lines) == 0 {
		return lines, err
	}
	names := make([]string, 0, len(lines))
	for _, line := range lines {
		if name := trimVersionSuffix(line); name != "" {
			names = append(names, name)
		}
	}
	return names, nil
}

// QueryVersion returns the installed version of pkgName.
func (Nix) QueryVersion(pkgName string) (string, error) {
	lines, err := nixEnvQuery(pkgName)
	if err != nil || len(lines) == 0 {
		return "", err
	}
	ver, _ := strings.CutPrefix(lines[0], pkgName+"-")
	return ver, nil
}

// Search returns package names from nixpkgs whose name contains query.
// nix-env -qaP outputs "channel.attrPath  pkgname-version" per line; we
// extract the package name from the second field and apply a Go-level
// case-insensitive filter because the regex match covers the full line
// (including descriptions), so not every result's name contains query.
//
// NOTE: nix-env -qaP triggers a full nixpkgs evaluation regardless of the
// pattern, which can take 10–30 s on a cold channel. This is a nix-env
// limitation; prefer "nix search nixpkgs <query>" for interactive use.
func (Nix) Search(query string) ([]string, error) {
	lines, err := runListOutput("nix-env", "-qaP", query)
	if err != nil || len(lines) == 0 {
		return lines, err
	}
	q := strings.ToLower(query)
	seen := make(map[string]bool)
	var names []string
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		name := trimVersionSuffix(fields[1])
		if name != "" && strings.Contains(strings.ToLower(name), q) && !seen[name] {
			seen[name] = true
			names = append(names, name)
		}
	}
	return names, nil
}
