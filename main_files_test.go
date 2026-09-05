package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ks1686/genv/internal/genvfile"
	"github.com/ks1686/genv/internal/testutil"
)

func TestFilesCmd_usageAndUnknown(t *testing.T) {
	if code := run([]string{"files"}); code != exitUsage {
		t.Fatalf("files with no subcommand: got %d, want %d", code, exitUsage)
	}
	if code := run([]string{"files", "unknown"}); code != exitUsage {
		t.Fatalf("files unknown: got %d, want %d", code, exitUsage)
	}
	if code := run([]string{"files", "adopt"}); code != exitUsage {
		t.Fatalf("files adopt missing target: got %d, want %d", code, exitUsage)
	}
}

func TestApply_PerEntryBackupReplacesWithoutForce(t *testing.T) {
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
		`"files":{"links":[{"source":`+jsonString(sourcePath)+`,"target":`+jsonString(targetPath)+`,"mode":"managed-link","backup":true}]}`+
		`}`)

	code := run([]string{"apply", "--file", specPath, "--lock-file", lockPath, "--yes", "--no-hooks"})
	if code != exitOK {
		t.Fatalf("apply with per-entry backup: expected exitOK, got %d", code)
	}
	fi, err := os.Lstat(targetPath)
	if err != nil {
		t.Fatalf("lstat: %v", err)
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		t.Fatal("expected symlink after per-entry backup replace")
	}
	matches, err := filepath.Glob(targetPath + ".backup.*")
	if err != nil {
		t.Fatalf("glob backup: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected one backup file, got %v", matches)
	}
}

func TestApply_PerEntryBackupLeavesOtherMismatches(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "xdg"))
	testutil.SetHome(t, dir)
	specPath := filepath.Join(dir, "genv.json")
	lockPath := filepath.Join(dir, "genv.lock.json")
	replaceSrc := filepath.Join(dir, "replace-src.txt")
	keepSrc := filepath.Join(dir, "keep-src.txt")
	replaceDst := filepath.Join(dir, "replace.txt")
	keepDst := filepath.Join(dir, "keep.txt")

	writeTestFile(t, replaceSrc, "desired-replace\n")
	writeTestFile(t, keepSrc, "desired-keep\n")
	writeTestFile(t, replaceDst, "old-replace\n")
	writeTestFile(t, keepDst, "old-keep\n")
	writeTestFile(t, specPath, `{`+
		`"schemaVersion":"5",`+
		`"files":{"links":[`+
		`{"source":`+jsonString(replaceSrc)+`,"target":`+jsonString(replaceDst)+`,"mode":"managed-link","backup":true},`+
		`{"source":`+jsonString(keepSrc)+`,"target":`+jsonString(keepDst)+`,"mode":"managed-link"}`+
		`]}}`)

	var code int
	var stdout, stderr string
	stdout = captureStdout(t, func() {
		stderr = captureStderr(t, func() {
			code = run([]string{"apply", "--file", specPath, "--lock-file", lockPath, "--yes", "--no-hooks"})
		})
	})
	if code != exitLogic {
		t.Fatalf("mixed apply: expected exitLogic, got %d\nstdout=%s\nstderr=%s", code, stdout, stderr)
	}
	combined := stdout + stderr
	if !strings.Contains(combined, "mismatch") || !strings.Contains(combined, keepDst) {
		t.Fatalf("expected keep path reported as mismatch; stdout=%s stderr=%s", stdout, stderr)
	}

	fi, err := os.Lstat(replaceDst)
	if err != nil {
		t.Fatalf("lstat replace: %v", err)
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		t.Fatal("backup:true entry should become a symlink")
	}
	keepInfo, err := os.Lstat(keepDst)
	if err != nil {
		t.Fatalf("lstat keep: %v", err)
	}
	if keepInfo.Mode()&os.ModeSymlink != 0 {
		t.Fatal("entry without backup should remain a regular file")
	}
	got, err := os.ReadFile(keepDst)
	if err != nil {
		t.Fatalf("read keep: %v", err)
	}
	if string(got) != "old-keep\n" {
		t.Fatalf("keep target = %q, want old-keep", got)
	}
}

func TestFilesAdopt_seedsSourceAndRecordsLock(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "xdg"))
	testutil.SetHome(t, dir)
	specPath := filepath.Join(dir, "genv.json")
	lockPath := filepath.Join(dir, "genv.lock.json")
	sourcePath := filepath.Join(dir, "foo")
	targetPath := filepath.Join(dir, ".foo")

	writeTestFile(t, targetPath, "live-dotfile\n")
	writeTestFile(t, specPath, `{`+
		`"schemaVersion":"5",`+
		`"files":{"links":[{"source":`+jsonString(sourcePath)+`,"target":`+jsonString(targetPath)+`,"mode":"managed-link"}]}`+
		`}`)

	code := run([]string{"files", "adopt", targetPath, "--file", specPath, "--lock-file", lockPath})
	if code != exitOK {
		t.Fatalf("files adopt: expected exitOK, got %d", code)
	}

	gotSource, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatalf("seeded source missing: %v", err)
	}
	if string(gotSource) != "live-dotfile\n" {
		t.Fatalf("source = %q, want live-dotfile", gotSource)
	}
	gotLink, err := os.Readlink(targetPath)
	if err != nil {
		t.Fatalf("Readlink: %v", err)
	}
	if gotLink != sourcePath {
		t.Fatalf("symlink points to %q, want %q", gotLink, sourcePath)
	}
	matches, err := filepath.Glob(targetPath + ".backup.*")
	if err != nil {
		t.Fatalf("glob backup: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected one backup, got %v", matches)
	}

	lf, err := genvfile.ReadLock(lockPath)
	if err != nil {
		t.Fatalf("read lock: %v", err)
	}
	if len(lf.Files) != 1 {
		t.Fatalf("lock files = %#v, want 1", lf.Files)
	}
	if lf.Files[0].Target != targetPath || lf.Files[0].Source != sourcePath {
		t.Fatalf("lock entry = %#v", lf.Files[0])
	}
	if lf.Files[0].ContentHash == "" {
		t.Fatal("adopted lock missing contentHash")
	}
}

func TestFilesAdopt_dryRunPrintsThreeSteps(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "xdg"))
	testutil.SetHome(t, dir)
	specPath := filepath.Join(dir, "genv.json")
	lockPath := filepath.Join(dir, "genv.lock.json")
	sourcePath := filepath.Join(dir, "foo")
	targetPath := filepath.Join(dir, ".foo")

	writeTestFile(t, targetPath, "live-dotfile\n")
	writeTestFile(t, specPath, `{`+
		`"schemaVersion":"5",`+
		`"files":{"links":[{"source":`+jsonString(sourcePath)+`,"target":`+jsonString(targetPath)+`,"mode":"managed-link"}]}`+
		`}`)

	var code int
	stdout := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			code = run([]string{"files", "adopt", targetPath, "--file", specPath, "--lock-file", lockPath, "--dry-run"})
		})
	})
	if code != exitOK {
		t.Fatalf("files adopt --dry-run: expected exitOK, got %d\n%s", code, stdout)
	}
	if !strings.Contains(stdout, "copy") || !strings.Contains(stdout, "backup") || !strings.Contains(stdout, "link") {
		t.Fatalf("dry-run should print copy/backup/link steps; got:\n%s", stdout)
	}
	if !strings.Contains(stdout, sourcePath) || !strings.Contains(stdout, targetPath) {
		t.Fatalf("dry-run should name source and target; got:\n%s", stdout)
	}
	if _, err := os.Stat(sourcePath); !os.IsNotExist(err) {
		t.Fatalf("dry-run must not seed source; stat = %v", err)
	}
	fi, err := os.Lstat(targetPath)
	if err != nil {
		t.Fatalf("lstat: %v", err)
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		t.Fatal("dry-run must not create the link")
	}
	if _, err := os.Stat(lockPath); !os.IsNotExist(err) {
		lf, readErr := genvfile.ReadLock(lockPath)
		if readErr != nil {
			t.Fatalf("read lock: %v", readErr)
		}
		if len(lf.Files) != 0 {
			t.Fatalf("dry-run must not record lock files, got %#v", lf.Files)
		}
	}
}
