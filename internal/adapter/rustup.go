package adapter

import (
	"slices"
	"strings"
)

// Rustup manages explicitly tracked Rust toolchains, components, and targets.
type Rustup struct{}

func (Rustup) Name() string { return "rustup" }

func (Rustup) Available() bool {
	_, err := lookPath("rustup")
	return err == nil
}

func (Rustup) NormalizeID(id string, managers map[string]string) (string, bool) {
	return normalizeID("rustup", id, managers)
}

func (Rustup) PlanInstall(pkgName string) []string {
	pkg, ok := parseRustupPackageID(pkgName)
	if !ok {
		return rustupHelpCommand()
	}
	switch pkg.kind {
	case rustupToolchain:
		return []string{"rustup", "toolchain", "install", pkg.name}
	case rustupComponent:
		return []string{"rustup", "component", "add", pkg.name, "--toolchain", pkg.toolchain}
	case rustupTarget:
		return []string{"rustup", "target", "add", pkg.name, "--toolchain", pkg.toolchain}
	}
	return rustupHelpCommand()
}

func (Rustup) PlanUninstall(pkgName string) []string {
	pkg, ok := parseRustupPackageID(pkgName)
	if !ok {
		return rustupHelpCommand()
	}
	switch pkg.kind {
	case rustupToolchain:
		return []string{"rustup", "toolchain", "uninstall", pkg.name}
	case rustupComponent:
		return []string{"rustup", "component", "remove", pkg.name, "--toolchain", pkg.toolchain}
	case rustupTarget:
		return []string{"rustup", "target", "remove", pkg.name, "--toolchain", pkg.toolchain}
	}
	return rustupHelpCommand()
}

func (Rustup) PlanUpgrade(pkgName string) []string {
	pkg, ok := parseRustupPackageID(pkgName)
	if !ok {
		return rustupHelpCommand()
	}
	return []string{"rustup", "update", pkg.upgradeToolchain()}
}

func (Rustup) PlanClean() [][]string { return nil }

func (r Rustup) Query(pkgName string) (bool, error) {
	pkg, ok := parseRustupPackageID(pkgName)
	if !ok {
		return false, nil
	}
	switch pkg.kind {
	case rustupToolchain:
		return r.queryToolchain(pkg.name)
	case rustupComponent:
		return rustupListContainsInstalled("component", pkg.name, pkg.toolchain)
	case rustupTarget:
		return rustupListContainsInstalled("target", pkg.name, pkg.toolchain)
	}
	return false, nil
}

func (Rustup) ListInstalled() ([]string, error) {
	toolchains, err := rustupToolchains()
	if err != nil {
		return nil, err
	}
	installed := make([]string, 0, len(toolchains))
	for _, toolchain := range toolchains {
		installed = append(installed, "toolchain:"+toolchain)
	}
	return installed, nil
}

func (r Rustup) QueryVersion(pkgName string) (string, error) {
	pkg, ok := parseRustupPackageID(pkgName)
	if !ok || pkg.kind != rustupToolchain {
		return "", nil
	}
	found, err := r.queryToolchain(pkg.name)
	if err != nil || !found {
		return "", err
	}
	return pkg.name, nil
}

type rustupPackageKind int

const (
	rustupToolchain rustupPackageKind = iota
	rustupComponent
	rustupTarget
)

type rustupPackage struct {
	kind      rustupPackageKind
	name      string
	toolchain string
}

func (p rustupPackage) upgradeToolchain() string {
	if p.kind == rustupToolchain {
		return p.name
	}
	return p.toolchain
}

func parseRustupPackageID(id string) (rustupPackage, bool) {
	if name, ok := strings.CutPrefix(id, "toolchain:"); ok {
		return parseRustupToolchainID(name)
	}
	if rest, ok := strings.CutPrefix(id, "component:"); ok {
		return parseRustupScopedID(rest, rustupComponent)
	}
	if rest, ok := strings.CutPrefix(id, "target:"); ok {
		return parseRustupScopedID(rest, rustupTarget)
	}
	return rustupPackage{}, false
}

func parseRustupToolchainID(name string) (rustupPackage, bool) {
	if name == "" {
		return rustupPackage{}, false
	}
	return rustupPackage{kind: rustupToolchain, name: name}, true
}

func parseRustupScopedID(rest string, kind rustupPackageKind) (rustupPackage, bool) {
	name, toolchain, ok := strings.Cut(rest, "@")
	if !ok || name == "" || toolchain == "" {
		return rustupPackage{}, false
	}
	return rustupPackage{kind: kind, name: name, toolchain: toolchain}, true
}

func (Rustup) queryToolchain(name string) (bool, error) {
	toolchains, err := rustupToolchains()
	if err != nil {
		return false, err
	}
	return slices.Contains(toolchains, name), nil
}

func rustupToolchains() ([]string, error) {
	lines, err := runListOutput("rustup", "toolchain", "list")
	if err != nil {
		return nil, err
	}
	return parseRustupToolchains(lines), nil
}

func parseRustupToolchains(lines []string) []string {
	toolchains := make([]string, 0, len(lines))
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) > 0 {
			toolchains = append(toolchains, fields[0])
		}
	}
	return toolchains
}

func rustupListContainsInstalled(kind string, name string, toolchain string) (bool, error) {
	lines, err := runListOutput("rustup", kind, "list", "--toolchain", toolchain)
	if err != nil {
		return false, err
	}
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) >= 2 && rustupListNameMatches(kind, fields[0], name) && fields[1] == "(installed)" {
			return true, nil
		}
	}
	return false, nil
}

func rustupListNameMatches(kind string, listedName string, trackedName string) bool {
	if listedName == trackedName {
		return true
	}
	return kind == "component" && strings.HasPrefix(listedName, trackedName+"-")
}

func rustupHelpCommand() []string {
	return []string{"rustup", "help"}
}
