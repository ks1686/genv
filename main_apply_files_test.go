package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/ks1686/genv/internal/adapter"
	"github.com/ks1686/genv/internal/genvfile"
	"github.com/ks1686/genv/internal/schema"
	"github.com/ks1686/genv/internal/testutil"
)

func TestApply_FileMismatchDoesNotAbortPackages(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "xdg"))
	testutil.SetHome(t, dir)
	specPath := filepath.Join(dir, "genv.json")
	lockPath := filepath.Join(dir, "genv.lock.json")
	sourcePath := filepath.Join(dir, "source.txt")
	targetPath := filepath.Join(dir, "target.txt")
	installLog := filepath.Join(dir, "install.log")

	writeTestFile(t, sourcePath, "desired\n")
	writeTestFile(t, targetPath, "blocking-real-file\n")
	registerLifecycleHookAdapter(t, lifecycleHookAdapter{installMarker: installLog})

	writeTestFile(t, specPath, `{`+
		`"schemaVersion":"6",`+
		`"packages":[{"id":"alpha","prefer":"test-hook-manager"}],`+
		`"files":{"links":[{"source":`+jsonString(sourcePath)+`,"target":`+jsonString(targetPath)+`,"mode":"link"}]}`+
		`}`)

	var code int
	var stdout, stderr string
	stdout = captureStdout(t, func() {
		stderr = captureStderr(t, func() {
			code = run([]string{"apply", "--file", specPath, "--lock-file", lockPath, "--yes", "--no-hooks"})
		})
	})

	if code != exitLogic {
		t.Fatalf("apply with mismatch: expected exitLogic (%d), got %d\nstdout=%s\nstderr=%s", exitLogic, code, stdout, stderr)
	}
	combined := stdout + stderr
	if !strings.Contains(combined, "mismatch") {
		t.Fatalf("expected mismatch in output; stdout=%s stderr=%s", stdout, stderr)
	}
	if !strings.Contains(combined, targetPath) && !strings.Contains(combined, filepath.Base(targetPath)) {
		t.Fatalf("expected mismatch path in output; stdout=%s stderr=%s", stdout, stderr)
	}
	got, err := os.ReadFile(installLog)
	if err != nil {
		t.Fatalf("package install should still run despite file mismatch; read install log: %v\nstdout=%s stderr=%s", err, stdout, stderr)
	}
	if string(got) != "install" {
		t.Fatalf("install log = %q, want install", got)
	}
	fi, err := os.Lstat(targetPath)
	if err != nil {
		t.Fatalf("lstat target: %v", err)
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		t.Fatal("target became a symlink without --force")
	}
}

func TestApply_PackageFailureStillAppliesFiles(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	testutil.SetHome(t, dir)
	withFailingBrewInstall(t)
	spec := filepath.Join(dir, "genv.json")
	lock := filepath.Join(dir, "genv.lock.json")
	src := filepath.Join(dir, "src.txt")
	dst := filepath.Join(dir, "dst.txt")
	writeTestFile(t, src, "hello\n")
	writeTestFile(t, spec, `{`+
		`"schemaVersion":"6",`+
		`"packages":[{"id":"git","prefer":"brew"}],`+
		`"files":{"links":[{"source":`+jsonString(src)+`,"target":`+jsonString(dst)+`,"mode":"link"}]}`+
		`}`)
	writeLock(t, lock, nil)

	code := run([]string{"apply", "--file", spec, "--lock-file", lock, "--yes", "--no-hooks"})
	if code == exitOK {
		t.Fatal("expected apply to fail when brew install fails")
	}
	fi, err := os.Lstat(dst)
	if err != nil {
		t.Fatalf("file not applied after package error (exit %d): %v", code, err)
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		t.Fatal("expected dst to be a symlink")
	}
}

// skipPackagesListAdapter records ListInstalled so --skip-packages can prove
// it does not inventory managers just to render the unchanged package table.
type skipPackagesListAdapter struct {
	lifecycleHookAdapter
	listCalls int
}

func (a *skipPackagesListAdapter) ListInstalled() ([]string, error) {
	a.listCalls++
	return nil, nil
}

func registerSkipPackagesListAdapter(t *testing.T, a *skipPackagesListAdapter) {
	t.Helper()
	originalAll := adapter.All
	originalKnown := schema.KnownManagers["test-hook-manager"]
	adapter.All = append([]adapter.Adapter{a}, originalAll...)
	schema.KnownManagers["test-hook-manager"] = true
	t.Cleanup(func() {
		adapter.All = originalAll
		if originalKnown {
			schema.KnownManagers["test-hook-manager"] = true
		} else {
			delete(schema.KnownManagers, "test-hook-manager")
		}
	})
}

func TestApplyAndStatus_recordsAndReportsManagedLinkContentHash(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "xdg"))
	testutil.SetHome(t, dir)
	spec := filepath.Join(dir, "genv.json")
	lock := filepath.Join(dir, "genv.lock.json")
	src := filepath.Join(dir, "npmrc")
	dst := filepath.Join(dir, ".npmrc")
	writeTestFile(t, src, "registry=https://registry.npmjs.org/\n")
	writeTestFile(t, spec, `{`+
		`"schemaVersion":"6",`+
		`"files":{"links":[{"source":`+jsonString(src)+`,"target":`+jsonString(dst)+`,"mode":"managed-link"}]}`+
		`}`)

	if code := run([]string{"apply", "--file", spec, "--lock-file", lock, "--yes", "--skip-packages"}); code != exitOK {
		t.Fatalf("apply: expected exitOK (%d), got %d", exitOK, code)
	}
	lf, err := genvfile.ReadLock(lock)
	if err != nil {
		t.Fatalf("read lock: %v", err)
	}
	if len(lf.Files) != 1 || lf.Files[0].ContentHash == "" {
		t.Fatalf("lock files = %#v, want one entry with contentHash", lf.Files)
	}
	appliedHash := lf.Files[0].ContentHash

	var statusCode int
	statusOut := captureStdout(t, func() {
		statusCode = run([]string{"status", "--files", "--file", spec, "--lock-file", lock})
	})
	if statusCode != exitOK {
		t.Fatalf("status after apply: exit %d, want 0\n%s", statusCode, statusOut)
	}

	writeTestFile(t, src, "registry=https://registry.npmjs.org/\n# jamf\n")
	statusOut = captureStdout(t, func() {
		statusCode = run([]string{"status", "--files", "--file", spec, "--lock-file", lock})
	})
	if statusCode != exitLogic {
		t.Fatalf("status after source edit: exit %d, want %d\n%s", statusCode, exitLogic, statusOut)
	}
	if !strings.Contains(statusOut, "drifted") {
		t.Fatalf("status after source edit: expected drifted, got %q", statusOut)
	}
	if !strings.Contains(statusOut, dst) && !strings.Contains(statusOut, filepath.Base(dst)) {
		t.Fatalf("status after source edit: expected path %q in output %q", dst, statusOut)
	}

	got, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("read source: %v", err)
	}
	if !strings.Contains(string(got), "jamf") {
		t.Fatalf("apply must not revert source; got %q", got)
	}

	if code := run([]string{"apply", "--file", spec, "--lock-file", lock, "--yes", "--skip-packages"}); code != exitOK {
		t.Fatalf("re-apply: expected exitOK, got %d", code)
	}
	got, err = os.ReadFile(src)
	if err != nil {
		t.Fatalf("read source after re-apply: %v", err)
	}
	if !strings.Contains(string(got), "jamf") {
		t.Fatalf("re-apply reverted source to %q", got)
	}
	lf, err = genvfile.ReadLock(lock)
	if err != nil {
		t.Fatalf("read lock after re-apply: %v", err)
	}
	if len(lf.Files) != 1 || lf.Files[0].ContentHash == "" || lf.Files[0].ContentHash == appliedHash {
		t.Fatalf("re-apply should refresh contentHash; before=%q after=%#v", appliedHash, lf.Files)
	}

	statusOut = captureStdout(t, func() {
		statusCode = run([]string{"status", "--files", "--file", spec, "--lock-file", lock})
	})
	if statusCode != exitOK {
		t.Fatalf("status after re-apply: exit %d, want 0\n%s", statusCode, statusOut)
	}
}

func TestApply_SkipPackagesStillAppliesFiles(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	testutil.SetHome(t, dir)
	lister := &skipPackagesListAdapter{}
	registerSkipPackagesListAdapter(t, lister)
	spec := filepath.Join(dir, "genv.json")
	lock := filepath.Join(dir, "genv.lock.json")
	src := filepath.Join(dir, "src.txt")
	dst := filepath.Join(dir, "dst.txt")
	writeTestFile(t, src, "hello\n")
	writeTestFile(t, spec, `{`+
		`"schemaVersion":"6",`+
		`"packages":[`+
		`{"id":"cursor","prefer":"test-hook-manager"},`+
		`{"id":"extra","prefer":"test-hook-manager"}`+
		`],`+
		`"files":{"links":[{"source":`+jsonString(src)+`,"target":`+jsonString(dst)+`,"mode":"link"}]}`+
		`}`)
	writeLock(t, lock, []genvfile.LockedPackage{
		{ID: "cursor", Manager: "test-hook-manager", PkgName: "cursor"},
	})

	var jsonCode int
	jsonOut := captureStdout(t, func() {
		jsonCode = run([]string{"apply", "--file", spec, "--lock-file", lock, "--dry-run", "--json", "--skip-packages"})
	})
	if jsonCode != exitOK {
		t.Fatalf("dry-run json exit %d\n%s", jsonCode, jsonOut)
	}
	var env map[string]any
	if err := json.Unmarshal([]byte(jsonOut), &env); err != nil {
		t.Fatalf("dry-run json: %v\n%s", err, jsonOut)
	}
	data, _ := env["data"].(map[string]any)
	if data == nil {
		t.Fatalf("dry-run json missing data: %s", jsonOut)
	}
	for _, key := range []string{"toInstall", "toRemove", "unchanged"} {
		got, _ := data[key].([]any)
		if len(got) != 0 {
			t.Fatalf("dry-run json %s = %#v, want empty package plan", key, data[key])
		}
	}
	if adopted, ok := data["adopted"]; ok {
		if items, _ := adopted.([]any); len(items) != 0 {
			t.Fatalf("dry-run json adopted = %#v, want omitted or empty", adopted)
		}
	}
	files, _ := data["files"].([]any)
	if len(files) == 0 {
		t.Fatalf("dry-run json should still plan files; got %s", jsonOut)
	}

	var code int
	var stdout string
	stdout = captureStdout(t, func() {
		code = run([]string{"apply", "--file", spec, "--lock-file", lock, "--yes", "--no-hooks", "--skip-packages"})
	})
	if code != exitOK {
		t.Fatalf("exit %d\n%s", code, stdout)
	}
	if strings.Contains(stdout, "(up to date)") {
		t.Fatalf("skip-packages must not print the per-package table; got:\n%s", stdout)
	}
	if strings.Contains(stdout, "package") {
		t.Fatalf("skip-packages header must not count packages; got:\n%s", stdout)
	}
	if !strings.Contains(stdout, "files") || !strings.Contains(stdout, "env") || !strings.Contains(stdout, "services") {
		t.Fatalf("skip-packages header should name files/env/services; got:\n%s", stdout)
	}
	if lister.listCalls != 0 {
		t.Fatalf("skip-packages must not inventory live managers; ListInstalled calls = %d", lister.listCalls)
	}
	if _, err := os.Lstat(dst); err != nil {
		t.Fatalf("link not applied: %v", err)
	}
	lf, err := genvfile.ReadLock(lock)
	if err != nil {
		t.Fatal(err)
	}
	var sawCursor bool
	for _, p := range lf.Packages {
		if p.ID == "extra" {
			t.Fatal("skip-packages must not lock/install extra")
		}
		if p.ID == "cursor" {
			sawCursor = true
		}
	}
	if !sawCursor {
		t.Fatal("skip-packages must leave already-locked cursor in the lock")
	}
}

func TestApply_DryRunTextShowsFilePaths(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "xdg"))
	testutil.SetHome(t, dir)
	specPath := filepath.Join(dir, "genv.json")
	lockPath := filepath.Join(dir, "genv.lock.json")
	sourcePath := filepath.Join(dir, "source.txt")
	targetPath := filepath.Join(dir, "target.txt")

	writeTestFile(t, sourcePath, "desired\n")
	writeTestFile(t, targetPath, "blocking\n")
	writeTestFile(t, specPath, `{`+
		`"schemaVersion":"5",`+
		`"files":{"links":[{"source":`+jsonString(sourcePath)+`,"target":`+jsonString(targetPath)+`,"mode":"link"}]}`+
		`}`)

	var code int
	stdout := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			code = run([]string{"apply", "--file", specPath, "--lock-file", lockPath, "--dry-run", "--yes"})
		})
	})
	if code != exitOK && code != exitLogic {
		t.Fatalf("dry-run: unexpected exit %d\n%s", code, stdout)
	}
	if !strings.Contains(stdout, targetPath) {
		t.Fatalf("dry-run text should name file path; got: %s", stdout)
	}
	if !strings.Contains(stdout, "mismatch") {
		t.Fatalf("dry-run text should include mismatch kind; got: %s", stdout)
	}
}

func TestApply_ForceBackupFlagBacksUpWithoutPerEntryBackup(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "xdg"))
	testutil.SetHome(t, dir)
	specPath := filepath.Join(dir, "genv.json")
	lockPath := filepath.Join(dir, "genv.lock.json")
	sourcePath := filepath.Join(dir, "source.txt")
	targetPath := filepath.Join(dir, "target.txt")

	writeTestFile(t, sourcePath, "desired\n")
	writeTestFile(t, targetPath, "old-content\n")
	writeTestFile(t, specPath, `{`+
		`"schemaVersion":"5",`+
		`"files":{"links":[{"source":`+jsonString(sourcePath)+`,"target":`+jsonString(targetPath)+`,"mode":"link"}]}`+
		`}`)

	code := run([]string{"apply", "--file", specPath, "--lock-file", lockPath, "--force", "--backup", "--yes", "--no-hooks"})
	if code != exitOK {
		t.Fatalf("apply --force --backup: expected exitOK, got %d", code)
	}
	fi, err := os.Lstat(targetPath)
	if err != nil {
		t.Fatalf("lstat: %v", err)
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		t.Fatal("expected symlink after --force")
	}
	matches, err := filepath.Glob(targetPath + ".backup.*")
	if err != nil {
		t.Fatalf("glob backup: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected one backup file, got %v", matches)
	}
}

func TestApply_BrewLockDesyncAlreadyAbsentRecovers(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "xdg"))
	testutil.SetHome(t, dir)
	withAbsentBrewPackage(t)
	specPath := filepath.Join(dir, "genv.json")
	lockPath := filepath.Join(dir, "genv.lock.json")
	writeTestFile(t, specPath, `{"schemaVersion":"6","packages":[]}`)
	writeLock(t, lockPath, []genvfile.LockedPackage{
		{ID: "copilot-cli", Manager: "brew", PkgName: "copilot-cli"},
	})

	var code int
	var stderr string
	_ = captureStdout(t, func() {
		stderr = captureStderr(t, func() {
			code = run([]string{"apply", "--file", specPath, "--lock-file", lockPath, "--yes", "--no-hooks"})
		})
	})
	if code != exitOK {
		t.Fatalf("apply brew lock desync: expected exitOK, got %d; stderr=%s", code, stderr)
	}

	lf, err := genvfile.ReadLock(lockPath)
	if err != nil {
		t.Fatalf("read lock: %v", err)
	}
	for _, p := range lf.Packages {
		if p.ID == "copilot-cli" {
			t.Fatalf("lock still lists copilot-cli after apply; stderr=%s", stderr)
		}
	}
}

func TestApply_PackageRemovalFailureRunsPostApplyWithoutMismatchSkip(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "xdg"))
	testutil.SetHome(t, dir)
	specPath := filepath.Join(dir, "genv.json")
	lockPath := filepath.Join(dir, "genv.lock.json")
	hookLog := filepath.Join(dir, "hook.log")
	registerLifecycleHookAdapter(t, lifecycleHookAdapter{failUninstall: true})

	writeTestFile(t, specPath, `{`+
		`"schemaVersion":"6",`+
		`"packages":[],`+
		`"hooks":{"postApply":[{"command":`+jsonString("printf ran >> "+strconv.Quote(hookLog))+`}]}`+
		`}`)
	writeLock(t, lockPath, []genvfile.LockedPackage{
		{ID: "alpha", Manager: "test-hook-manager", PkgName: "alpha"},
	})

	var code int
	var stderr string
	_ = captureStdout(t, func() {
		stderr = captureStderr(t, func() {
			code = run([]string{"apply", "--file", specPath, "--lock-file", lockPath, "--yes"})
		})
	})
	if code != exitLogic {
		t.Fatalf("expected exitLogic on uninstall failure, got %d; stderr=%s", code, stderr)
	}
	if !strings.Contains(stderr, `remove "alpha"`) && !strings.Contains(stderr, "uninstall") {
		t.Fatalf("stderr should name the package removal failure; got %q", stderr)
	}
	if strings.Contains(stderr, "unresolved file mismatches") {
		t.Fatalf("package removal failure must not be reported as a file mismatch; stderr=%q", stderr)
	}
	got, err := os.ReadFile(hookLog)
	if err != nil {
		t.Fatalf("post-apply hook should still run when uninstall fails without a file mismatch: %v; stderr=%s", err, stderr)
	}
	if string(got) != "ran" {
		t.Fatalf("hook log = %q, want ran", got)
	}
}

func TestApply_FileMismatchSkipsPostApplyHooksWithMessage(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "xdg"))
	testutil.SetHome(t, dir)
	specPath := filepath.Join(dir, "genv.json")
	lockPath := filepath.Join(dir, "genv.lock.json")
	goodSource := filepath.Join(dir, "good-src.txt")
	goodTarget := filepath.Join(dir, "good-tgt.txt")
	badSource := filepath.Join(dir, "bad-src.txt")
	badTarget := filepath.Join(dir, "bad-tgt.txt")
	hookLog := filepath.Join(dir, "hook.log")

	writeTestFile(t, goodSource, "good\n")
	writeTestFile(t, badSource, "desired\n")
	writeTestFile(t, badTarget, "blocking\n")
	writeTestFile(t, specPath, `{`+
		`"schemaVersion":"6",`+
		`"files":{"links":[`+
		`{"source":`+jsonString(goodSource)+`,"target":`+jsonString(goodTarget)+`,"mode":"link"},`+
		`{"source":`+jsonString(badSource)+`,"target":`+jsonString(badTarget)+`,"mode":"link"}`+
		`]},`+
		`"hooks":{"postApply":[{"command":`+jsonString("printf ran >> "+strconv.Quote(hookLog))+`}]}`+
		`}`)

	var code int
	var stderr string
	_ = captureStdout(t, func() {
		stderr = captureStderr(t, func() {
			code = run([]string{"apply", "--file", specPath, "--lock-file", lockPath, "--yes"})
		})
	})
	if code != exitLogic {
		t.Fatalf("expected exitLogic, got %d; stderr=%s", code, stderr)
	}
	if _, err := os.Lstat(goodTarget); err != nil {
		t.Fatalf("good link should still be created: %v", err)
	}
	if _, err := os.Stat(hookLog); !os.IsNotExist(err) {
		t.Fatalf("post-apply hook should not run on unresolved mismatch; err=%v", err)
	}
	if !strings.Contains(stderr, "skipping post-apply hooks") || !strings.Contains(stderr, "mismatch") {
		t.Fatalf("stderr should explain skipped hooks due to mismatches; got %q", stderr)
	}
}

func TestApply_DryRunSourceRootUsesCanonicalTree(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "xdg"))
	testutil.SetHome(t, dir)

	liveDir := filepath.Join(dir, "live")
	workDir := filepath.Join(dir, "work")
	keepSrc := filepath.Join(liveDir, "keep.txt")
	hushSrc := filepath.Join(liveDir, "hushlogin")
	keepTarget := filepath.Join(dir, "home-keep")
	hushTarget := filepath.Join(dir, "home-hush")
	writeTestFile(t, keepSrc, "keep\n")
	writeTestFile(t, hushSrc, "new hush\n")
	if err := os.Symlink(keepSrc, keepTarget); err != nil {
		t.Fatalf("symlink keep: %v", err)
	}
	writeTestFile(t, hushTarget, "old hush\n")

	spec := `{` +
		`"schemaVersion":"5",` +
		`"files":{"links":[` +
		`{"source":"keep.txt","target":` + jsonString(keepTarget) + `,"mode":"managed-link"},` +
		`{"source":"hushlogin","target":` + jsonString(hushTarget) + `,"mode":"link"}` +
		`]}}`
	writeTestFile(t, filepath.Join(liveDir, "genv.json"), spec)
	writeTestFile(t, filepath.Join(workDir, "genv.json"), spec)

	workSpec := filepath.Join(workDir, "genv.json")
	lockPath := filepath.Join(dir, "genv.lock.json")

	var code int
	stdout := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			code = run([]string{"apply", "--file", workSpec, "--lock-file", lockPath, "--dry-run", "--json", "--yes"})
		})
	})
	if code != exitOK && code != exitLogic {
		t.Fatalf("dry-run without source-root: exit %d\n%s", code, stdout)
	}
	kinds := applyJSONFileKinds(t, stdout)
	if kinds[keepTarget] == "ok" {
		t.Fatalf("without --source-root, keep link should resolve to the spec copy and not be ok; got %#v\n%s", kinds, stdout)
	}

	stdout = captureStdout(t, func() {
		_ = captureStderr(t, func() {
			code = run([]string{"apply", "--file", workSpec, "--lock-file", lockPath, "--source-root", liveDir, "--dry-run", "--json", "--yes"})
		})
	})
	if code != exitOK && code != exitLogic {
		t.Fatalf("dry-run with source-root: exit %d\n%s", code, stdout)
	}
	kinds = applyJSONFileKinds(t, stdout)
	if kinds[keepTarget] != "ok" {
		t.Fatalf("with --source-root, existing keep link should be ok; got %#v\n%s", kinds, stdout)
	}
	if kinds[hushTarget] != "mismatch" {
		t.Fatalf("real content mismatch should still show; got %#v\n%s", kinds, stdout)
	}
}

func TestApply_WetApplyHonorsSourceRoot(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "xdg"))
	testutil.SetHome(t, dir)

	liveDir := filepath.Join(dir, "live")
	workDir := filepath.Join(dir, "work")
	liveSrc := filepath.Join(liveDir, "src.txt")
	workSrc := filepath.Join(workDir, "src.txt")
	target := filepath.Join(dir, "linked.txt")
	writeTestFile(t, liveSrc, "from-live\n")
	writeTestFile(t, workSrc, "from-work\n")

	spec := `{` +
		`"schemaVersion":"5",` +
		`"files":{"links":[{"source":"src.txt","target":` + jsonString(target) + `,"mode":"link"}]}` +
		`}`
	writeTestFile(t, filepath.Join(workDir, "genv.json"), spec)

	code := run([]string{
		"apply", "--file", filepath.Join(workDir, "genv.json"),
		"--lock-file", filepath.Join(dir, "genv.lock.json"),
		"--source-root", liveDir, "--yes", "--no-hooks",
	})
	if code != exitOK {
		t.Fatalf("wet apply --source-root: expected exitOK, got %d", code)
	}
	got, err := os.Readlink(target)
	if err != nil {
		t.Fatalf("target should be a symlink: %v", err)
	}
	if got != liveSrc {
		t.Fatalf("symlink points to %q, want live tree %q", got, liveSrc)
	}
}

func TestApply_SourceRootMissingIsRefused(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "xdg"))
	testutil.SetHome(t, dir)
	specPath := filepath.Join(dir, "genv.json")
	writeTestFile(t, specPath, `{"schemaVersion":"5","packages":[]}`)

	var code int
	stderr := captureStderr(t, func() {
		_ = captureStdout(t, func() {
			code = run([]string{
				"apply", "--file", specPath,
				"--lock-file", filepath.Join(dir, "genv.lock.json"),
				"--source-root", filepath.Join(dir, "missing-root"),
				"--dry-run", "--yes",
			})
		})
	})
	if code == exitOK {
		t.Fatalf("missing --source-root should fail; stderr=%s", stderr)
	}
	if !strings.Contains(stderr, "source-root") {
		t.Fatalf("error should name --source-root; stderr=%s", stderr)
	}
}

func applyJSONFileKinds(t *testing.T, stdout string) map[string]string {
	t.Helper()
	var env struct {
		Data struct {
			Files []struct {
				Target string `json:"target"`
				Kind   string `json:"kind"`
			} `json:"files"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(stdout), &env); err != nil {
		t.Fatalf("apply --json: %v\n%s", err, stdout)
	}
	out := make(map[string]string, len(env.Data.Files))
	for _, f := range env.Data.Files {
		out[f.Target] = f.Kind
	}
	return out
}

func jsonString(s string) string {
	b, err := json.Marshal(s)
	if err != nil {
		panic(err)
	}
	return string(b)
}

func jsonHook(cmd string) string {
	return `"command":` + jsonString(cmd)
}

func hookAppend(path, word string) string {
	if runtime.GOOS == "windows" {
		return "Add-Content -LiteralPath " + psSingleQuote(path) + " -Value " + psSingleQuote(word) + " -NoNewline"
	}
	return "printf " + word + " >> " + strconv.Quote(path)
}

func hookAppendFail(path, word string) string {
	return hookAppend(path, word) + "; exit 99"
}

func normalizeHookLog(b []byte) string {
	return strings.ReplaceAll(string(b), "\r\n", "\n")
}

func skipIfWindowsNoUnixPerms(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("unix file modes are not enforced on Windows")
	}
}

func hookPrintEnvLine(path string, vars ...string) string {
	if runtime.GOOS == "windows" {
		parts := make([]string, len(vars))
		for i, v := range vars {
			parts[i] = "$env:" + v
		}
		return "Add-Content -LiteralPath " + psSingleQuote(path) + " -Value ((" + strings.Join(parts, ",") + ") -join ':')"
	}
	format := strings.Repeat("%s:", len(vars))
	format = strings.TrimSuffix(format, ":") + `\n`
	args := make([]string, len(vars))
	for i, v := range vars {
		args[i] = `"$` + v + `"`
	}
	return "printf '" + format + "' " + strings.Join(args, " ") + " >> " + strconv.Quote(path)
}

func psSingleQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}
