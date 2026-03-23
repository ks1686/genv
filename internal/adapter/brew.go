package adapter

import "strings"

// brewBase holds the Available and PlanInstall implementations shared between
// Brew and Linuxbrew. Both use the same brew binary; only their registry name
// and NormalizeID key differ.
type brewBase struct{}

func (brewBase) Available() bool {
	_, err := lookPath("brew")
	return err == nil
}

func (brewBase) PlanInstall(pkgName string) []string {
	return []string{"brew", "install", pkgName}
}

func (brewBase) PlanUninstall(pkgName string) []string {
	return []string{"brew", "uninstall", pkgName}
}

func (brewBase) PlanClean() [][]string {
	return [][]string{{"brew", "cleanup"}}
}

// Brew is the adapter for Homebrew (macOS and Linux).
type Brew struct{ brewBase }

func (Brew) Name() string { return "brew" }

func (Brew) NormalizeID(id string, managers map[string]string) (string, bool) {
	return normalizeID("brew", id, managers)
}

func (Brew) Query(pkgName string) (bool, error) {
	if ok, err := runQuery("brew", "list", "--formula", pkgName); ok || err != nil {
		return ok, err
	}
	return runQuery("brew", "list", "--cask", pkgName)
}

// ListInstalled returns both formulae and casks managed by Homebrew.
func (Brew) ListInstalled() ([]string, error) {
	formulae, err := runListOutput("brew", "list", "--formula", "--1")
	if err != nil {
		return nil, err
	}
	casks, err := runListOutput("brew", "list", "--cask", "--1")
	if err != nil {
		return nil, err
	}
	return append(formulae, casks...), nil
}

func (Brew) QueryVersion(pkgName string) (string, error) { return brewQueryVersion(pkgName) }

// Linuxbrew is the adapter for Homebrew on Linux (distinct manager ID so
// genv.json can target it explicitly, but uses the same brew binary).
type Linuxbrew struct{ brewBase }

func (Linuxbrew) Name() string { return "linuxbrew" }

func (Linuxbrew) NormalizeID(id string, managers map[string]string) (string, bool) {
	return normalizeID("linuxbrew", id, managers)
}

func (Linuxbrew) Query(pkgName string) (bool, error) {
	return runQuery("brew", "list", "--formula", pkgName)
}

func (Linuxbrew) ListInstalled() ([]string, error) {
	return runListOutput("brew", "list", "--formula", "--1")
}

func (Linuxbrew) QueryVersion(pkgName string) (string, error) { return brewQueryVersion(pkgName) }

// brewQueryVersion is the shared QueryVersion implementation for Brew and Linuxbrew.
// "brew list --versions pkgname" outputs "pkgname 1.0.0" or empty when not installed.
func brewQueryVersion(pkgName string) (string, error) {
	out, err := runVersionOutput("brew", "list", "--versions", pkgName)
	if err != nil || out == "" {
		return out, err
	}
	if parts := strings.SplitN(out, " ", 2); len(parts) == 2 {
		return parts[1], nil
	}
	return "", nil
}
