package adapter

import "strings"

// Helm manages Helm plugins only (helm plugin install/uninstall/update <name>).
// Helm repositories and project chart dependencies are intentionally out of
// scope. A tracked id is the plugin name; install requires the plugin source
// URL supplied via the managers.helm override formatted as "<name>=<url>".
type Helm struct{}

func (Helm) Name() string { return "helm" }

func (Helm) Available() bool {
	_, err := lookPath("helm")
	return err == nil
}

// NormalizeID combines the plugin id with a managers.helm URL override into the
// "<name>=<url>" form used by PlanInstall, mirroring the Deno convention.
func (Helm) NormalizeID(id string, managers map[string]string) (string, bool) {
	name, explicit := normalizeID("helm", id, managers)
	if !explicit {
		return name, false
	}
	if strings.Contains(name, "=") {
		return name, true
	}
	return id + "=" + name, true
}

func (Helm) PlanInstall(pkgName string) []string {
	spec, ok := parseHelmSpec(pkgName)
	if !ok {
		return helmInvalidCommand(pkgName)
	}
	return []string{"helm", "plugin", "install", spec.url}
}

func (Helm) PlanUninstall(pkgName string) []string {
	name, ok := helmPluginName(pkgName)
	if !ok {
		return helmInvalidCommand(pkgName)
	}
	return []string{"helm", "plugin", "uninstall", name}
}

// PlanUpgrade updates a single tracked plugin by name; it never updates all
// installed plugins.
func (Helm) PlanUpgrade(pkgName string) []string {
	name, ok := helmPluginName(pkgName)
	if !ok {
		return helmInvalidCommand(pkgName)
	}
	return []string{"helm", "plugin", "update", name}
}

func (Helm) PlanClean() [][]string { return nil }

func (h Helm) Query(pkgName string) (bool, error) {
	name, ok := helmPluginName(pkgName)
	if !ok {
		return false, nil
	}
	entries, err := h.listEntries()
	if err != nil {
		return false, err
	}
	_, found := entries[name]
	return found, nil
}

func (h Helm) ListInstalled() ([]string, error) {
	entries, err := h.listEntries()
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for name := range entries {
		names = append(names, name)
	}
	return names, nil
}

func (h Helm) QueryVersion(pkgName string) (string, error) {
	name, ok := helmPluginName(pkgName)
	if !ok {
		return "", nil
	}
	entries, err := h.listEntries()
	if err != nil {
		return "", err
	}
	return entries[name], nil
}

type helmSpec struct {
	name string
	url  string
}

func parseHelmSpec(pkgName string) (helmSpec, bool) {
	name, url, ok := strings.Cut(strings.TrimSpace(pkgName), "=")
	name = strings.TrimSpace(name)
	url = strings.TrimSpace(url)
	if !ok || name == "" || url == "" {
		return helmSpec{}, false
	}
	return helmSpec{name: name, url: url}, true
}

// helmPluginName extracts the tracked plugin name from either a bare name or a
// "<name>=<url>" install spec.
func helmPluginName(pkgName string) (string, bool) {
	if spec, ok := parseHelmSpec(pkgName); ok {
		return spec.name, true
	}
	name := strings.TrimSpace(pkgName)
	if name == "" || strings.ContainsAny(name, "= \t\n") {
		return "", false
	}
	return name, true
}

// listEntries parses `helm plugin list` output into a name->version map. The
// output is a header line "NAME\tVERSION\tDESCRIPTION" followed by tab/space
// separated rows.
func (Helm) listEntries() (map[string]string, error) {
	lines, err := runListOutput("helm", "plugin", "list")
	if err != nil {
		return nil, err
	}
	entries := make(map[string]string, len(lines))
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) < 2 || strings.EqualFold(fields[0], "NAME") {
			continue
		}
		entries[fields[0]] = fields[1]
	}
	return entries, nil
}

func helmInvalidCommand(pkgName string) []string {
	return []string{"sh", "-c", "printf '%s\n' 'genv: helm plugins need a name plus source url; use managers.helm with the url, or a name=url override' >&2; exit 1", "genv-helm-invalid", pkgName}
}
