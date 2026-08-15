package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

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
