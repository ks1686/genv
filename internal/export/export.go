package export

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/ks1686/genv/internal/genvfile"
	"github.com/ks1686/genv/internal/schema"
)

// Options controls filesystem behavior for BuildWithOptions.
type Options struct {
	// BaseDir resolves relative file assets. Empty means the current directory.
	BaseDir string
}

// Build materializes targetID into outDir as genv.json plus report artifacts.
func Build(f *schema.GenvFile, targetID string, outDir string) (Report, error) {
	return BuildWithOptions(f, targetID, outDir, Options{})
}

// BuildWithOptions materializes targetID into outDir as genv.json plus
// report.json and report.md. The snapshot is schemaVersion 8 with exactly one
// target bucket.
func BuildWithOptions(f *schema.GenvFile, targetID string, outDir string, opts Options) (Report, error) {
	if targetID == "" {
		return nil, fmt.Errorf("export target: target id is required")
	}
	if outDir == "" {
		return nil, fmt.Errorf("export target %q: output directory is required", targetID)
	}
	effective, err := schema.MergeTarget(f, targetID)
	if err != nil {
		return nil, err
	}

	report := buildReport(effective.Packages, effective.Files, targetID)
	bundle, envReport := bundleFromFlat(effective)
	report = append(report, envReport...)
	if err := rewriteAndCopyFileAssets(bundle.Files, opts.BaseDir, outDir); err != nil {
		return report.sorted(), err
	}

	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return report.sorted(), fmt.Errorf("creating export directory %s: %w", outDir, err)
	}
	if err := writeSnapshot(filepath.Join(outDir, "genv.json"), targetID, bundle); err != nil {
		return report.sorted(), err
	}
	if err := writeReport(filepath.Join(outDir, "report.json"), report); err != nil {
		return report.sorted(), err
	}
	if err := writeReportMarkdown(filepath.Join(outDir, "report.md"), report); err != nil {
		return report.sorted(), err
	}
	return report.sorted(), nil
}

type snapshotDocument struct {
	SchemaVersion string                          `json:"schemaVersion"`
	Targets       map[string]*schema.TargetBundle `json:"targets"`
}

func writeSnapshot(path, targetID string, bundle *schema.TargetBundle) error {
	doc := snapshotDocument{
		SchemaVersion: schema.Version8,
		Targets: map[string]*schema.TargetBundle{
			targetID: bundle,
		},
	}
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return fmt.Errorf("serializing export snapshot: %w", err)
	}
	data = append(data, '\n')
	if _, valErrs, parseErr := schema.ParseAndValidate(data); parseErr != nil {
		return fmt.Errorf("%w: exported snapshot: %w", genvfile.ErrInvalidFile, parseErr)
	} else if len(valErrs) > 0 {
		msgs := make([]string, len(valErrs))
		for i, valErr := range valErrs {
			msgs[i] = valErr.Error()
		}
		return fmt.Errorf("%w: exported snapshot validation errors:\n  %s", genvfile.ErrInvalidFile, strings.Join(msgs, "\n  "))
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("saving %s: %w", path, err)
	}
	return nil
}

func bundleFromFlat(f *schema.GenvFile) (*schema.TargetBundle, Report) {
	var report Report
	bundle := &schema.TargetBundle{
		Packages: copyPackages(f.Packages),
		Files:    copyFilesConfig(f.Files),
		Hooks:    copyHooksConfig(f.Hooks),
	}
	if len(f.Env) > 0 {
		bundle.Env = make(map[string]*schema.EnvVar, len(f.Env))
		for name, envVar := range f.Env {
			if envVar.Sensitive {
				report = append(report, ReportItem{
					Class:   ClassWarning,
					Code:    "sensitive-env-omitted",
					Message: fmt.Sprintf("env.%s is marked sensitive and was omitted from the export snapshot", name),
				})
				continue
			}
			v := envVar
			bundle.Env[name] = &v
		}
		if len(bundle.Env) == 0 {
			bundle.Env = nil
		}
	}
	bundle.Shell = copyShellToTarget(f.Shell)
	if len(f.Services) > 0 {
		bundle.Services = make(map[string]*schema.Service, len(f.Services))
		for name, svc := range f.Services {
			v := copyService(svc)
			v.Host = nil
			bundle.Services[name] = &v
		}
	}
	return normalizeBundle(bundle), report
}

func normalizeBundle(bundle *schema.TargetBundle) *schema.TargetBundle {
	if len(bundle.Packages) == 0 {
		bundle.Packages = nil
	}
	if bundle.Shell != nil && len(bundle.Shell.Aliases) == 0 && len(bundle.Shell.Functions) == 0 && len(bundle.Shell.Source) == 0 {
		bundle.Shell = nil
	}
	if len(bundle.Services) == 0 {
		bundle.Services = nil
	}
	if bundle.Files != nil && len(bundle.Files.Links) == 0 && len(bundle.Files.Templates) == 0 && len(bundle.Files.Dirs) == 0 {
		bundle.Files = nil
	}
	if bundle.Hooks != nil &&
		len(bundle.Hooks.PreApply) == 0 &&
		len(bundle.Hooks.PostApply) == 0 &&
		len(bundle.Hooks.PreAdd) == 0 &&
		len(bundle.Hooks.PostAdd) == 0 &&
		len(bundle.Hooks.PreRemove) == 0 &&
		len(bundle.Hooks.PostRemove) == 0 &&
		len(bundle.Hooks.PreUpgrade) == 0 &&
		len(bundle.Hooks.PostUpgrade) == 0 {
		bundle.Hooks = nil
	}
	return bundle
}

func buildReport(packages []schema.Package, files *schema.FilesConfig, targetID string) Report {
	var report Report
	allowed := managerAllowlist(targetID)
	for _, pkg := range packages {
		constraints := managerConstraints(pkg)
		if len(constraints) > 0 && !intersects(constraints, allowed) {
			report = append(report, ReportItem{
				Class:     ClassError,
				Code:      "manager-not-supported",
				Message:   fmt.Sprintf("package %q has no managers usable on target %q", pkg.ID, targetID),
				PackageID: pkg.ID,
			})
		}
		if aptDNFRelevant(targetID, constraints) {
			report = append(report, ReportItem{
				Class:     ClassSuggestion,
				Code:      "apt-dnf-deferred",
				Message:   fmt.Sprintf("apt/dnf package-manager adapters are deferred for target %q", targetID),
				PackageID: pkg.ID,
			})
		}
	}
	if files != nil {
		for i, link := range files.Links {
			if isAbsolutePath(link.Source) {
				report = append(report, absoluteSourceItem(fmt.Sprintf("files.links[%d].source", i), link.Source))
			}
		}
		for i, tpl := range files.Templates {
			if isAbsolutePath(tpl.Source) {
				report = append(report, absoluteSourceItem(fmt.Sprintf("files.templates[%d].source", i), tpl.Source))
			}
		}
	}
	return report
}

func absoluteSourceItem(field, source string) ReportItem {
	return ReportItem{
		Class:   ClassError,
		Code:    "absolute-source",
		Message: fmt.Sprintf("%s is absolute and cannot be bundled: %s", field, source),
	}
}

func managerConstraints(pkg schema.Package) map[string]bool {
	out := make(map[string]bool)
	if pkg.Prefer != "" {
		out[pkg.Prefer] = true
	}
	for mgr := range pkg.Managers {
		out[mgr] = true
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func aptDNFRelevant(targetID string, constraints map[string]bool) bool {
	if constraints["apt"] || constraints["dnf"] {
		return true
	}
	return len(constraints) == 0 && (targetID == "ubuntu" || targetID == "linux")
}

func intersects(a, b map[string]bool) bool {
	for k := range a {
		if b[k] {
			return true
		}
	}
	return false
}

func managerAllowlist(targetID string) map[string]bool {
	switch targetID {
	case "macos":
		return set("brew", "mas", "linuxbrew", universalManagers)
	case "arch", "wsl-arch":
		return set("pacman", "paru", "yay", "snap", "linuxbrew", universalManagers)
	case "ubuntu":
		return set("snap", "linuxbrew", universalManagers)
	case "windows":
		return set("winget", "scoop", "choco", universalManagers)
	case "linux":
		return set("pacman", "paru", "yay", "snap", "linuxbrew", universalManagers)
	default:
		return set(universalManagers)
	}
}

var universalManagers = []string{
	"bun", "npm", "pnpm", "yarn", "deno", "volta",
	"uv", "pipx", "pip-user", "poetry", "conda", "mamba", "pixi",
	"cargo", "go", "rustup", "gem", "composer", "dotnet-tool",
	"ghcup", "stack", "opam", "juliaup", "sdkman", "asdf", "mise",
	"krew", "helm", "vscode",
}

func set(values ...any) map[string]bool {
	out := make(map[string]bool)
	for _, value := range values {
		switch v := value.(type) {
		case string:
			out[v] = true
		case []string:
			for _, s := range v {
				out[s] = true
			}
		}
	}
	return out
}

func rewriteAndCopyFileAssets(files *schema.FilesConfig, baseDir, outDir string) error {
	if files == nil {
		return nil
	}
	for i := range files.Links {
		source := files.Links[i].Source
		if source == "" || isAbsolutePath(source) {
			continue
		}
		rel, err := copyAsset(baseDir, outDir, source, "link", i)
		if err != nil {
			return err
		}
		files.Links[i].Source = rel
	}
	for i := range files.Templates {
		source := files.Templates[i].Source
		if source == "" || isAbsolutePath(source) {
			continue
		}
		rel, err := copyAsset(baseDir, outDir, source, "template", i)
		if err != nil {
			return err
		}
		files.Templates[i].Source = rel
	}
	return nil
}

func isAbsolutePath(path string) bool {
	if filepath.IsAbs(path) {
		return true
	}
	if strings.HasPrefix(path, "/") {
		return true
	}
	if len(path) >= 3 && isASCIIAlpha(path[0]) && path[1] == ':' && (path[2] == '\\' || path[2] == '/') {
		return true
	}
	if strings.HasPrefix(path, `\\`) {
		parts := strings.FieldsFunc(path[2:], func(r rune) bool { return r == '\\' || r == '/' })
		return len(parts) >= 2 && parts[0] != "" && parts[1] != ""
	}
	return false
}

func isASCIIAlpha(b byte) bool {
	return (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z')
}

func copyAsset(baseDir, outDir, source, kind string, index int) (string, error) {
	sourcePath := source
	if baseDir != "" {
		sourcePath = filepath.Join(baseDir, source)
	}
	destRel := assetDestination(source, kind, index)
	destPath := filepath.Join(outDir, filepath.FromSlash(destRel))
	if err := copyPath(sourcePath, destPath); err != nil {
		return "", fmt.Errorf("copying export asset %s to %s: %w", sourcePath, destPath, err)
	}
	return destRel, nil
}

func assetDestination(source, kind string, index int) string {
	clean := filepath.ToSlash(filepath.Clean(source))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		base := filepath.Base(clean)
		if base == "." || base == string(filepath.Separator) || base == "" {
			base = "asset"
		}
		clean = fmt.Sprintf("external/%s-%d-%s", kind, index, base)
	}
	return filepath.ToSlash(filepath.Join("files", filepath.FromSlash(clean)))
}

func copyPath(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return copyDir(src, dst)
	}
	return copyFile(src, dst, info.Mode().Perm())
}

func copyDir(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		return copyFile(path, target, info.Mode().Perm())
	})
}

func copyFile(src, dst string, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

func copyPackages(in []schema.Package) []schema.Package {
	if len(in) == 0 {
		return nil
	}
	out := make([]schema.Package, len(in))
	for i, pkg := range in {
		out[i] = pkg
		out[i].Host = nil
		out[i].Managers = copyStringMap(pkg.Managers)
	}
	return out
}

func copyStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func copyShellToTarget(in *schema.ShellConfig) *schema.TargetShellConfig {
	if in == nil {
		return nil
	}
	out := &schema.TargetShellConfig{
		Source: copyStrings(in.Source),
	}
	if len(in.Aliases) > 0 {
		out.Aliases = make(map[string]*schema.ShellAlias, len(in.Aliases))
		for name, alias := range in.Aliases {
			v := alias
			out.Aliases[name] = &v
		}
	}
	if len(in.Functions) > 0 {
		out.Functions = make(map[string]*schema.ShellFunction, len(in.Functions))
		for name, fn := range in.Functions {
			v := fn
			out.Functions[name] = &v
		}
	}
	return out
}

func copyFilesConfig(in *schema.FilesConfig) *schema.FilesConfig {
	if in == nil {
		return nil
	}
	return &schema.FilesConfig{
		Links:     copyFileLinks(in.Links),
		Templates: copyFileTemplates(in.Templates),
		Dirs:      copyFileDirs(in.Dirs),
	}
}

func copyFileLinks(in []schema.FileLink) []schema.FileLink {
	if len(in) == 0 {
		return nil
	}
	out := make([]schema.FileLink, len(in))
	for i, v := range in {
		out[i] = v
		out[i].Host = nil
	}
	return out
}

func copyFileTemplates(in []schema.FileTemplate) []schema.FileTemplate {
	if len(in) == 0 {
		return nil
	}
	out := make([]schema.FileTemplate, len(in))
	for i, v := range in {
		out[i] = v
		out[i].Host = nil
	}
	return out
}

func copyFileDirs(in []schema.FileDir) []schema.FileDir {
	if len(in) == 0 {
		return nil
	}
	out := make([]schema.FileDir, len(in))
	for i, v := range in {
		out[i] = v
		out[i].Host = nil
	}
	return out
}

func copyHooksConfig(in *schema.HooksConfig) *schema.HooksConfig {
	if in == nil {
		return nil
	}
	return &schema.HooksConfig{
		PreApply:    copyHooks(in.PreApply),
		PostApply:   copyHooks(in.PostApply),
		PreAdd:      copyHooks(in.PreAdd),
		PostAdd:     copyHooks(in.PostAdd),
		PreRemove:   copyHooks(in.PreRemove),
		PostRemove:  copyHooks(in.PostRemove),
		PreUpgrade:  copyHooks(in.PreUpgrade),
		PostUpgrade: copyHooks(in.PostUpgrade),
	}
}

func copyHooks(in []schema.Hook) []schema.Hook {
	if len(in) == 0 {
		return nil
	}
	out := make([]schema.Hook, len(in))
	for i, v := range in {
		out[i] = v
		out[i].Host = nil
	}
	return out
}

func copyService(in schema.Service) schema.Service {
	out := in
	out.Start = copyStrings(in.Start)
	out.Stop = copyStrings(in.Stop)
	out.Restart = copyStrings(in.Restart)
	out.Status = copyStrings(in.Status)
	return out
}

func copyStrings(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	return append([]string(nil), in...)
}
