package adapter

import "strings"

// Vscode manages VS Code / Cursor extensions via the editor CLI. The binary
// is resolved once as `cursor` when that is on PATH, otherwise `code`.
// Extensions are tracked by their "<publisher>.<name>" id. There is no
// broad "update all extensions" operation: upgrade reinstalls the tracked
// id, which installs the latest stable version for that single extension.
type Vscode struct{}

func (Vscode) Name() string { return "vscode" }

// vscodeCLI returns the editor CLI to invoke. Cursor-only hosts have
// `cursor` and no `code`; requiring a user-level shim is not supported.
// The second return is whether that CLI is actually on PATH.
func vscodeCLI() (name string, ok bool) {
	if _, err := lookPath("cursor"); err == nil {
		return "cursor", true
	}
	if _, err := lookPath("code"); err == nil {
		return "code", true
	}
	return "code", false
}

func vscodeCLIName() string {
	name, _ := vscodeCLI()
	return name
}

func (Vscode) Available() bool {
	_, ok := vscodeCLI()
	return ok
}

func (Vscode) NormalizeID(id string, managers map[string]string) (string, bool) {
	return normalizeID("vscode", id, managers)
}

func (Vscode) PlanInstall(pkgName string) []string {
	return []string{vscodeCLIName(), "--install-extension", pkgName}
}

func (Vscode) PlanUninstall(pkgName string) []string {
	return []string{vscodeCLIName(), "--uninstall-extension", vscodeExtensionID(pkgName)}
}

// PlanUpgrade reinstalls the single tracked extension with --force, which pulls
// the latest *stable* version for that id. It never passes --pre-release and
// never runs a broad update-all.
func (Vscode) PlanUpgrade(pkgName string) []string {
	return []string{vscodeCLIName(), "--install-extension", vscodeExtensionID(pkgName), "--force"}
}

func (Vscode) PlanClean() [][]string { return nil }

func (v Vscode) Query(pkgName string) (bool, error) {
	entries, err := v.listEntries()
	if err != nil {
		return false, err
	}
	_, ok := entries[strings.ToLower(vscodeExtensionID(pkgName))]
	return ok, nil
}

func (v Vscode) ListInstalled() ([]string, error) {
	entries, err := v.listEntries()
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for name := range entries {
		names = append(names, name)
	}
	return names, nil
}

func (v Vscode) QueryVersion(pkgName string) (string, error) {
	entries, err := v.listEntries()
	if err != nil {
		return "", err
	}
	return entries[strings.ToLower(vscodeExtensionID(pkgName))], nil
}

func (v Vscode) ListInstalledVersions() (map[string]string, error) {
	return v.listEntries()
}

// ListOutdated reports tracked VS Code/Cursor extensions whose installed
// version differs from the marketplace's newest stable version. Pre-release
// gallery versions are ignored: PlanUpgrade cannot install them.
func (v Vscode) ListOutdated(pkgNames []string) (map[string]string, error) {
	installed, err := v.listEntries()
	if err != nil {
		return nil, err
	}
	names := pkgNames
	if len(names) == 0 {
		names = make([]string, 0, len(installed))
		for name := range installed {
			names = append(names, name)
		}
	}

	ids := make([]string, 0, len(names))
	for _, name := range names {
		ids = append(ids, vscodeExtensionID(name))
	}
	latest, err := fetchVscodeLatestStableVersions(ids)
	if err != nil {
		return nil, err
	}

	outdated := make(map[string]string)
	for _, name := range names {
		id := vscodeExtensionID(name)
		current, ok := installed[strings.ToLower(id)]
		if !ok {
			continue
		}
		want := latest[strings.ToLower(id)]
		if want != "" && want != current {
			outdated[name] = want
		}
	}
	if len(outdated) == 0 {
		return nil, nil
	}
	return outdated, nil
}

// vscodeExtensionID strips an "@version" suffix, leaving the publisher.name id
// that the editor CLI expects for uninstall and query.
func vscodeExtensionID(pkgName string) string {
	return atVersionBaseName(pkgName)
}

// listEntries parses `<cli> --list-extensions --show-versions`, whose lines
// are "publisher.name@version". Extension ids are matched
// case-insensitively, so keys are lowercased.
func (Vscode) listEntries() (map[string]string, error) {
	lines, err := runListOutput(vscodeCLIName(), "--list-extensions", "--show-versions")
	if err != nil {
		return nil, err
	}
	entries := make(map[string]string, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		id, version, ok := strings.Cut(line, "@")
		if !ok || id == "" {
			continue
		}
		entries[strings.ToLower(id)] = version
	}
	return entries, nil
}
