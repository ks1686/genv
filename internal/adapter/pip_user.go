package adapter

import (
	"errors"
	"os/exec"
	"strings"
)

// PipUser is the adapter for pip --user installs.
type PipUser struct{}

func (PipUser) Name() string { return "pip-user" }

func (PipUser) Available() bool {
	if _, err := lookPath("python3"); err != nil {
		return false
	}
	return pipUserProbe() == nil
}

var pipUserProbe = func() error {
	// Bounded probe: an ExitError (pip missing) and a timeout both mean
	// "not usable", so the raw error is enough here.
	_, err := runProbe("python3", "-m", "pip", "--version")
	return err
}

func (PipUser) NormalizeID(id string, managers map[string]string) (string, bool) {
	return normalizeID("pip-user", id, managers)
}

func (PipUser) PlanInstall(pkgName string) []string {
	return []string{"python3", "-m", "pip", "install", "--user", pkgName}
}

func (PipUser) PlanUninstall(pkgName string) []string {
	return []string{"python3", "-m", "pip", "uninstall", "-y", PythonBasePackageName(pkgName)}
}

func (PipUser) PlanUpgrade(pkgName string) []string {
	return []string{"python3", "-m", "pip", "install", "--user", "--upgrade", pkgName}
}

func (PipUser) PlanClean() [][]string {
	return [][]string{{"python3", "-m", "pip", "cache", "purge"}}
}

func (PipUser) Query(pkgName string) (bool, error) {
	name := PythonBasePackageName(pkgName)
	entries, err := PipUser{}.listEntries()
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

func (PipUser) ListInstalled() ([]string, error) {
	entries, err := PipUser{}.listEntries()
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.name)
	}
	return names, nil
}

// ListForScan returns user-site packages that are not dependencies of other
// installed packages (`pip list --user --not-required`), minus installer and
// stdlib-like noise (pip, setuptools, certifi, …). Leftover transitives of
// tools already tracked via uv often show up as orphans; the skip set drops
// the usual ones. Pass scan --all to adopt the full user-site list.
func (PipUser) ListForScan() ([]string, error) {
	out, err := runProbe("python3", "-m", "pip", "list", "--user", "--not-required", "--format=json")
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return nil, nil
		}
		return nil, err
	}
	entries, err := parsePipListJSON(out)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if pipUserScanSkip[pythonScanKey(entry.name)] {
			continue
		}
		names = append(names, entry.name)
	}
	return names, nil
}

// pipUserScanSkip is installer tooling plus common network/stdlib-like
// transitives that show up as pip --user orphans after a tool moves to uv.
var pipUserScanSkip = map[string]bool{
	"certifi":            true,
	"charset-normalizer": true,
	"colorama":           true,
	"exceptiongroup":     true,
	"idna":               true,
	"importlib-metadata": true,
	"packaging":          true,
	"pip":                true,
	"pyparsing":          true,
	"setuptools":         true,
	"six":                true,
	"tomli":              true,
	"typing-extensions":  true,
	"urllib3":            true,
	"wheel":              true,
	"zipp":               true,
}

func pythonScanKey(name string) string {
	return strings.ReplaceAll(strings.ToLower(strings.TrimSpace(name)), "_", "-")
}

func (PipUser) QueryVersion(pkgName string) (string, error) {
	name := PythonBasePackageName(pkgName)
	entries, err := PipUser{}.listEntries()
	if err != nil {
		return "", err
	}
	for _, entry := range entries {
		if entry.name == name {
			return entry.version, nil
		}
	}
	return "", nil
}

func (PipUser) ListInstalledVersions() (map[string]string, error) {
	entries, err := PipUser{}.listEntries()
	if err != nil {
		return nil, err
	}
	versions := make(map[string]string, len(entries))
	for _, entry := range entries {
		versions[entry.name] = entry.version
	}
	return versions, nil
}

func (PipUser) ListOutdated(pkgNames []string) (map[string]string, error) {
	entries, err := PipUser{}.listEntries()
	if err != nil {
		return nil, err
	}
	return listRegistryOutdated(versionMapOf(entries, func(e pythonEntry) (string, string) {
		return e.name, e.version
	}), pkgNames, PythonBasePackageName, pypiLatestVersion)
}

func (PipUser) listEntries() ([]pythonEntry, error) {
	out, err := runProbe("python3", "-m", "pip", "list", "--user", "--format=json")
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return nil, nil
		}
		return nil, err
	}
	return parsePipListJSON(out)
}
