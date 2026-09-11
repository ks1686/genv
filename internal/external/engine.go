package external

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/ks1686/genv/internal/genvfile"
	"github.com/ks1686/genv/internal/schema"
)

// Engine executes release-backed external package operations.
type Engine struct {
	Client      Client
	Host        Host
	Mode        ExecutionMode
	Acknowledge func(message string) bool
	Stdin       io.Reader
	Output      io.Writer
	Home        string
}

// Installed is the verified result of one external installation.
type Installed struct {
	Version string
	Receipt *genvfile.ExternalReceipt
}

// Planned is metadata-only selection for dry-run output.
type Planned struct {
	Version      string
	AssetName    string
	AssetURL     string
	InstallType  string
	Scope        string
	Verification []string
	Detail       string
}

// Install resolves, verifies, and transactionally installs a managed external package.
func (e Engine) Install(ctx context.Context, pkg schema.Package) (Installed, error) {
	if pkg.External == nil {
		return Installed{}, fmt.Errorf("external recipe is required")
	}
	recipe := pkg.External
	release, err := e.Client.Resolve(ctx, recipe.Source, recipe.AllowInsecureHTTP)
	if err != nil {
		return Installed{}, fmt.Errorf("resolve %q release: %w", pkg.ID, err)
	}
	platform, err := SelectPlatform(recipe.Platforms, e.Host)
	if err != nil {
		return Installed{}, fmt.Errorf("select %q platform: %w", pkg.ID, err)
	}
	templateValues := TemplateValues{Version: release.Version, Tag: release.Tag, OS: e.Host.OS, Arch: e.Host.Arch, Destination: platform.Install.Destination}
	install, err := expandInstallTemplates(platform.Install, templateValues)
	if err != nil {
		return Installed{}, fmt.Errorf("expand %q install template: %w", pkg.ID, err)
	}
	templateValues.Destination = install.Destination
	home := e.Home
	if home == "" {
		home, err = os.UserHomeDir()
		if err != nil {
			return Installed{}, fmt.Errorf("resolve home directory: %w", err)
		}
	}
	if err := validateInstallScope(install, home); err != nil {
		return Installed{}, err
	}
	asset, err := resolveArtifact(release, platform, e.Host)
	if err != nil {
		return Installed{}, fmt.Errorf("select %q artifact: %w", pkg.ID, err)
	}
	stagingDir, err := os.MkdirTemp("", "genv-external-*")
	if err != nil {
		return Installed{}, fmt.Errorf("create external staging directory: %w", err)
	}
	defer os.RemoveAll(stagingDir)
	artifact, err := e.Client.Download(ctx, asset.URL, recipe.AllowInsecureHTTP, stagingDir, 0)
	if err != nil {
		return Installed{}, err
	}
	verification, err := e.verifyArtifact(ctx, recipe, release, asset, artifact, stagingDir)
	if err != nil {
		return Installed{}, fmt.Errorf("verify %q: %w", pkg.ID, err)
	}
	verified := verification != "unverified"
	artifactURL, _ := url.Parse(asset.URL)
	if err := CheckExecutionPolicy(PolicyInput{
		Verified: verified, Script: install.Type == "script", InsecureTransport: artifactURL != nil && artifactURL.Scheme == "http",
		BackgroundAllowed: recipe.AllowBackgroundExecution, Mode: e.Mode,
	}); err != nil {
		return Installed{}, err
	}
	if !verified && (e.Acknowledge == nil || !e.Acknowledge(fmt.Sprintf("Install unverified external package %q?", pkg.ID))) {
		return Installed{}, fmt.Errorf("unverified external package %q was not acknowledged", pkg.ID)
	}
	var paths []genvfile.ExternalPathReceipt
	var uninstall []string
	var restore func()
	var finish func() error
	switch install.Type {
	case "direct":
		destination, expandErr := expandDestination(install.Destination)
		if expandErr != nil {
			return Installed{}, expandErr
		}
		restore, finish, err = installDirect(artifact.Path, destination)
		if err != nil {
			err = wrapSystemScopeError(install.Scope, destination, err)
		}
		paths = []genvfile.ExternalPathReceipt{{Path: destination, SHA256: artifact.SHA256}}
	case "archive":
		paths, restore, finish, err = installArchive(artifact.Path, asset.Name, install)
	case "script":
		values := templateValues
		values.Script = artifact.Path
		uninstall, err = expandCommand(install.Uninstall, values)
		if err == nil {
			err = RunInstallerScript(ctx, artifact.Path, install, values, e.Stdin, e.Output)
			if err != nil && len(uninstall) > 0 {
				_ = RunUninstall(context.Background(), uninstall, e.Stdin, e.Output)
			}
		}
		restore = func() {
			if len(uninstall) > 0 {
				_ = RunUninstall(context.Background(), uninstall, e.Stdin, e.Output)
			}
		}
		finish = func() error { return nil }
	default:
		err = fmt.Errorf("external install type %q is not implemented", install.Type)
	}
	if err != nil {
		return Installed{}, fmt.Errorf("install %q: %w", pkg.ID, err)
	}
	version, err := detectVersion(ctx, recipe.Detect)
	if err != nil {
		restore()
		return Installed{}, fmt.Errorf("verify installed %q: %w", pkg.ID, err)
	}
	if version != release.Version {
		restore()
		return Installed{}, fmt.Errorf("verify installed %q: detected version %q, expected %q", pkg.ID, version, release.Version)
	}
	if err := finish(); err != nil {
		restore()
		return Installed{}, fmt.Errorf("finalize %q installation: %w", pkg.ID, err)
	}
	recipeDigest, err := hashRecipe(recipe)
	if err != nil {
		return Installed{}, fmt.Errorf("hash %q recipe: %w", pkg.ID, err)
	}
	return Installed{Version: version, Receipt: &genvfile.ExternalReceipt{
		SourceType: recipe.Source.Type, ReleaseID: release.ID, ReleaseTag: release.Tag,
		ArtifactURL: sanitizedURL(asset.URL), ArtifactSHA256: artifact.SHA256,
		Verification: verification, RecipeSHA256: recipeDigest, InstallType: install.Type,
		Owned: install.Type != "script", Paths: paths, Uninstall: uninstall,
	}}, nil
}

// Plan resolves release metadata and artifact selection without downloading.
func (e Engine) Plan(ctx context.Context, pkg schema.Package) (Planned, error) {
	if pkg.External == nil {
		return Planned{}, fmt.Errorf("external recipe is required")
	}
	recipe := pkg.External
	release, err := e.Client.Resolve(ctx, recipe.Source, recipe.AllowInsecureHTTP)
	if err != nil {
		return Planned{}, fmt.Errorf("resolve %q release: %w", pkg.ID, err)
	}
	platform, err := SelectPlatform(recipe.Platforms, e.Host)
	if err != nil {
		return Planned{}, fmt.Errorf("select %q platform: %w", pkg.ID, err)
	}
	templateValues := TemplateValues{Version: release.Version, Tag: release.Tag, OS: e.Host.OS, Arch: e.Host.Arch, Destination: platform.Install.Destination}
	install, err := expandInstallTemplates(platform.Install, templateValues)
	if err != nil {
		return Planned{}, fmt.Errorf("expand %q install template: %w", pkg.ID, err)
	}
	home := e.Home
	if home == "" {
		home, err = os.UserHomeDir()
		if err != nil {
			return Planned{}, fmt.Errorf("resolve home directory: %w", err)
		}
	}
	if err := validateInstallScope(install, home); err != nil {
		return Planned{}, err
	}
	asset, err := resolveArtifact(release, platform, e.Host)
	if err != nil {
		return Planned{}, fmt.Errorf("select %q artifact: %w", pkg.ID, err)
	}
	methods := make([]string, 0, len(recipe.Verify))
	for _, verification := range recipe.Verify {
		methods = append(methods, verification.Type)
	}
	if len(methods) == 0 {
		methods = []string{"unverified"}
	}
	scope := install.Scope
	if scope == "" {
		scope = "user"
	}
	detail := fmt.Sprintf("%s %s; verify=%s; scope=%s", install.Type, release.Version, strings.Join(methods, "+"), scope)
	if hint := ElevationHint(scope); hint != "" {
		detail += "; " + hint
	}
	return Planned{
		Version: release.Version, AssetName: asset.Name, AssetURL: sanitizedURL(asset.URL),
		InstallType: install.Type, Scope: scope, Verification: methods, Detail: detail,
	}, nil
}

func resolveArtifact(release Release, platform schema.ExternalPlatform, host Host) (Asset, error) {
	if platform.AssetRegex != "" {
		return SelectAsset(release, platform)
	}
	artifactURL, err := ExpandArtifactURL(platform.ArtifactURL, release, host)
	if err != nil {
		return Asset{}, err
	}
	parsed, err := url.Parse(artifactURL)
	if err != nil {
		return Asset{}, fmt.Errorf("parse artifact URL: %w", err)
	}
	return Asset{Name: filepath.Base(parsed.Path), URL: artifactURL}, nil
}

func (e Engine) verifyArtifact(ctx context.Context, recipe *schema.ExternalRecipe, release Release, asset Asset, artifact DownloadedArtifact, stagingDir string) (string, error) {
	if len(recipe.Verify) == 0 {
		if recipe.AllowUnverified {
			return "unverified", nil
		}
		return "", fmt.Errorf("external recipe has no verification policy")
	}
	methods := make([]string, 0, len(recipe.Verify))
	for _, verification := range recipe.Verify {
		var expected string
		switch verification.Type {
		case "githubDigest":
			expected = asset.Digest
		case "sha256":
			expected = verification.Value
			if expected == "" {
				var err error
				expected, err = jsonPointer(release.Metadata, verification.ValuePointer)
				if err != nil {
					return "", err
				}
			}
		case "sha256File":
			checksum, err := e.verificationMaterial(ctx, recipe, release, verification.AssetRegex, verification.URL, stagingDir)
			if err != nil {
				return "", err
			}
			expected, err = ChecksumForAsset(checksum, asset.Name)
			if err != nil {
				return "", err
			}
		case "minisign":
			signature, err := e.verificationMaterial(ctx, recipe, release, verification.SignatureAssetRegex, verification.URL, stagingDir)
			if err != nil {
				return "", err
			}
			payload, err := os.ReadFile(artifact.Path)
			if err != nil {
				return "", err
			}
			publicKey, err := localKey(verification.PublicKey, verification.PublicKeyFile)
			if err != nil {
				return "", err
			}
			if err := VerifyMinisign(payload, signature, string(publicKey)); err != nil {
				return "", err
			}
			methods = append(methods, verification.Type)
			continue
		case "openpgp":
			signature, err := e.verificationMaterial(ctx, recipe, release, verification.SignatureAssetRegex, verification.URL, stagingDir)
			if err != nil {
				return "", err
			}
			payload, err := os.ReadFile(artifact.Path)
			if err != nil {
				return "", err
			}
			publicKey, err := localKey(verification.PublicKey, verification.PublicKeyFile)
			if err != nil {
				return "", err
			}
			if err := VerifyOpenPGP(payload, signature, publicKey, verification.Fingerprint); err != nil {
				return "", err
			}
			methods = append(methods, verification.Type)
			continue
		case "sigstore":
			bundleData, err := e.verificationMaterial(ctx, recipe, release, verification.BundleAssetRegex, verification.URL, stagingDir)
			if err != nil {
				return "", err
			}
			bundlePath := filepath.Join(stagingDir, "sigstore-bundle.json")
			if err := os.WriteFile(bundlePath, bundleData, 0o600); err != nil {
				return "", fmt.Errorf("stage Sigstore bundle: %w", err)
			}
			if err := VerifySigstore(artifact.Path, bundlePath, verification.Identity, verification.Issuer); err != nil {
				return "", err
			}
			methods = append(methods, verification.Type)
			continue
		default:
			return "", fmt.Errorf("verification type %q is not implemented", verification.Type)
		}
		if err := VerifySHA256(artifact.SHA256, expected); err != nil {
			return "", err
		}
		methods = append(methods, verification.Type)
	}
	return strings.Join(methods, ","), nil
}

func (e Engine) verificationMaterial(ctx context.Context, recipe *schema.ExternalRecipe, release Release, assetRegex, urlTemplate, stagingDir string) ([]byte, error) {
	var endpoint string
	if assetRegex != "" {
		asset, err := SelectAsset(release, schema.ExternalPlatform{AssetRegex: assetRegex})
		if err != nil {
			return nil, err
		}
		endpoint = asset.URL
	} else {
		var err error
		endpoint, err = ExpandArtifactURL(urlTemplate, release, e.Host)
		if err != nil {
			return nil, err
		}
	}
	downloaded, err := e.Client.Download(ctx, endpoint, recipe.AllowInsecureHTTP, stagingDir, 16<<20)
	if err != nil {
		return nil, err
	}
	return os.ReadFile(downloaded.Path)
}

func localKey(inline, path string) ([]byte, error) {
	if inline != "" {
		return []byte(inline), nil
	}
	expanded, err := expandDestination(path)
	if err != nil {
		return nil, err
	}
	key, err := os.ReadFile(expanded)
	if err != nil {
		return nil, fmt.Errorf("read verification public key: %w", err)
	}
	return key, nil
}

func installDirect(source, destination string) (restore func(), finish func() error, err error) {
	return installDirectWithMode(source, destination, 0o755)
}

func installDirectWithMode(source, destination string, mode os.FileMode) (restore func(), finish func() error, err error) {
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return nil, nil, err
	}
	staged, err := os.CreateTemp(filepath.Dir(destination), ".genv-install-*")
	if err != nil {
		return nil, nil, err
	}
	stagedPath := staged.Name()
	cleanupStaged := true
	defer func() {
		_ = staged.Close()
		if cleanupStaged {
			_ = os.Remove(stagedPath)
		}
	}()
	in, err := os.Open(source)
	if err != nil {
		return nil, nil, err
	}
	if _, err := io.Copy(staged, in); err != nil {
		_ = in.Close()
		return nil, nil, err
	}
	if err := in.Close(); err != nil {
		return nil, nil, err
	}
	if err := staged.Chmod(mode); err != nil {
		return nil, nil, err
	}
	if err := staged.Sync(); err != nil {
		return nil, nil, err
	}
	if err := staged.Close(); err != nil {
		return nil, nil, err
	}
	backup := destination + ".genv-backup"
	hadExisting := false
	if _, statErr := os.Lstat(destination); statErr == nil {
		_ = os.Remove(backup)
		if err := os.Rename(destination, backup); err != nil {
			return nil, nil, err
		}
		hadExisting = true
	} else if !os.IsNotExist(statErr) {
		return nil, nil, statErr
	}
	if err := os.Rename(stagedPath, destination); err != nil {
		if hadExisting {
			_ = os.Rename(backup, destination)
		}
		return nil, nil, err
	}
	cleanupStaged = false
	restored := false
	restore = func() {
		if restored {
			return
		}
		restored = true
		_ = os.Remove(destination)
		if hadExisting {
			_ = os.Rename(backup, destination)
		}
	}
	finish = func() error {
		if hadExisting {
			return os.Remove(backup)
		}
		return nil
	}
	return restore, finish, nil
}

func detectVersion(ctx context.Context, detect schema.ExternalDetect) (string, error) {
	cmd := exec.CommandContext(ctx, detect.Command[0], detect.Command[1:]...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", err
	}
	re, err := regexp.Compile(detect.VersionRegex)
	if err != nil {
		return "", err
	}
	match := re.FindSubmatch(output)
	if len(match) != 2 || len(match[1]) == 0 {
		return "", fmt.Errorf("version output did not match configured regex")
	}
	return string(match[1]), nil
}

func expandDestination(path string) (string, error) {
	if path == "~" || strings.HasPrefix(path, "~/") || strings.HasPrefix(path, `~\`) {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve home directory: %w", err)
		}
		path = filepath.Join(home, path[2:])
	}
	if !filepath.IsAbs(path) {
		return "", fmt.Errorf("external destination must be absolute or home-relative")
	}
	return filepath.Clean(path), nil
}

func hashRecipe(recipe *schema.ExternalRecipe) (string, error) {
	data, err := json.Marshal(recipe)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:]), nil
}

// RecipeSHA256 returns the canonical non-secret recipe identity stored in lock receipts.
func RecipeSHA256(recipe *schema.ExternalRecipe) (string, error) {
	return hashRecipe(recipe)
}

func sanitizedURL(value string) string {
	u, err := url.Parse(value)
	if err != nil {
		return ""
	}
	u.User = nil
	u.RawQuery = ""
	u.Fragment = ""
	return u.String()
}
