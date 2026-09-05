package adapter

import (
	"context"
	"strings"
)

// Apt is the adapter for Debian/Ubuntu apt-get (and dpkg queries).
type Apt struct{}

func (Apt) Name() string { return "apt" }

func (Apt) Available() bool {
	if _, err := lookPath("apt-get"); err == nil {
		return true
	}
	_, err := lookPath("apt")
	return err == nil
}

func (Apt) NormalizeID(id string, managers map[string]string) (string, bool) {
	return normalizeID("apt", id, managers)
}

func (Apt) PlanInstall(pkgName string) []string {
	return []string{"sudo", "apt-get", "install", "-y", pkgName}
}

func (Apt) PlanUninstall(pkgName string) []string {
	return []string{"sudo", "apt-get", "remove", "-y", pkgName}
}

func (Apt) PlanUpgrade(pkgName string) []string {
	return []string{"sudo", "apt-get", "install", "--only-upgrade", "-y", pkgName}
}

func (Apt) PlanUpgradeBatch(pkgNames []string) []string {
	args := []string{"sudo", "apt-get", "install", "--only-upgrade", "-y"}
	return append(args, pkgNames...)
}

func (Apt) PlanRefresh() []string {
	return []string{"sudo", "apt-get", "update"}
}

func (Apt) PlanClean() [][]string {
	return [][]string{{"sudo", "apt-get", "clean"}}
}

func (Apt) Query(pkgName string) (bool, error) {
	return runQuery("dpkg-query", "-W", pkgName)
}

func (Apt) Search(query string) ([]string, error) {
	return Apt{}.SearchContext(context.Background(), query)
}

func (Apt) SearchContext(ctx context.Context, query string) ([]string, error) {
	lines, err := runListOutputContext(ctx, "apt-cache", "search", "--names-only", query)
	if err != nil || len(lines) == 0 {
		return lines, err
	}
	var names []string
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		name := fields[0]
		if containsFold(name, query) {
			names = append(names, name)
		}
	}
	return names, nil
}

func (Apt) ListNames() ([]string, error) {
	return Apt{}.ListNamesContext(context.Background())
}

func (Apt) ListNamesContext(ctx context.Context) ([]string, error) {
	return runListOutputContext(ctx, "apt-cache", "pkgnames")
}

func (Apt) ListInstalled() ([]string, error) {
	return runListOutput("dpkg-query", "-W", "-f", "${Package}\n")
}

func (Apt) ListInstalledVersions() (map[string]string, error) {
	lines, err := runListOutput("dpkg-query", "-W", "-f", "${Package} ${Version}\n")
	if err != nil {
		return nil, err
	}
	versions := make(map[string]string, len(lines))
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) >= 2 {
			versions[fields[0]] = fields[1]
		}
	}
	return versions, nil
}

func (Apt) QueryVersion(pkgName string) (string, error) {
	out, err := runVersionOutput("dpkg-query", "-W", "-f", "${Version}", pkgName)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

func (Apt) ListOutdated(pkgNames []string) (map[string]string, error) {
	out, err := runProbe("apt-get", "-s", "upgrade")
	if err != nil {
		return nil, err
	}
	return intersectNameMap(parseAptSimulatedUpgrade(trimmedNonEmptyLines(string(out))), pkgNames), nil
}

// parseAptSimulatedUpgrade extracts "Inst pkg [...] (newver ..." lines from
// `apt-get -s upgrade`.
func parseAptSimulatedUpgrade(lines []string) map[string]string {
	out := map[string]string{}
	for _, line := range lines {
		if !strings.HasPrefix(line, "Inst ") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		name := fields[1]
		latest := "outdated"
		if i := strings.Index(line, "("); i >= 0 {
			rest := strings.TrimPrefix(line[i+1:], "")
			if ver := strings.Fields(rest); len(ver) > 0 {
				latest = strings.TrimSuffix(ver[0], ")")
			}
		}
		out[name] = latest
	}
	return out
}
