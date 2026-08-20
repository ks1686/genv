package adapter

// External tracks packages genv does not install (official installers,
// vendor downloads). Apply records them when the binary is on PATH.
type External struct{}

func (External) Name() string { return "external" }

func (External) Available() bool { return true }

func (External) TrackOnly() {}

func (External) NormalizeID(id string, managers map[string]string) (string, bool) {
	return normalizeID("external", id, managers)
}

func (External) PlanInstall(string) []string { return nil }

func (External) PlanUninstall(string) []string { return nil }

func (External) PlanUpgrade(string) []string { return nil }

func (External) PlanClean() [][]string { return nil }

func (External) Query(pkgName string) (bool, error) {
	_, err := lookPath(pkgName)
	return err == nil, nil
}

func (External) ListInstalled() ([]string, error) { return nil, nil }

func (External) QueryVersion(string) (string, error) { return "", nil }
