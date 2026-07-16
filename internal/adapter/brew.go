package adapter

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

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

func (brewBase) PlanUpgrade(pkgName string) []string {
	return []string{"brew", "upgrade", pkgName}
}

// PlanUpgradeBatch upgrades multiple formulae/casks in one brew invocation.
func (brewBase) PlanUpgradeBatch(pkgNames []string) []string {
	args := []string{"brew", "upgrade"}
	return append(args, pkgNames...)
}

// brewOutdatedEntry mirrors one element of `brew outdated --json=v2`.
type brewOutdatedEntry struct {
	Name           string `json:"name"`
	CurrentVersion string `json:"current_version"`
}

// ListOutdated reports Homebrew formulae and casks with a newer version
// available, keyed by name -> latest version, intersected with pkgNames.
// Note: `brew outdated` reflects the state of the last `brew update`; genv does
// not run `brew update` first to avoid a network fetch on every scheduled check.
func (brewBase) ListOutdated(pkgNames []string) (map[string]string, error) {
	out, err := exec.Command("brew", "outdated", "--json=v2").Output()
	if err != nil {
		return nil, err
	}
	var payload struct {
		Formulae []brewOutdatedEntry `json:"formulae"`
		Casks    []brewOutdatedEntry `json:"casks"`
	}
	if err := json.Unmarshal(out, &payload); err != nil {
		return nil, fmt.Errorf("parse brew outdated: %w", err)
	}
	want := make(map[string]bool, len(pkgNames))
	for _, n := range pkgNames {
		want[n] = true
	}
	outdated := make(map[string]string)
	for _, group := range [][]brewOutdatedEntry{payload.Formulae, payload.Casks} {
		for _, e := range group {
			if len(pkgNames) > 0 && !want[e.Name] {
				continue
			}
			outdated[e.Name] = e.CurrentVersion
		}
	}
	if len(outdated) == 0 {
		return nil, nil
	}
	return outdated, nil
}

func (brewBase) PlanClean() [][]string {
	return [][]string{{"brew", "cleanup"}}
}

// Search returns brew formula/cask names containing query.
// "brew search" output may include "==> Formulae" / "==> Casks" section headers
// which are skipped. Package names that are not an exact case-insensitive match
// of a section header are returned.
func (brewBase) Search(query string) ([]string, error) {
	lines, err := runListOutput("brew", "search", query)
	if err != nil || len(lines) == 0 {
		return lines, err
	}
	var names []string
	for _, line := range lines {
		if strings.HasPrefix(line, "==>") {
			continue
		}
		if containsFold(line, query) {
			names = append(names, line)
		}
	}
	return names, nil
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
	formulae, err := runListOutput("brew", "list", "--formula", "-1")
	if err != nil {
		return nil, err
	}
	casks, err := runListOutput("brew", "list", "--cask", "-1")
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
	return runListOutput("brew", "list", "--formula", "-1")
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
