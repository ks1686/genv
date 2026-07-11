package adapter

import "strings"

// Asdf manages asdf plugins and tool versions by explicit ids:
//   - "plugin:<name>"        -> asdf plugin add/remove <name>
//   - "tool:<plugin>@<ver>"  -> asdf install/uninstall <plugin> <ver>
//
// It never runs `asdf plugin update --all` or any broad update; upgrades always
// target one explicit tool version.
type Asdf struct{}

func (Asdf) Name() string { return "asdf" }

func (Asdf) Available() bool {
	_, err := lookPath("asdf")
	return err == nil
}

func (Asdf) NormalizeID(id string, managers map[string]string) (string, bool) {
	return normalizeID("asdf", id, managers)
}

func (Asdf) PlanInstall(pkgName string) []string {
	spec, ok := parseAsdfID(pkgName)
	if !ok {
		return asdfInvalidCommand(pkgName)
	}
	if spec.kind == asdfPlugin {
		return []string{"asdf", "plugin", "add", spec.name}
	}
	return []string{"asdf", "install", spec.plugin, spec.version}
}

func (Asdf) PlanUninstall(pkgName string) []string {
	spec, ok := parseAsdfID(pkgName)
	if !ok {
		return asdfInvalidCommand(pkgName)
	}
	if spec.kind == asdfPlugin {
		return []string{"asdf", "plugin", "remove", spec.name}
	}
	return []string{"asdf", "uninstall", spec.plugin, spec.version}
}

func (Asdf) PlanUpgrade(pkgName string) []string {
	spec, ok := parseAsdfID(pkgName)
	if !ok {
		return asdfInvalidCommand(pkgName)
	}
	if spec.kind == asdfPlugin {
		// A plugin is not a version; the safe explicit action is re-adding it,
		// never `asdf plugin update --all`.
		return []string{"asdf", "plugin", "add", spec.name}
	}
	return []string{"asdf", "install", spec.plugin, spec.version}
}

func (Asdf) PlanClean() [][]string { return nil }

func (Asdf) Query(string) (bool, error) { return false, nil }

func (Asdf) ListInstalled() ([]string, error) { return nil, nil }

func (Asdf) QueryVersion(string) (string, error) { return "", nil }

type asdfKind int

const (
	asdfPlugin asdfKind = iota
	asdfTool
)

type asdfSpec struct {
	kind    asdfKind
	name    string // plugin name when kind == asdfPlugin
	plugin  string // plugin name when kind == asdfTool
	version string // tool version when kind == asdfTool
}

func parseAsdfID(id string) (asdfSpec, bool) {
	if name, ok := strings.CutPrefix(id, "plugin:"); ok {
		if name == "" {
			return asdfSpec{}, false
		}
		return asdfSpec{kind: asdfPlugin, name: name}, true
	}
	if rest, ok := strings.CutPrefix(id, "tool:"); ok {
		plugin, version, cut := strings.Cut(rest, "@")
		if !cut || plugin == "" || version == "" {
			return asdfSpec{}, false
		}
		return asdfSpec{kind: asdfTool, plugin: plugin, version: version}, true
	}
	return asdfSpec{}, false
}

func asdfInvalidCommand(pkgName string) []string {
	return []string{"sh", "-c", "printf '%s\n' 'genv: asdf requires plugin:<name> or tool:<plugin>@<version>' >&2; exit 1", "genv-asdf-invalid", pkgName}
}
