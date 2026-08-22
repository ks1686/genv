package adapter

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

type Go struct{}

func (Go) Name() string { return "go" }

func (Go) Available() bool {
	_, err := lookPath("go")
	return err == nil
}

func (Go) NormalizeID(id string, managers map[string]string) (string, bool) {
	return normalizeID("go", id, managers)
}

func (Go) PlanInstall(pkgName string) []string {
	return []string{"go", "install", goInstallSpec(pkgName)}
}

func (Go) PlanUninstall(pkgName string) []string {
	binDir, ok := goBinDir()
	if !ok {
		return inertGoCommand(pkgName)
	}
	return goUninstallCommand(binDir, pkgName)
}

func (Go) PlanUpgrade(pkgName string) []string {
	return []string{"go", "install", goInstallSpec(pkgName)}
}

func (Go) PlanClean() [][]string { return nil }

func (Go) Query(pkgName string) (bool, error) {
	binDir, ok := goBinDir()
	if !ok {
		return false, nil
	}
	binPath, ok := goBinaryPath(binDir, pkgName)
	if !ok {
		return false, nil
	}
	info, err := os.Stat(binPath)
	if err == nil {
		return !info.IsDir(), nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return false, err
}

func (Go) ListInstalled() ([]string, error) { return nil, nil }

func (Go) QueryVersion(string) (string, error) { return "", nil }

func goInstallSpec(pkgName string) string {
	if strings.Contains(pkgName, "@") {
		return pkgName
	}
	return pkgName + "@latest"
}

func goBinaryBaseName(pkgName string) (string, bool) {
	baseSpec := strings.TrimRight(atVersionBaseName(strings.TrimSpace(pkgName)), "/")
	if baseSpec == "" {
		return "", false
	}
	parts := strings.Split(baseSpec, "/")
	for _, part := range parts {
		if part == "" || part == "." || part == ".." {
			return "", false
		}
	}
	idx := len(parts) - 1
	if isGoSemanticImportSuffix(parts[idx]) {
		idx--
	}
	if idx < 0 {
		return "", false
	}
	name := parts[idx]
	if name == "" || name == "." || name == ".." || strings.ContainsAny(name, `/\`) {
		return "", false
	}
	return name, true
}

func isGoSemanticImportSuffix(part string) bool {
	if len(part) < 2 || part[0] != 'v' {
		return false
	}
	for _, r := range part[1:] {
		if r < '0' || r > '9' {
			return false
		}
	}
	return part != "v0" && part != "v1"
}

func goBinDir() (string, bool) {
	out, err := runProbe("go", "env", "GOBIN", "GOPATH")
	if err != nil {
		return "", false
	}
	return parseGoEnvBinDir(string(out))
}

func parseGoEnvBinDir(output string) (string, bool) {
	lines := strings.Split(strings.ReplaceAll(output, "\r\n", "\n"), "\n")
	if len(lines) > 0 {
		if goBin := strings.TrimSpace(lines[0]); goBin != "" {
			return goBin, true
		}
	}
	if len(lines) < 2 {
		return "", false
	}
	for _, entry := range filepath.SplitList(strings.TrimSpace(lines[1])) {
		if entry != "" {
			return filepath.Join(entry, "bin"), true
		}
	}
	return "", false
}

func goUninstallCommand(binDir string, pkgName string) []string {
	binPath, ok := goBinaryPath(binDir, pkgName)
	if !ok {
		return inertGoCommand(pkgName)
	}
	return []string{"rm", "-f", binPath}
}

func goBinaryPath(binDir string, pkgName string) (string, bool) {
	name, ok := goBinaryBaseName(pkgName)
	if !ok {
		return "", false
	}
	cleanDir := filepath.Clean(binDir)
	if cleanDir == "." || !filepath.IsAbs(cleanDir) {
		return "", false
	}
	candidate := filepath.Join(cleanDir, name)
	rel, err := filepath.Rel(cleanDir, candidate)
	if err != nil || rel != name {
		return "", false
	}
	return candidate, true
}

func inertGoCommand(pkgName string) []string {
	return []string{"sh", "-c", "printf '%s\n' 'genv: cannot safely uninstall Go package; resolved Go bin directory or module basename is unsafe' >&2; exit 1", "genv-go-uninstall", pkgName}
}
