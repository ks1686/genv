package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ks1686/genv/internal/genvfile"
	"github.com/ks1686/genv/internal/testutil"
)

func writeBrewRefreshSpec(t *testing.T, spec string) (specPath, lockPath string) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "xdg"))
	specPath = filepath.Join(dir, "genv.json")
	lockPath = filepath.Join(dir, "genv.lock.json")
	if err := os.WriteFile(specPath, []byte(spec), 0o644); err != nil {
		t.Fatalf("write spec: %v", err)
	}
	writeLock(t, lockPath, []genvfile.LockedPackage{
		{ID: "git", Manager: "brew", PkgName: "git", InstalledVersion: "1.0.0"},
		{ID: "jq", Manager: "brew", PkgName: "jq", InstalledVersion: "1.6.0"},
	})
	return specPath, lockPath
}

func installRefreshThenOutdatedBrew(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	state := filepath.Join(dir, "refreshed")
	stateSh := strings.ReplaceAll(state, "\\", "/")
	testutil.InstallFakeBinary(t, "brew",
		`if [ "$1" = "update" ]; then touch '`+stateSh+`'; exit 0; fi
if [ "$1" = "outdated" ]; then
  if [ -f '`+stateSh+`' ]; then
    echo '{"formulae":[{"name":"jq","current_version":"1.7.1"}],"casks":[]}'
  else
    echo '{"formulae":[],"casks":[]}'
  fi
  exit 0
fi
exit 0`)
}

func TestUpgrade_DryRun_shows_refresh_then_post_refresh_outdated(t *testing.T) {
	installRefreshThenOutdatedBrew(t)
	specPath, lockPath := writeBrewRefreshSpec(t, `{"schemaVersion":"5","packages":[{"id":"git"},{"id":"jq"}]}`)
	before, err := os.ReadFile(specPath)
	if err != nil {
		t.Fatalf("read spec: %v", err)
	}

	var code int
	out := captureStdout(t, func() {
		code = run([]string{"upgrade", "--dry-run", "--file", specPath, "--lock-file", lockPath})
	})
	if code != exitOK {
		t.Fatalf("upgrade --dry-run: expected exitOK (%d), got %d\n%s", exitOK, code, out)
	}
	if !strings.Contains(out, "  brew  ==> brew update") {
		t.Fatalf("upgrade plan missing refresh line:\n%s", out)
	}
	if !strings.Contains(out, "jq") || !strings.Contains(out, "brew upgrade") {
		t.Fatalf("upgrade plan missing post-refresh jq:\n%s", out)
	}
	if strings.Contains(out, "  git  via brew") {
		t.Fatalf("git should be absent from pre-refresh outdated:\n%s", out)
	}
	after, err := os.ReadFile(specPath)
	if err != nil {
		t.Fatalf("read spec after: %v", err)
	}
	if string(after) != string(before) {
		t.Fatal("refresh must not rewrite genv.json")
	}
}

func TestUpdatesCheck_shows_refresh_line(t *testing.T) {
	installRefreshThenOutdatedBrew(t)
	specPath, lockPath := writeBrewRefreshSpec(t, `{"schemaVersion":"5","packages":[{"id":"git"},{"id":"jq"}]}`)

	var code int
	out := captureStdout(t, func() {
		code = run([]string{"updates", "check", "--file", specPath, "--lock-file", lockPath})
	})
	if code != exitOK {
		t.Fatalf("updates check: expected exitOK (%d), got %d\n%s", exitOK, code, out)
	}
	if !strings.Contains(out, "  brew  ==> brew update") {
		t.Fatalf("updates check missing refresh line:\n%s", out)
	}
	if !strings.Contains(out, "jq") {
		t.Fatalf("updates check missing post-refresh jq:\n%s", out)
	}
}

func TestUpgrade_Yes_prints_refresh_keep_all_warning(t *testing.T) {
	testutil.InstallFakeBinary(t, "brew",
		`if [ "$1" = "update" ]; then echo boom >&2; exit 1; fi
if [ "$1" = "outdated" ]; then echo '{"formulae":[],"casks":[]}'; exit 0; fi
if [ "$1" = "upgrade" ]; then exit 0; fi
exit 0`)
	specPath, lockPath := writeBrewRefreshSpec(t, `{"schemaVersion":"5","packages":[{"id":"git"},{"id":"jq"}]}`)

	var code int
	var errOut string
	out := captureStdout(t, func() {
		errOut = captureStderr(t, func() {
			code = run([]string{"upgrade", "--yes", "--file", specPath, "--lock-file", lockPath})
		})
	})
	if code != exitOK {
		t.Fatalf("upgrade --yes: expected exitOK (%d), got %d\nstdout=%s\nstderr=%s", exitOK, code, out, errOut)
	}
	if !strings.Contains(errOut, "could not refresh brew") || !strings.Contains(errOut, "keeping all") {
		t.Fatalf("wet upgrade stderr missing keep-all warning:\n%s", errOut)
	}
	if !strings.Contains(out, "  brew  ==> brew update") {
		t.Fatalf("wet upgrade plan missing refresh line:\n%s", out)
	}
}

func TestUpdatesRunOnce_logs_refresh_keep_all_warning(t *testing.T) {
	testutil.InstallFakeBinary(t, "brew",
		`if [ "$1" = "update" ]; then echo boom >&2; exit 1; fi
if [ "$1" = "outdated" ]; then echo '{"formulae":[],"casks":[]}'; exit 0; fi
exit 0`)
	specPath, lockPath := writeBrewRefreshSpec(t,
		`{"schemaVersion":"6","packages":[{"id":"git"},{"id":"jq"}],"updates":{"enabled":true,"interval":"1h","autoApply":false}}`)

	code := run([]string{"updates", "__run-once", "--file", specPath, "--lock-file", lockPath})
	if code != exitOK {
		t.Fatalf("updates __run-once: expected exitOK (%d), got %d", exitOK, code)
	}
	logPath := filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "genv", "updates.log")
	got, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read updates.log: %v", err)
	}
	if !strings.Contains(string(got), "could not refresh brew") || !strings.Contains(string(got), "keeping all") {
		t.Fatalf("updates.log missing refresh keep-all:\n%s", got)
	}
}
