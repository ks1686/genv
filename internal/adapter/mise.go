package adapter

import (
	"slices"
	"strings"
)

// Mise manages global tools via mise using an explicit "<tool>@<version>" id.
// It installs/pins globally with `mise use -g` and removes with `mise uninstall`.
// It never runs a broad `mise upgrade` of every tool.
type Mise struct{}

func (Mise) Name() string { return "mise" }

func (Mise) Available() bool {
	_, err := lookPath("mise")
	return err == nil
}

func (Mise) NormalizeID(id string, managers map[string]string) (string, bool) {
	return normalizeID("mise", id, managers)
}

func (Mise) PlanInstall(pkgName string) []string {
	spec, ok := parseMiseID(pkgName)
	if !ok {
		return miseInvalidCommand(pkgName)
	}
	return []string{"mise", "use", "-g", spec.tool + "@" + spec.version}
}

func (Mise) PlanUninstall(pkgName string) []string {
	spec, ok := parseMiseID(pkgName)
	if !ok {
		return miseInvalidCommand(pkgName)
	}
	return []string{"mise", "uninstall", spec.tool + "@" + spec.version}
}

func (Mise) PlanUpgrade(pkgName string) []string {
	spec, ok := parseMiseID(pkgName)
	if !ok {
		return miseInvalidCommand(pkgName)
	}
	return []string{"mise", "use", "-g", spec.tool + "@" + spec.version}
}

func (Mise) PlanClean() [][]string { return nil }

func (m Mise) Query(pkgName string) (bool, error) {
	spec, ok := parseMiseID(pkgName)
	if !ok {
		return false, nil
	}
	installed, err := miseInstalledVersions(spec.tool)
	if err != nil {
		return false, err
	}
	return slices.Contains(installed, spec.version), nil
}

func (Mise) ListInstalled() ([]string, error) { return nil, nil }

func (Mise) QueryVersion(pkgName string) (string, error) {
	spec, ok := parseMiseID(pkgName)
	if !ok {
		return "", nil
	}
	installed, err := miseInstalledVersions(spec.tool)
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

type miseSpec struct {
	tool    string
	version string
}

func parseMiseID(id string) (miseSpec, bool) {
	tool, version, ok := strings.Cut(id, "@")
	if !ok || tool == "" || version == "" {
		return miseSpec{}, false
	}
	return miseSpec{tool: tool, version: version}, true
}

// miseInstalledVersions parses `mise ls <tool> --installed` output. Each data
// row lists the tool then the version as the first two fields.
func miseInstalledVersions(tool string) ([]string, error) {
	lines, err := runListOutput("mise", "ls", tool, "--installed")
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

func miseInvalidCommand(pkgName string) []string {
	return []string{"sh", "-c", "printf '%s\n' 'genv: mise requires a <tool>@<version> id, e.g. node@22.11.0' >&2; exit 1", "genv-mise-invalid", pkgName}
}
