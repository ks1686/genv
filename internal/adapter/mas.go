package adapter

import (
	"slices"
	"strings"
)

// Mas is the adapter for the Mac App Store, driven by the `mas` CLI. It is only
// available on macOS (the mas binary exists nowhere else).
//
// App Store apps are identified by their numeric product ID rather than a
// human-readable name, so friendly genv IDs map to those numbers through the
// "managers" field, e.g.
//
//	{"id": "xcode", "managers": {"mas": "497799835"}}
type Mas struct{}

func (Mas) Name() string { return "mas" }

func (Mas) Available() bool {
	_, err := lookPath("mas")
	return err == nil
}

func (Mas) NormalizeID(id string, managers map[string]string) (string, bool) {
	return normalizeID("mas", id, managers)
}

func (Mas) PlanInstall(pkgName string) []string {
	return []string{"mas", "install", pkgName}
}

// PlanUninstall removes the app bundle from /Applications, which requires root,
// so the command is prefixed with sudo — the same approach used by Pacman and
// Snap.
func (Mas) PlanUninstall(pkgName string) []string {
	return []string{"sudo", "mas", "uninstall", pkgName}
}

func (Mas) PlanUpgrade(pkgName string) []string {
	return []string{"mas", "upgrade", pkgName}
}

// PlanUpgradeBatch upgrades multiple App Store apps in one mas invocation.
func (Mas) PlanUpgradeBatch(pkgNames []string) []string {
	args := []string{"mas", "upgrade"}
	return append(args, pkgNames...)
}

// PlanClean is a no-op: mas keeps no local cache to purge.
func (Mas) PlanClean() [][]string { return nil }

func (Mas) Search(query string) ([]string, error) {
	apps, err := masSearchApps(query)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(apps))
	for _, app := range apps {
		ids = append(ids, app.id)
	}
	return ids, nil
}

func (Mas) CompletionNames(prefix string) ([]string, error) {
	apps, err := masSearchApps(prefix)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(apps))
	for _, app := range apps {
		names = append(names, strings.ToLower(strings.ReplaceAll(app.name, " ", "-")))
	}
	return names, nil
}

func (Mas) Query(pkgName string) (bool, error) {
	ids, err := masListIDs()
	if err != nil {
		return false, err
	}
	return slices.Contains(ids, pkgName), nil
}

// ListInstalled returns the numeric product IDs of every installed App Store app.
func (Mas) ListInstalled() ([]string, error) {
	return masListIDs()
}

// QueryVersion returns the installed version for the app with product ID pkgName.
// "mas list" prints one app per line as "<id>  <name>  (<version>)"; the version
// is the parenthesized final field.
func (Mas) QueryVersion(pkgName string) (string, error) {
	lines, err := runListOutput("mas", "list")
	if err != nil {
		return "", err
	}
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[0] == pkgName {
			return strings.Trim(fields[len(fields)-1], "()"), nil
		}
	}
	return "", nil
}

// ListInstalledVersions returns product ID -> version for every installed app,
// satisfying the optional VersionLister interface. "mas list" already reports
// each app's version, so this avoids a separate QueryVersion call per app.
func (Mas) ListInstalledVersions() (map[string]string, error) {
	lines, err := runListOutput("mas", "list")
	if err != nil {
		return nil, err
	}
	versions := make(map[string]string, len(lines))
	for _, line := range lines {
		if fields := strings.Fields(line); len(fields) >= 2 {
			versions[fields[0]] = strings.Trim(fields[len(fields)-1], "()")
		}
	}
	return versions, nil
}

// ListOutdated reports installed App Store apps with an update available, keyed
// by numeric product ID -> target version, intersected with pkgNames. `mas
// outdated` prints one app per line as "497799835 Xcode (14.0 -> 14.1)" or
// "497799835 Xcode (14.1)".
func (Mas) ListOutdated(pkgNames []string) (map[string]string, error) {
	lines, err := runListOutput("mas", "outdated")
	if err != nil {
		return nil, err
	}
	want := make(map[string]bool, len(pkgNames))
	for _, n := range pkgNames {
		want[n] = true
	}
	outdated := make(map[string]string)
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		id := fields[0]
		if len(pkgNames) > 0 && !want[id] {
			continue
		}
		outdated[id] = masOutdatedTarget(line)
	}
	if len(outdated) == 0 {
		return nil, nil
	}
	return outdated, nil
}

// masOutdatedTarget extracts the target version from a `mas outdated` line: the
// version after "->" when present, otherwise the parenthesized version, or a
// non-empty sentinel when neither parses (the value is only for display —
// presence in the map is what marks the app outdated).
func masOutdatedTarget(line string) string {
	open := strings.LastIndex(line, "(")
	closeIdx := strings.LastIndex(line, ")")
	if open >= 0 && closeIdx > open {
		inner := strings.TrimSpace(line[open+1 : closeIdx])
		if _, after, ok := strings.Cut(inner, "->"); ok {
			inner = strings.TrimSpace(after)
		}
		if inner != "" {
			return inner
		}
	}
	return "outdated"
}

type masSearchApp struct {
	id   string
	name string
}

func masSearchApps(query string) ([]masSearchApp, error) {
	lines, err := runListOutput("mas", "search", query)
	if err != nil {
		return nil, err
	}
	query = strings.ToLower(query)
	var apps []masSearchApp
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		name := strings.Join(fields[1:len(fields)-1], " ")
		if strings.Contains(strings.ToLower(name), query) {
			apps = append(apps, masSearchApp{id: fields[0], name: name})
		}
	}
	return apps, nil
}

// masListIDs runs "mas list" and returns the leading numeric product ID of each
// line. An unavailable mas or an empty store yields nil.
func masListIDs() ([]string, error) {
	lines, err := runListOutput("mas", "list")
	if err != nil {
		return nil, err
	}
	var ids []string
	for _, line := range lines {
		if fields := strings.Fields(line); len(fields) > 0 {
			ids = append(ids, fields[0])
		}
	}
	return ids, nil
}
