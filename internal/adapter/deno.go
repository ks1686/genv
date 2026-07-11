package adapter

import "strings"

// Deno manages named globally installed Deno scripts. Because Adapter planning
// receives one package string, genv encodes Deno install specs as name=url.
// NormalizeID accepts managers.deno as the URL and combines it with the package id.
type Deno struct{}

func (Deno) Name() string { return "deno" }

func (Deno) Available() bool {
	_, err := lookPath("deno")
	return err == nil
}

func (Deno) NormalizeID(id string, managers map[string]string) (string, bool) {
	name, explicit := normalizeID("deno", id, managers)
	if !explicit {
		return name, false
	}
	if strings.Contains(name, "=") {
		return name, true
	}
	return id + "=" + name, true
}

func (Deno) PlanInstall(pkgName string) []string {
	spec, ok := parseDenoToolSpec(pkgName)
	if !ok {
		return inertDenoCommand(pkgName)
	}
	return []string{"deno", "install", "--global", "--name", spec.name, spec.url}
}

func (Deno) PlanUninstall(pkgName string) []string {
	name, ok := denoToolName(pkgName)
	if !ok {
		return inertDenoCommand(pkgName)
	}
	return []string{"deno", "uninstall", "--global", name}
}

func (d Deno) PlanUpgrade(pkgName string) []string {
	return d.PlanInstall(pkgName)
}

func (Deno) PlanClean() [][]string { return nil }

func (d Deno) Query(pkgName string) (bool, error) {
	name, ok := denoToolName(pkgName)
	if !ok {
		return false, nil
	}
	entries, err := d.listEntries()
	if err != nil {
		return false, err
	}
	for _, entry := range entries {
		if entry.name == name {
			return true, nil
		}
	}
	return false, nil
}

func (d Deno) ListInstalled() ([]string, error) {
	entries, err := d.listEntries()
	if err != nil {
		return nil, err
	}
	return entriesNames(entries), nil
}

func (Deno) QueryVersion(string) (string, error) { return "", nil }

func (Deno) listEntries() ([]jsPackageEntry, error) {
	lines, err := runListOutput("deno", "install", "--global", "--list")
	if err != nil {
		return nil, err
	}
	return parseDenoInstallListEntries(lines), nil
}

type denoToolSpec struct {
	name string
	url  string
}

func parseDenoToolSpec(pkgName string) (denoToolSpec, bool) {
	name, url, ok := strings.Cut(strings.TrimSpace(pkgName), "=")
	name = strings.TrimSpace(name)
	url = strings.TrimSpace(url)
	if !ok || !validDenoToolName(name) || !validDenoURL(url) {
		return denoToolSpec{}, false
	}
	return denoToolSpec{name: name, url: url}, true
}

func denoToolName(pkgName string) (string, bool) {
	if spec, ok := parseDenoToolSpec(pkgName); ok {
		return spec.name, true
	}
	name := strings.TrimSpace(pkgName)
	if !validDenoToolName(name) || strings.Contains(name, "://") {
		return "", false
	}
	return name, true
}

func validDenoToolName(name string) bool {
	if name == "" || strings.ContainsAny(name, "= /\\\t\n\r") {
		return false
	}
	return name != "." && name != ".."
}

func validDenoURL(url string) bool {
	return strings.HasPrefix(url, "https://") || strings.HasPrefix(url, "http://") || strings.HasPrefix(url, "jsr:") || strings.HasPrefix(url, "npm:")
}

func parseDenoInstallListEntries(lines []string) []jsPackageEntry {
	entries := make([]jsPackageEntry, 0, len(lines))
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) > 0 && validDenoToolName(fields[0]) {
			entries = append(entries, jsPackageEntry{name: fields[0]})
		}
	}
	return entries
}

func inertDenoCommand(pkgName string) []string {
	return []string{"sh", "-c", "printf '%s\n' 'genv: deno packages require a manager override formatted as command=https://module-url or managers.deno URL with package id as command name' >&2; exit 1", "genv-deno-invalid-spec", pkgName}
}
