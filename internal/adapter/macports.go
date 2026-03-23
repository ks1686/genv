package adapter

import "strings"

// MacPorts is the adapter for MacPorts (macOS).
type MacPorts struct{}

func (MacPorts) Name() string { return "macports" }

func (MacPorts) Available() bool {
	_, err := lookPath("port")
	return err == nil
}

func (MacPorts) NormalizeID(id string, managers map[string]string) (string, bool) {
	return normalizeID("macports", id, managers)
}

func (MacPorts) PlanInstall(pkgName string) []string {
	return []string{"sudo", "port", "install", pkgName}
}

func (MacPorts) PlanUninstall(pkgName string) []string {
	return []string{"sudo", "port", "uninstall", pkgName}
}

func (MacPorts) PlanClean() [][]string {
	return [][]string{{"sudo", "port", "clean", "--all", "installed"}}
}

func (MacPorts) Query(pkgName string) (bool, error) { return runQuery("port", "installed", pkgName) }

// ListInstalled parses "port echo installed" which lists names with @version suffixes.
func (MacPorts) ListInstalled() ([]string, error) {
	lines, err := runListOutput("port", "echo", "installed")
	if err != nil {
		return nil, err
	}
	var names []string
	for _, line := range lines {
		// Lines look like "vim @9.0.0607_2+huge (active)"; strip from "@" onward.
		name := strings.SplitN(line, "@", 2)[0]
		name = strings.TrimSpace(name)
		if name != "" {
			names = append(names, name)
		}
	}
	return names, nil
}

func (MacPorts) QueryVersion(pkgName string) (string, error) {
	// "port installed pkgname" output: "  pkgname @version (active)"
	lines, err := runListOutput("port", "installed", pkgName)
	if err != nil {
		return "", err
	}
	for _, line := range lines {
		if !strings.Contains(line, "@") {
			continue
		}
		parts := strings.SplitN(line, "@", 2)
		if len(parts) != 2 {
			continue
		}
		ver := strings.TrimSpace(parts[1])
		ver = strings.TrimSuffix(ver, " (active)")
		ver = strings.TrimSpace(ver)
		// strip variant suffix like "+huge+perl"
		if idx := strings.Index(ver, "+"); idx > 0 {
			ver = ver[:idx]
		}
		return ver, nil
	}
	return "", nil
}
