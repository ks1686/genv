package adapter

import (
	"context"
	"errors"
	"os/exec"
	"strings"
)

// Dnf is the adapter for Fedora / RHEL-like dnf (rpm queries).
type Dnf struct{}

func (Dnf) Name() string { return "dnf" }

func (Dnf) Available() bool {
	_, err := lookPath("dnf")
	return err == nil
}

func (Dnf) NormalizeID(id string, managers map[string]string) (string, bool) {
	return normalizeID("dnf", id, managers)
}

func (Dnf) PlanInstall(pkgName string) []string {
	return []string{"sudo", "dnf", "install", "-y", pkgName}
}

func (Dnf) PlanUninstall(pkgName string) []string {
	return []string{"sudo", "dnf", "remove", "-y", pkgName}
}

func (Dnf) PlanUpgrade(pkgName string) []string {
	return []string{"sudo", "dnf", "upgrade", "-y", pkgName}
}

func (Dnf) PlanUpgradeBatch(pkgNames []string) []string {
	args := []string{"sudo", "dnf", "upgrade", "-y"}
	return append(args, pkgNames...)
}

func (Dnf) PlanRefresh() []string {
	return []string{"sudo", "dnf", "makecache"}
}

func (Dnf) PlanClean() [][]string {
	return [][]string{{"sudo", "dnf", "clean", "all"}}
}

func (Dnf) Query(pkgName string) (bool, error) {
	return runQuery("rpm", "-q", pkgName)
}

func (Dnf) Search(query string) ([]string, error) {
	return Dnf{}.SearchContext(context.Background(), query)
}

func (Dnf) SearchContext(ctx context.Context, query string) ([]string, error) {
	lines, err := runListOutputContext(ctx, "dnf", "repoquery", "-q", "--qf", "%{name}", query+"*")
	if err != nil || len(lines) == 0 {
		return lines, err
	}
	var names []string
	seen := map[string]bool{}
	for _, line := range lines {
		name := strings.TrimSpace(line)
		if name == "" || seen[name] || !containsFold(name, query) {
			continue
		}
		seen[name] = true
		names = append(names, name)
	}
	return names, nil
}

func (Dnf) ListNames() ([]string, error) {
	return Dnf{}.ListNamesContext(context.Background())
}

func (Dnf) ListNamesContext(ctx context.Context) ([]string, error) {
	return runListOutputContext(ctx, "dnf", "repoquery", "-q", "--qf", "%{name}")
}

func (Dnf) ListInstalled() ([]string, error) {
	return runListOutput("rpm", "-qa", "--qf", "%{NAME}\n")
}

func (Dnf) ListInstalledVersions() (map[string]string, error) {
	lines, err := runListOutput("rpm", "-qa", "--qf", "%{NAME} %{VERSION}-%{RELEASE}\n")
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

func (Dnf) QueryVersion(pkgName string) (string, error) {
	out, err := runVersionOutput("rpm", "-q", "--qf", "%{VERSION}-%{RELEASE}", pkgName)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

func (Dnf) ListOutdated(pkgNames []string) (map[string]string, error) {
	out, err := runProbeCombined("dnf", "check-update", "-q")
	if err != nil {
		var exitErr *exec.ExitError
		// dnf check-update exits 100 when updates are available.
		if !errors.As(err, &exitErr) || exitErr.ExitCode() != 100 {
			return nil, err
		}
	}
	return intersectNameMap(parseDnfCheckUpdate(trimmedNonEmptyLines(string(out))), pkgNames), nil
}

func parseDnfCheckUpdate(lines []string) map[string]string {
	out := map[string]string{}
	for _, line := range lines {
		if strings.HasPrefix(line, "Last metadata") || strings.HasPrefix(line, "Security:") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		name := fields[0]
		if i := strings.Index(name, "."); i > 0 {
			name = name[:i]
		}
		out[name] = fields[1]
	}
	return out
}
