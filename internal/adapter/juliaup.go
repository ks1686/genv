package adapter

import (
	"slices"
	"strings"
)

// Juliaup manages Julia channels/versions via juliaup. A tracked id is a
// channel name (e.g. "release", "lts", "1.10"). It never manages Julia project
// packages.
type Juliaup struct{}

func (Juliaup) Name() string { return "juliaup" }

func (Juliaup) Available() bool {
	_, err := lookPath("juliaup")
	return err == nil
}

func (Juliaup) NormalizeID(id string, managers map[string]string) (string, bool) {
	return normalizeID("juliaup", id, managers)
}

func (Juliaup) PlanInstall(pkgName string) []string {
	channel, ok := juliaupChannel(pkgName)
	if !ok {
		return juliaupInvalidCommand(pkgName)
	}
	return []string{"juliaup", "add", channel}
}

func (Juliaup) PlanUninstall(pkgName string) []string {
	channel, ok := juliaupChannel(pkgName)
	if !ok {
		return juliaupInvalidCommand(pkgName)
	}
	return []string{"juliaup", "remove", channel}
}

func (Juliaup) PlanUpgrade(pkgName string) []string {
	channel, ok := juliaupChannel(pkgName)
	if !ok {
		return juliaupInvalidCommand(pkgName)
	}
	return []string{"juliaup", "update", channel}
}

func (Juliaup) PlanClean() [][]string { return nil }

func (j Juliaup) Query(pkgName string) (bool, error) {
	channel, ok := juliaupChannel(pkgName)
	if !ok {
		return false, nil
	}
	channels, err := juliaupInstalledChannels()
	if err != nil {
		return false, err
	}
	return slices.Contains(channels, channel), nil
}

func (j Juliaup) ListInstalled() ([]string, error) {
	channels, err := juliaupInstalledChannels()
	if err != nil {
		return nil, err
	}
	return channels, nil
}

func (Juliaup) QueryVersion(string) (string, error) { return "", nil }

func juliaupChannel(id string) (string, bool) {
	channel := strings.TrimSpace(id)
	if channel == "" || strings.ContainsAny(channel, " \t\n") {
		return "", false
	}
	return channel, true
}

// juliaupInstalledChannels parses `juliaup status` output. Rows list installed
// channels; the channel token is the first non-marker field on each data line.
func juliaupInstalledChannels() ([]string, error) {
	lines, err := runListOutput("juliaup", "status")
	if err != nil {
		return nil, err
	}
	channels := make([]string, 0, len(lines))
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		token := fields[0]
		if token == "*" && len(fields) > 1 {
			token = fields[1]
		}
		if strings.EqualFold(token, "Default") || strings.EqualFold(token, "Channel") {
			continue
		}
		channels = append(channels, token)
	}
	return channels, nil
}

func juliaupInvalidCommand(pkgName string) []string {
	return []string{"sh", "-c", "printf '%s\n' 'genv: juliaup requires a single channel id such as release, lts, or 1.10' >&2; exit 1", "genv-juliaup-invalid", pkgName}
}
