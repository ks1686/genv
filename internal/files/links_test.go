package files

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ks1686/genv/internal/schema"
)

// sentinelSymlinkErr stands in for the Windows os.Symlink privilege error
// (Developer Mode off, unelevated); tests assert errors.Is reaches it.
//
//nolint:staticcheck // ST1005: intentional verbatim replica of the real Windows os.Symlink error string, which ends with a period.
var sentinelSymlinkErr = errors.New("A required privilege is not held by the client.")

func TestWindowsSymlinkHint_wrapsOnWindows(t *testing.T) {
	got := windowsSymlinkHint("windows", sentinelSymlinkErr)
	if got == nil {
		t.Fatal("windowsSymlinkHint(windows, err) = nil, want wrapped error")
	}
	if !errors.Is(got, sentinelSymlinkErr) {
		t.Fatalf("errors.Is could not reach original error through wrapper: %v", got)
	}
	msg := got.Error()
	if !strings.Contains(msg, sentinelSymlinkErr.Error()) {
		t.Fatalf("wrapped message %q does not contain original error %q", msg, sentinelSymlinkErr.Error())
	}
	lower := strings.ToLower(msg)
	if !strings.Contains(lower, "developer mode") {
		t.Fatalf("wrapped message %q missing 'Developer Mode' hint", msg)
	}
	if !strings.Contains(lower, "administrator") {
		t.Fatalf("wrapped message %q missing 'Administrator' hint", msg)
	}
}

func TestWindowsSymlinkHint_passthroughOnNonWindows(t *testing.T) {
	got := windowsSymlinkHint("linux", sentinelSymlinkErr)
	if got != sentinelSymlinkErr {
		t.Fatalf("windowsSymlinkHint(linux, err) = %v, want identical error value", got)
	}
	got = windowsSymlinkHint("darwin", sentinelSymlinkErr)
	if got != sentinelSymlinkErr {
		t.Fatalf("windowsSymlinkHint(darwin, err) = %v, want identical error value", got)
	}
}

func TestWindowsSymlinkHint_nilPassthrough(t *testing.T) {
	if got := windowsSymlinkHint("windows", nil); got != nil {
		t.Fatalf("windowsSymlinkHint(windows, nil) = %v, want nil", got)
	}
}

func setupSource(t *testing.T, home, name string) string {
	t.Helper()
	source := filepath.Join(home, "repo", name)
	if err := os.MkdirAll(filepath.Dir(source), 0o755); err != nil {
		t.Fatalf("mkdir source parent: %v", err)
	}
	if err := os.WriteFile(source, []byte("hello"), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}
	return source
}

func TestApply_createsMissingLink(t *testing.T) {
	home := t.TempDir()
	setTestHome(t, home)

	source := setupSource(t, home, "simple.txt")
	target := filepath.Join(home, ".genv-test", "simple.txt")
	cfg := &schema.FilesConfig{
		Links: []schema.FileLink{{
			Source: source,
			Target: "~/.genv-test/simple.txt",
			Mode:   "link",
		}},
	}

	res, err := Apply(context.Background(), cfg, "any", ApplyOptions{})
	if err != nil {
		t.Fatalf("Apply error = %v, want nil", err)
	}
	if len(res.Created) != 1 || res.Created[0] != target {
		t.Fatalf("Created = %v, want [%s]", res.Created, target)
	}
	got, err := os.Readlink(target)
	if err != nil {
		t.Fatalf("Readlink: %v", err)
	}
	if got != source {
		t.Fatalf("symlink points to %q, want %q", got, source)
	}
}

func TestApply_skipsCorrectLink(t *testing.T) {
	home := t.TempDir()
	setTestHome(t, home)

	source := setupSource(t, home, "simple.txt")
	target := filepath.Join(home, ".genv-test", "simple.txt")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("mkdir target parent: %v", err)
	}
	if err := os.Symlink(source, target); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	cfg := &schema.FilesConfig{
		Links: []schema.FileLink{{
			Source: source,
			Target: "~/.genv-test/simple.txt",
			Mode:   "link",
		}},
	}
	res, err := Apply(context.Background(), cfg, "any", ApplyOptions{})
	if err != nil {
		t.Fatalf("Apply error = %v, want nil", err)
	}
	if len(res.Skipped) != 1 || res.Skipped[0] != target {
		t.Fatalf("Skipped = %v, want [%s]", res.Skipped, target)
	}
}

func TestApply_linkMismatchNoForceLeavesFileUntouched(t *testing.T) {
	home := t.TempDir()
	setTestHome(t, home)

	source := setupSource(t, home, "simple.txt")
	target := filepath.Join(home, ".genv-test", "simple.txt")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("mkdir target parent: %v", err)
	}
	original := []byte("i am a hand-written file\n")
	if err := os.WriteFile(target, original, 0o644); err != nil {
		t.Fatalf("write planted file: %v", err)
	}

	cfg := &schema.FilesConfig{
		Links: []schema.FileLink{{
			Source: source,
			Target: "~/.genv-test/simple.txt",
			Mode:   "link",
		}},
	}
	res, err := Apply(context.Background(), cfg, "any", ApplyOptions{})
	if err == nil {
		t.Fatal("Apply error = nil, want mismatch error")
	}
	if len(res.Mismatched) != 1 || res.Mismatched[0] != target {
		t.Fatalf("Mismatched = %v, want [%s]", res.Mismatched, target)
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read target: %v", err)
	}
	if !bytes.Equal(got, original) {
		t.Fatalf("target was modified: got %q, want %q", got, original)
	}
	if fi, err := os.Lstat(target); err != nil || fi.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("target should remain a regular file")
	}
}

func TestApply_linkMismatchForceBackup(t *testing.T) {
	home := t.TempDir()
	setTestHome(t, home)

	source := setupSource(t, home, "simple.txt")
	target := filepath.Join(home, ".genv-test", "simple.txt")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("mkdir target parent: %v", err)
	}
	original := []byte("i am a hand-written file\n")
	if err := os.WriteFile(target, original, 0o644); err != nil {
		t.Fatalf("write planted file: %v", err)
	}

	cfg := &schema.FilesConfig{
		Links: []schema.FileLink{{
			Source: source,
			Target: "~/.genv-test/simple.txt",
			Mode:   "link",
			Backup: true,
		}},
	}
	res, err := Apply(context.Background(), cfg, "any", ApplyOptions{Force: true})
	if err != nil {
		t.Fatalf("Apply error = %v, want nil", err)
	}
	if len(res.Updated) != 1 || res.Updated[0] != target {
		t.Fatalf("Updated = %v, want [%s]", res.Updated, target)
	}
	got, err := os.Readlink(target)
	if err != nil {
		t.Fatalf("Readlink: %v", err)
	}
	if got != source {
		t.Fatalf("symlink points to %q, want %q", got, source)
	}
	matches, _ := filepath.Glob(target + ".backup.*")
	if len(matches) != 1 {
		t.Fatalf("expected one backup, got %v", matches)
	}
	backupData, err := os.ReadFile(matches[0])
	if err != nil {
		t.Fatalf("read backup: %v", err)
	}
	if !bytes.Equal(backupData, original) {
		t.Fatalf("backup data mismatch: got %q, want %q", backupData, original)
	}
}

func TestApply_managedLinkSelfHealsWrongSymlink(t *testing.T) {
	home := t.TempDir()
	setTestHome(t, home)

	source := setupSource(t, home, "simple.txt")
	wrongSource := setupSource(t, home, "other.txt")

	target := filepath.Join(home, ".genv-test", "simple.txt")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("mkdir target parent: %v", err)
	}
	if err := os.Symlink(wrongSource, target); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	cfg := &schema.FilesConfig{
		Links: []schema.FileLink{{
			Source: source,
			Target: "~/.genv-test/simple.txt",
			Mode:   "managed-link",
		}},
	}
	res, err := Apply(context.Background(), cfg, "any", ApplyOptions{})
	if err != nil {
		t.Fatalf("Apply error = %v, want nil", err)
	}
	if len(res.Updated) != 1 || res.Updated[0] != target {
		t.Fatalf("Updated = %v, want [%s]", res.Updated, target)
	}
	got, err := os.Readlink(target)
	if err != nil {
		t.Fatalf("Readlink: %v", err)
	}
	if got != source {
		t.Fatalf("symlink points to %q, want %q", got, source)
	}
}

func TestApply_linkMismatchBackupTrueReplacesWithoutForce(t *testing.T) {
	home := t.TempDir()
	setTestHome(t, home)

	source := setupSource(t, home, "simple.txt")
	target := filepath.Join(home, ".genv-test", "simple.txt")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("mkdir target parent: %v", err)
	}
	original := []byte("i am a hand-written file\n")
	if err := os.WriteFile(target, original, 0o644); err != nil {
		t.Fatalf("write planted file: %v", err)
	}

	cfg := &schema.FilesConfig{
		Links: []schema.FileLink{{
			Source: source,
			Target: "~/.genv-test/simple.txt",
			Mode:   "managed-link",
			Backup: true,
		}},
	}
	res, err := Apply(context.Background(), cfg, "any", ApplyOptions{})
	if err != nil {
		t.Fatalf("Apply error = %v, want nil", err)
	}
	if len(res.Updated) != 1 || res.Updated[0] != target {
		t.Fatalf("Updated = %v, want [%s]", res.Updated, target)
	}
	if len(res.Mismatched) != 0 {
		t.Fatalf("Mismatched = %v, want none", res.Mismatched)
	}
	got, err := os.Readlink(target)
	if err != nil {
		t.Fatalf("Readlink: %v", err)
	}
	if got != source {
		t.Fatalf("symlink points to %q, want %q", got, source)
	}
	matches, _ := filepath.Glob(target + ".backup.*")
	if len(matches) != 1 {
		t.Fatalf("expected one backup, got %v", matches)
	}
	backupData, err := os.ReadFile(matches[0])
	if err != nil {
		t.Fatalf("read backup: %v", err)
	}
	if !bytes.Equal(backupData, original) {
		t.Fatalf("backup data mismatch: got %q, want %q", backupData, original)
	}
}

func TestApply_mixedBackupOnlyReplacesBackupEntry(t *testing.T) {
	home := t.TempDir()
	setTestHome(t, home)

	replaceSource := setupSource(t, home, "replace.txt")
	keepSource := setupSource(t, home, "keep.txt")
	replaceTarget := filepath.Join(home, ".genv-test", "replace.txt")
	keepTarget := filepath.Join(home, ".genv-test", "keep.txt")
	if err := os.MkdirAll(filepath.Dir(replaceTarget), 0o755); err != nil {
		t.Fatalf("mkdir target parent: %v", err)
	}
	replaceOriginal := []byte("replace-me\n")
	keepOriginal := []byte("leave-me\n")
	if err := os.WriteFile(replaceTarget, replaceOriginal, 0o644); err != nil {
		t.Fatalf("write replace target: %v", err)
	}
	if err := os.WriteFile(keepTarget, keepOriginal, 0o644); err != nil {
		t.Fatalf("write keep target: %v", err)
	}

	cfg := &schema.FilesConfig{
		Links: []schema.FileLink{
			{
				Source: replaceSource,
				Target: "~/.genv-test/replace.txt",
				Mode:   "managed-link",
				Backup: true,
			},
			{
				Source: keepSource,
				Target: "~/.genv-test/keep.txt",
				Mode:   "managed-link",
			},
		},
	}
	res, err := Apply(context.Background(), cfg, "any", ApplyOptions{})
	if err == nil {
		t.Fatal("Apply error = nil, want mismatch error")
	}
	if len(res.Updated) != 1 || res.Updated[0] != replaceTarget {
		t.Fatalf("Updated = %v, want [%s]", res.Updated, replaceTarget)
	}
	if len(res.Mismatched) != 1 || res.Mismatched[0] != keepTarget {
		t.Fatalf("Mismatched = %v, want [%s]", res.Mismatched, keepTarget)
	}
	got, err := os.Readlink(replaceTarget)
	if err != nil {
		t.Fatalf("Readlink replace: %v", err)
	}
	if got != replaceSource {
		t.Fatalf("replace symlink points to %q, want %q", got, replaceSource)
	}
	keepData, err := os.ReadFile(keepTarget)
	if err != nil {
		t.Fatalf("read keep target: %v", err)
	}
	if !bytes.Equal(keepData, keepOriginal) {
		t.Fatalf("keep target was modified: got %q, want %q", keepData, keepOriginal)
	}
	if fi, err := os.Lstat(keepTarget); err != nil || fi.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("keep target should remain a regular file")
	}
}

func TestApply_globalBackupWithoutForceDoesNotReplace(t *testing.T) {
	home := t.TempDir()
	setTestHome(t, home)

	source := setupSource(t, home, "simple.txt")
	target := filepath.Join(home, ".genv-test", "simple.txt")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("mkdir target parent: %v", err)
	}
	original := []byte("leave-me\n")
	if err := os.WriteFile(target, original, 0o644); err != nil {
		t.Fatalf("write planted file: %v", err)
	}

	cfg := &schema.FilesConfig{
		Links: []schema.FileLink{{
			Source: source,
			Target: "~/.genv-test/simple.txt",
			Mode:   "managed-link",
		}},
	}
	res, err := Apply(context.Background(), cfg, "any", ApplyOptions{Backup: true})
	if err == nil {
		t.Fatal("Apply error = nil, want mismatch error")
	}
	if len(res.Mismatched) != 1 || res.Mismatched[0] != target {
		t.Fatalf("Mismatched = %v, want [%s]", res.Mismatched, target)
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read target: %v", err)
	}
	if !bytes.Equal(got, original) {
		t.Fatalf("target was modified: got %q, want %q", got, original)
	}
}

func TestApply_managedLinkRequiresForceForRealFile(t *testing.T) {
	home := t.TempDir()
	setTestHome(t, home)

	source := setupSource(t, home, "simple.txt")
	target := filepath.Join(home, ".genv-test", "simple.txt")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("mkdir target parent: %v", err)
	}
	if err := os.WriteFile(target, []byte("i am a file"), 0o644); err != nil {
		t.Fatalf("write planted file: %v", err)
	}

	cfg := &schema.FilesConfig{
		Links: []schema.FileLink{{
			Source: source,
			Target: "~/.genv-test/simple.txt",
			Mode:   "managed-link",
		}},
	}
	res, err := Apply(context.Background(), cfg, "any", ApplyOptions{})
	if err == nil {
		t.Fatal("Apply error = nil, want mismatch error")
	}
	if len(res.Mismatched) != 1 || res.Mismatched[0] != target {
		t.Fatalf("Mismatched = %v, want [%s]", res.Mismatched, target)
	}
}

func TestApply_forceLinkRefusesDirectoryWithoutBackup(t *testing.T) {
	home := t.TempDir()
	setTestHome(t, home)

	source := filepath.Join(home, "src.txt")
	if err := os.WriteFile(source, []byte("hello\n"), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}
	target := filepath.Join(home, ".genv-test", "simple.txt")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatalf("mkdir target: %v", err)
	}
	keep := filepath.Join(target, "keep")
	if err := os.WriteFile(keep, []byte("safe"), 0o644); err != nil {
		t.Fatalf("write keep: %v", err)
	}

	cfg := &schema.FilesConfig{
		Links: []schema.FileLink{{
			Source: source,
			Target: "~/.genv-test/simple.txt",
			Mode:   "link",
		}},
	}
	res, err := Apply(context.Background(), cfg, "any", ApplyOptions{Force: true})
	if err == nil {
		t.Fatal("error = nil, want refusing directory")
	}
	var msg string
	if res != nil && len(res.Errors) > 0 {
		msg = res.Errors[0].Error()
	} else {
		msg = err.Error()
	}
	if !strings.Contains(msg, "refusing to replace directory") {
		t.Fatalf("error = %v, want refusing directory", msg)
	}
	if _, statErr := os.Stat(keep); statErr != nil {
		t.Fatalf("directory contents were deleted: %v", statErr)
	}
}

func TestApply_forceLinkReplacesFileWithoutBackup(t *testing.T) {
	home := t.TempDir()
	setTestHome(t, home)

	source := filepath.Join(home, "src.txt")
	if err := os.WriteFile(source, []byte("hello\n"), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}
	target := filepath.Join(home, ".genv-test", "simple.txt")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(target, []byte("old"), 0o644); err != nil {
		t.Fatalf("write planted file: %v", err)
	}

	cfg := &schema.FilesConfig{
		Links: []schema.FileLink{{
			Source: source,
			Target: "~/.genv-test/simple.txt",
			Mode:   "link",
		}},
	}
	res, err := Apply(context.Background(), cfg, "any", ApplyOptions{Force: true})
	if err != nil {
		t.Fatalf("Apply error = %v, want nil", err)
	}
	if len(res.Updated) != 1 || res.Updated[0] != target {
		t.Fatalf("Updated = %v, want [%s]", res.Updated, target)
	}
	got, err := os.Readlink(target)
	if err != nil {
		t.Fatalf("Readlink: %v", err)
	}
	if got != source {
		t.Fatalf("symlink points to %q, want %q", got, source)
	}
	matches, _ := filepath.Glob(target + ".backup.*")
	if len(matches) != 0 {
		t.Fatalf("expected no backup without --backup, got %v", matches)
	}
}
