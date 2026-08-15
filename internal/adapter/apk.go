package adapter

import (
	"context"
	"os/exec"
	"strings"
)

// Apk is the adapter for Alpine Linux apk.
type Apk struct{}

func (Apk) Name() string { return "apk" }

func (Apk) Available() bool {
	_, err := lookPath("apk")
	return err == nil
}

func (Apk) NormalizeID(id string, managers map[string]string) (string, bool) {
	return normalizeID("apk", id, managers)
}

func (Apk) PlanInstall(pkgName string) []string {
	return []string{"sudo", "apk", "add", pkgName}
}

func (Apk) PlanUninstall(pkgName string) []string {
	return []string{"sudo", "apk", "del", pkgName}
}

func (Apk) PlanUpgrade(pkgName string) []string {
	return []string{"sudo", "apk", "upgrade", pkgName}
}

func (Apk) PlanUpgradeBatch(pkgNames []string) []string {
	args := []string{"sudo", "apk", "upgrade"}
	return append(args, pkgNames...)
}

func (Apk) PlanClean() [][]string {
	return [][]string{{"sudo", "apk", "cache", "clean"}}
}

func (Apk) Query(pkgName string) (bool, error) {
	return runQuery("apk", "info", "-e", pkgName)
}

func (Apk) Search(query string) ([]string, error) {
	return Apk{}.SearchContext(context.Background(), query)
}

func (Apk) SearchContext(ctx context.Context, query string) ([]string, error) {
	lines, err := runListOutputContext(ctx, "apk", "search", "-q", query)
	if err != nil || len(lines) == 0 {
		return lines, err
	}
	var names []string
	for _, line := range lines {
		name := strings.TrimSpace(line)
		if name == "" {
			continue
		}
		// apk search may print name-version; strip the last -<version> segment when present.
		if i := strings.LastIndex(name, "-"); i > 0 {
			if looksLikeApkVersion(name[i+1:]) {
				name = name[:i]
			}
		}
		if containsFold(name, query) {
			names = append(names, name)
		}
	}
	return names, nil
}

func looksLikeApkVersion(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r >= '0' && r <= '9' {
			return true
		}
	}
	return false
}

func (Apk) ListNames() ([]string, error) {
	return Apk{}.ListNamesContext(context.Background())
}

func (Apk) ListNamesContext(ctx context.Context) ([]string, error) {
	return runListOutputContext(ctx, "apk", "search", "-q")
}

func (Apk) ListInstalled() ([]string, error) {
	return runListOutput("apk", "info")
}

func (Apk) ListInstalledVersions() (map[string]string, error) {
	out, err := exec.Command("apk", "info", "-v").Output()
	if err != nil {
		return nil, err
	}
	return parseApkInfoVersions(trimmedNonEmptyLines(string(out))), nil
}

func parseApkInfoVersions(lines []string) map[string]string {
	versions := make(map[string]string, len(lines))
	for _, line := range lines {
		name, ver := splitApkNameVersion(strings.TrimSpace(line))
		if name != "" && ver != "" {
			versions[name] = ver
		}
	}
	return versions
}

func splitApkNameVersion(s string) (name, ver string) {
	i := strings.LastIndex(s, "-")
	if i <= 0 {
		return s, ""
	}
	return s[:i], s[i+1:]
}

func (Apk) QueryVersion(pkgName string) (string, error) {
	out, err := runVersionOutput("apk", "info", "-v", pkgName)
	if err != nil || out == "" {
		return "", err
	}
	_, ver := splitApkNameVersion(strings.TrimSpace(out))
	return ver, nil
}

func (Apk) ListOutdated(pkgNames []string) (map[string]string, error) {
	out, err := exec.Command("apk", "version", "-l", "<").Output()
	if err != nil {
		return nil, err
	}
	return intersectNameMap(parseApkVersionOutdated(trimmedNonEmptyLines(string(out))), pkgNames), nil
}

func parseApkVersionOutdated(lines []string) map[string]string {
	out := map[string]string{}
	for _, line := range lines {
		// "pkg-1.0-r0 < pkg-1.1-r0"
		parts := strings.Split(line, "<")
		if len(parts) != 2 {
			continue
		}
		name, _ := splitApkNameVersion(strings.TrimSpace(parts[0]))
		_, latest := splitApkNameVersion(strings.TrimSpace(parts[1]))
		if name != "" {
			if latest == "" {
				latest = "outdated"
			}
			out[name] = latest
		}
	}
	return out
}
