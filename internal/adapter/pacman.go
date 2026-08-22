package adapter

import (
	"context"
	"errors"
	"os/exec"
	"strings"
)

// Pacman is the adapter for pacman, the Arch Linux official-repository package
// manager. It supports only packages from Arch official repositories; AUR
// packages are handled by the Paru and Yay adapters.
//
// Unlike paru/yay (which self-elevate), pacman has no built-in privilege
// escalation, so mutating commands are prefixed with sudo — the same
// approach used by Snap.
type Pacman struct{}

func (Pacman) Name() string { return "pacman" }

func (Pacman) Available() bool {
	_, err := lookPath("pacman")
	return err == nil
}

func (Pacman) NormalizeID(id string, managers map[string]string) (string, bool) {
	return normalizeID("pacman", id, managers)
}

func (Pacman) PlanInstall(pkgName string) []string {
	return []string{"sudo", "pacman", "-S", "--needed", "--noconfirm", pkgName}
}

func (Pacman) PlanUninstall(pkgName string) []string {
	return []string{"sudo", "pacman", "-Rs", "--noconfirm", pkgName}
}

// PlanUpgrade reuses PlanInstall: pacman -S upgrades to the latest version.
func (Pacman) PlanUpgrade(pkgName string) []string {
	return []string{"sudo", "pacman", "-S", "--needed", "--noconfirm", pkgName}
}

// PlanUpgradeBatch upgrades multiple packages in one pacman invocation,
// avoiding repeated database syncs and privilege elevations.
func (Pacman) PlanUpgradeBatch(pkgNames []string) []string {
	args := []string{"sudo", "pacman", "-S", "--needed", "--noconfirm"}
	return append(args, pkgNames...)
}

func (Pacman) PlanClean() [][]string {
	return [][]string{{"sudo", "pacman", "-Sc", "--noconfirm"}}
}

func (Pacman) Query(pkgName string) (bool, error) { return runQuery("pacman", "-Qi", pkgName) }

// Search returns package names from official pacman repos whose name contains query.
func (Pacman) Search(query string) ([]string, error) {
	return Pacman{}.SearchContext(context.Background(), query)
}

func (Pacman) SearchContext(ctx context.Context, query string) ([]string, error) {
	lines, err := runListOutputContext(ctx, "pacman", "-Ss", query)
	if err != nil || len(lines) == 0 {
		return lines, err
	}
	return parsePacmanSearch(lines, query), nil
}

// ListNames returns all installable packages from official pacman repos.
func (Pacman) ListNames() ([]string, error) {
	return Pacman{}.ListNamesContext(context.Background())
}

func (Pacman) ListNamesContext(ctx context.Context) ([]string, error) {
	return runListOutputContext(ctx, "pacman", "-Slq")
}

func (Pacman) ListInstalled() ([]string, error) {
	return runListOutput("pacman", "-Qq")
}

// ListInstalledVersions returns the installed version of every package in the
// pacman database. This satisfies the optional VersionLister interface so the
// resolver can refresh lock versions with one command after a batch upgrade.
func (Pacman) ListInstalledVersions() (map[string]string, error) {
	lines, err := runListOutput("pacman", "-Q")
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

func (Pacman) QueryVersion(pkgName string) (string, error) {
	out, err := runVersionOutput("pacman", "-Q", pkgName)
	if err != nil || out == "" {
		return out, err
	}
	return parseMgrQueryVersion(out), nil
}

// ListOutdated reports installed pacman packages with a newer version in the
// repos, keyed by package name -> target version, intersected with pkgNames.
func (Pacman) ListOutdated(pkgNames []string) (map[string]string, error) {
	return listPacmanQuOutdated("pacman", pkgNames)
}

// parsePacmanQuLines parses `pacman -Qu` / `paru -Qu` / `yay -Qu` lines such as
// "git 2.47.0 -> 2.48.0" or "somepkg 1.2.3".
func parsePacmanQuLines(lines []string) map[string]string {
	out := map[string]string{}
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		name := fields[0]
		latest := "outdated"
		if len(fields) >= 2 {
			latest = fields[len(fields)-1]
		}
		out[name] = latest
	}
	return out
}

func listPacmanQuOutdated(cmd string, pkgNames []string) (map[string]string, error) {
	lines, err := runPacmanQuOutput(cmd)
	if err != nil {
		return nil, err
	}
	return intersectNameMap(parsePacmanQuLines(lines), pkgNames), nil
}

// runPacmanQuOutput runs cmd -Qu. Exit code 1 means the system is up to date.
func runPacmanQuOutput(cmd string) ([]string, error) {
	out, err := runProbe(cmd, "-Qu")
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
			return nil, nil
		}
		return nil, err
	}
	return trimmedNonEmptyLines(string(out)), nil
}
