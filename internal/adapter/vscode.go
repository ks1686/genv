package adapter

import "strings"

// Vscode manages VS Code extensions via the `code` CLI. Extensions are tracked
// by their "<publisher>.<name>" id. There is no broad "update all extensions"
// operation: upgrade reinstalls the tracked id, which installs the latest
// version for that single extension.
type Vscode struct{}

func (Vscode) Name() string { return "vscode" }

func (Vscode) Available() bool {
	_, err := lookPath("code")
	return err == nil
}

func (Vscode) NormalizeID(id string, managers map[string]string) (string, bool) {
	return normalizeID("vscode", id, managers)
}

func (Vscode) PlanInstall(pkgName string) []string {
	return []string{"code", "--install-extension", pkgName}
}

func (Vscode) PlanUninstall(pkgName string) []string {
	return []string{"code", "--uninstall-extension", vscodeExtensionID(pkgName)}
}

// PlanUpgrade reinstalls the single tracked extension with --force, which pulls
// the latest version for that id only. It never runs a broad update.
func (Vscode) PlanUpgrade(pkgName string) []string {
	return []string{"code", "--install-extension", vscodeExtensionID(pkgName), "--force"}
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

// vscodeExtensionID strips an "@version" suffix, leaving the publisher.name id
// that the code CLI expects for uninstall and query.
func vscodeExtensionID(pkgName string) string {
	return atVersionBaseName(pkgName)
}

// listEntries parses `code --list-extensions --show-versions`, whose lines are
// "publisher.name@version". Extension ids are matched case-insensitively, so
// keys are lowercased.
func (Vscode) listEntries() (map[string]string, error) {
	lines, err := runListOutput("code", "--list-extensions", "--show-versions")
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
