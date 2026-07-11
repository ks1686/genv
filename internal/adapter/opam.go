package adapter

import "strings"

// Opam manages OCaml packages inside an explicit switch. IDs are namespaced as
// "<switch>:<pkg>"; every operation is pinned to that switch so genv never
// mutates the current/default switch implicitly.
type Opam struct{}

func (Opam) Name() string { return "opam" }

func (Opam) Available() bool {
	_, err := lookPath("opam")
	return err == nil
}

func (Opam) NormalizeID(id string, managers map[string]string) (string, bool) {
	return normalizeID("opam", id, managers)
}

func (Opam) PlanInstall(pkgName string) []string {
	spec, ok := parseOpamID(pkgName)
	if !ok {
		return opamInvalidCommand(pkgName)
	}
	return []string{"opam", "install", "--switch", spec.switchName, "-y", spec.pkg}
}

func (Opam) PlanUninstall(pkgName string) []string {
	spec, ok := parseOpamID(pkgName)
	if !ok {
		return opamInvalidCommand(pkgName)
	}
	return []string{"opam", "remove", "--switch", spec.switchName, "-y", opamBasePackage(spec.pkg)}
}

func (Opam) PlanUpgrade(pkgName string) []string {
	spec, ok := parseOpamID(pkgName)
	if !ok {
		return opamInvalidCommand(pkgName)
	}
	return []string{"opam", "upgrade", "--switch", spec.switchName, "-y", opamBasePackage(spec.pkg)}
}

func (Opam) PlanClean() [][]string {
	return [][]string{{"opam", "clean"}}
}

func (o Opam) Query(pkgName string) (bool, error) {
	spec, ok := parseOpamID(pkgName)
	if !ok {
		return false, nil
	}
	installed, err := opamInstalledPackages(spec.switchName)
	if err != nil {
		return false, err
	}
	_, found := installed[opamBasePackage(spec.pkg)]
	return found, nil
}

func (Opam) ListInstalled() ([]string, error) { return nil, nil }

func (o Opam) QueryVersion(pkgName string) (string, error) {
	spec, ok := parseOpamID(pkgName)
	if !ok {
		return "", nil
	}
	installed, err := opamInstalledPackages(spec.switchName)
	if err != nil {
		return "", err
	}
	return installed[opamBasePackage(spec.pkg)], nil
}

type opamSpec struct {
	switchName string
	pkg        string
}

func parseOpamID(id string) (opamSpec, bool) {
	sw, pkg, ok := strings.Cut(id, ":")
	if !ok || sw == "" || pkg == "" {
		return opamSpec{}, false
	}
	return opamSpec{switchName: sw, pkg: pkg}, true
}

// opamBasePackage strips an opam version pin ("pkg.1.2.0" -> "pkg").
func opamBasePackage(pkg string) string {
	if before, _, ok := strings.Cut(pkg, "."); ok {
		return before
	}
	return pkg
}

// opamInstalledPackages parses `opam list --switch <switch> --installed --short
// --columns=name,version` output into a name->version map. The short columnar
// output is space-separated "name version" per line.
func opamInstalledPackages(switchName string) (map[string]string, error) {
	lines, err := runListOutput("opam", "list", "--switch", switchName, "--installed", "--short", "--columns=name,version")
	if err != nil {
		return nil, err
	}
	installed := make(map[string]string, len(lines))
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		version := ""
		if len(fields) > 1 {
			version = fields[1]
		}
		installed[fields[0]] = version
	}
	return installed, nil
}

func opamInvalidCommand(pkgName string) []string {
	return []string{"sh", "-c", "printf '%s\n' 'genv: opam requires a <switch>:<pkg> id so the target switch is explicit' >&2; exit 1", "genv-opam-invalid", pkgName}
}
