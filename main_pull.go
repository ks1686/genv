package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/ks1686/genv/internal/genvfile"
	pullassets "github.com/ks1686/genv/internal/pull"
	"github.com/ks1686/genv/internal/schema"
)

// pullCmd implements `genv pull [flags]`.
// It fetches the spec repository declared in genv.json's repo block (or supplied
// via --url/--ref flags), updates a hidden cache under ~/.cache/genv/repo, and
// copies the remote genv.json into the local spec path.
func pullCmd(args []string) int {
	fs := flag.NewFlagSet("pull", flag.ContinueOnError)
	fs.Usage = func() {
		fPrintln(os.Stderr, "usage: genv pull [flags]")
		fPrintln(os.Stderr)
		fPrintln(os.Stderr, "Fetch the spec from a git repository and update genv.json.")
		fPrintln(os.Stderr)
		fPrintln(os.Stderr, "flags:")
		fs.PrintDefaults()
	}

	file := fs.String("file", defaultSpecPath(), "path to genv.json")
	urlFlag := fs.String("url", "", "override the repository URL")
	refFlag := fs.String("ref", "", "override the repository ref (default: main)")
	dryRun := fs.Bool("dry-run", false, "print what would be pulled without writing")

	if err := fs.Parse(args); err != nil {
		return exitUsage
	}

	if *file == "-" {
		fPrintln(os.Stderr, "genv pull: cannot write spec to stdout")
		return exitUsage
	}

	f, err := genvfile.Read(*file)
	if err != nil {
		if errors.Is(err, genvfile.ErrNotFound) {
			fprintf(os.Stderr, "genv: %s not found\n", *file)
			return exitIO
		}
		fprintf(os.Stderr, "genv: %v\n", err)
		if errors.Is(err, genvfile.ErrInvalidFile) {
			return exitValidation
		}
		return exitIO
	}

	url, ref, err := resolvePullSource(f.Repo, *urlFlag, *refFlag)
	if err != nil {
		fprintf(os.Stderr, "genv pull: %v\n", err)
		return exitUsage
	}

	if *dryRun {
		fprintf(os.Stdout, "would pull %s @ %s into %s\n", url, ref, *file)
		assetSpec, remoteAssets := dryRunAssetSpec(f, url, ref)
		if !remoteAssets {
			fPrintln(os.Stdout, "remote spec not cached; listing local asset references")
		}
		if assets := pullassets.BundleAssetSources(assetSpec); len(assets) > 0 {
			fPrintln(os.Stdout, "would copy assets:")
			for _, asset := range assets {
				fprintf(os.Stdout, "  %s\n", asset)
			}
		}
		return exitOK
	}

	if _, err := exec.LookPath("git"); err != nil {
		fPrintln(os.Stderr, "genv pull: git is not installed")
		return exitLogic
	}

	cacheDir, err := pullCacheDir()
	if err != nil {
		fprintf(os.Stderr, "genv pull: cache directory: %v\n", err)
		return exitIO
	}

	if err := updateRepoCache(cacheDir, url, ref); err != nil {
		fprintf(os.Stderr, "genv pull: %v\n", err)
		return exitIO
	}

	src := filepath.Join(cacheDir, "genv.json")
	remote, err := genvfile.Read(src)
	if err != nil {
		fprintf(os.Stderr, "genv pull: reading remote spec: %v\n", err)
		if errors.Is(err, genvfile.ErrInvalidFile) {
			return exitValidation
		}
		return exitIO
	}
	if err := copyFile(src, *file); err != nil {
		fprintf(os.Stderr, "genv pull: writing spec: %v\n", err)
		return exitIO
	}
	copiedAssets, err := pullassets.CopyBundleAssets(cacheDir, filepath.Dir(*file), remote)
	if err != nil {
		fprintf(os.Stderr, "genv pull: copying assets: %v\n", err)
		return exitIO
	}

	fprintf(os.Stdout, "pulled %s @ %s into %s\n", url, ref, *file)
	if len(copiedAssets) > 0 {
		fPrintln(os.Stdout, "copied assets:")
		for _, asset := range copiedAssets {
			fprintf(os.Stdout, "  %s\n", asset)
		}
	}
	return exitOK
}

// resolvePullSource determines the effective URL and ref for `genv pull`.
// CLI flags override values from the spec's repo block. When no ref is
// specified anywhere, it defaults to "main".
func resolvePullSource(repo *schema.Repo, urlFlag, refFlag string) (url, ref string, err error) {
	if urlFlag != "" {
		url = urlFlag
	} else if repo != nil && repo.URL != "" {
		url = repo.URL
	} else {
		return "", "", errors.New("no repository URL: configure repo.url or pass --url")
	}

	if refFlag != "" {
		ref = refFlag
	} else if repo != nil && repo.Ref != "" {
		ref = repo.Ref
	} else {
		ref = "main"
	}

	if err := schema.ValidRepoURL(url); err != nil {
		return "", "", err
	}
	if err := schema.ValidGitRef(ref); err != nil {
		return "", "", err
	}

	return url, ref, nil
}

func dryRunAssetSpec(local *schema.GenvFile, url, ref string) (*schema.GenvFile, bool) {
	if _, err := exec.LookPath("git"); err != nil {
		return local, false
	}
	cacheDir, err := pullCacheDir()
	if err != nil {
		return local, false
	}
	if !dryRunShouldUpdateCache(cacheDir, url) {
		return local, false
	}
	if err := updateRepoCache(cacheDir, url, ref); err != nil {
		return local, false
	}
	remote, err := genvfile.Read(filepath.Join(cacheDir, "genv.json"))
	if err != nil {
		return local, false
	}
	return remote, true
}

func dryRunShouldUpdateCache(cacheDir, url string) bool {
	if _, err := os.Stat(cacheDir); err == nil {
		return true
	}
	if strings.HasPrefix(url, "file://") || filepath.IsAbs(url) {
		return true
	}
	if _, err := os.Stat(url); err == nil {
		return true
	}
	return false
}

// pullCacheDir returns the deterministic local cache path for the spec repo.
// It respects $XDG_CACHE_HOME and falls back to ~/.cache/genv/repo.
func pullCacheDir() (string, error) {
	cache := os.Getenv("XDG_CACHE_HOME")
	if cache == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("cannot determine home directory: %w", err)
		}
		cache = filepath.Join(home, ".cache")
	}
	return filepath.Join(cache, "genv", "repo"), nil
}

// updateRepoCache ensures cacheDir contains the requested ref of url.
// It clones on first use and fetches/updates on subsequent runs.
func updateRepoCache(cacheDir, url, ref string) error {
	if _, err := os.Stat(cacheDir); os.IsNotExist(err) {
		if err := os.MkdirAll(filepath.Dir(cacheDir), 0o700); err != nil {
			return fmt.Errorf("creating cache directory: %w", err)
		}
		if err := runGit("clone", "--", url, cacheDir); err != nil {
			return fmt.Errorf("cloning repo: %w", err)
		}
		return runGit("-C", cacheDir, "checkout", ref)
	}

	if err := runGit("-C", cacheDir, "remote", "set-url", "origin", url); err != nil {
		return fmt.Errorf("updating remote url: %w", err)
	}
	if err := runGit("-C", cacheDir, "fetch", "origin"); err != nil {
		return fmt.Errorf("fetching repo: %w", err)
	}
	if err := runGit("-C", cacheDir, "checkout", ref); err != nil {
		return fmt.Errorf("checking out ref %q: %w", ref, err)
	}
	if isLocalBranch(cacheDir, ref) {
		if err := runGit("-C", cacheDir, "merge", "--ff-only", "origin/"+ref); err != nil {
			return fmt.Errorf("fast-forwarding to origin/%s: %w", ref, err)
		}
	}
	return nil
}

// isLocalBranch reports whether ref names an existing local branch in repo.
func isLocalBranch(repo, ref string) bool {
	cmd := exec.Command("git", "-C", repo, "show-ref", "--verify", "--quiet", "refs/heads/"+ref)
	return cmd.Run() == nil
}

// runGit runs a git command and returns a wrapped error on failure.
func runGit(args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git %s: %w", args[0], err)
	}
	return nil
}

// copyFile copies src to dst, creating parent directories as needed.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("opening %s: %w", src, err)
	}
	defer func() { _ = in.Close() }()

	if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
		return fmt.Errorf("creating parent directory: %w", err)
	}

	out, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("creating %s: %w", dst, err)
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return fmt.Errorf("copying to %s: %w", dst, err)
	}
	if err := out.Close(); err != nil {
		return fmt.Errorf("closing %s: %w", dst, err)
	}
	return nil
}
