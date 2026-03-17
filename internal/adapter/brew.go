package adapter

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

// Brew is the adapter for Homebrew (macOS and Linux).
type Brew struct{ brewBase }

func (Brew) Name() string { return "brew" }

func (Brew) NormalizeID(id string, managers map[string]string) (string, bool) {
	return normalizeID("brew", id, managers)
}

func (Brew) Query(pkgName string) (bool, error) {
	return runQuery("brew", "list", "--formula", pkgName)
}

// Linuxbrew is the adapter for Homebrew on Linux (distinct manager ID so
// gpm.json can target it explicitly, but uses the same brew binary).
type Linuxbrew struct{ brewBase }

func (Linuxbrew) Name() string { return "linuxbrew" }

func (Linuxbrew) NormalizeID(id string, managers map[string]string) (string, bool) {
	return normalizeID("linuxbrew", id, managers)
}

func (Linuxbrew) Query(pkgName string) (bool, error) {
	return runQuery("brew", "list", "--formula", pkgName)
}
