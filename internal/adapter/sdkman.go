package adapter

import "strings"

// Sdkman manages SDKMAN! candidates by an explicit "<candidate>:<version>" id.
// It never runs `sdk selfupdate` or broad upgrades; each operation targets one
// tracked candidate version.
type Sdkman struct{}

func (Sdkman) Name() string { return "sdkman" }

func (Sdkman) Available() bool {
	_, err := lookPath("sdk")
	return err == nil
}

func (Sdkman) NormalizeID(id string, managers map[string]string) (string, bool) {
	return normalizeID("sdkman", id, managers)
}

func (Sdkman) PlanInstall(pkgName string) []string {
	spec, ok := parseSdkmanID(pkgName)
	if !ok {
		return sdkmanInvalidCommand(pkgName)
	}
	return []string{"sdk", "install", spec.candidate, spec.version}
}

func (Sdkman) PlanUninstall(pkgName string) []string {
	spec, ok := parseSdkmanID(pkgName)
	if !ok {
		return sdkmanInvalidCommand(pkgName)
	}
	return []string{"sdk", "uninstall", spec.candidate, spec.version}
}

func (Sdkman) PlanUpgrade(pkgName string) []string {
	spec, ok := parseSdkmanID(pkgName)
	if !ok {
		return sdkmanInvalidCommand(pkgName)
	}
	return []string{"sdk", "install", spec.candidate, spec.version}
}

func (Sdkman) PlanClean() [][]string {
	return [][]string{{"sdk", "flush", "temp"}}
}

func (Sdkman) Query(pkgName string) (bool, error) { return queryUnsupported("sdkman", pkgName) }

func (Sdkman) ListInstalled() ([]string, error) { return nil, nil }

func (Sdkman) QueryVersion(string) (string, error) { return "", nil }

type sdkmanSpec struct {
	candidate string
	version   string
}

func parseSdkmanID(id string) (sdkmanSpec, bool) {
	candidate, version, ok := strings.Cut(id, ":")
	if !ok || candidate == "" || version == "" {
		return sdkmanSpec{}, false
	}
	return sdkmanSpec{candidate: candidate, version: version}, true
}

func sdkmanInvalidCommand(pkgName string) []string {
	return []string{"sh", "-c", "printf '%s\n' 'genv: sdkman requires a <candidate>:<version> id, e.g. java:21.0.2-tem' >&2; exit 1", "genv-sdkman-invalid", pkgName}
}
