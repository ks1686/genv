package files

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ks1686/genv/internal/schema"
)

func TestApplyDir_DryRunReportsFileReplacement(t *testing.T) {
	target := filepath.Join(t.TempDir(), "target")
	if err := os.WriteFile(target, []byte("file"), 0o644); err != nil {
		t.Fatalf("write target: %v", err)
	}
	res := &ApplyResult{}

	if err := applyDir(context.Background(), schema.FileDir{Target: target}, ApplyOptions{DryRun: true}, res); err != nil {
		t.Fatalf("applyDir dry run: %v", err)
	}
	if len(res.Updated) != 1 || res.Updated[0] != target {
		t.Errorf("Updated = %v, want [%s]", res.Updated, target)
	}
	if info, err := os.Stat(target); err != nil || info.IsDir() {
		t.Errorf("dry run changed target: info=%v err=%v", info, err)
	}
}

func TestBackupExistingAndCollisionPath(t *testing.T) {
	target := filepath.Join(t.TempDir(), "target")
	if err := os.WriteFile(target, []byte("original"), 0o644); err != nil {
		t.Fatalf("write target: %v", err)
	}
	first := backupPathFor(target)
	if err := os.WriteFile(first, []byte("existing backup"), 0o644); err != nil {
		t.Fatalf("write existing backup: %v", err)
	}
	second := backupPathFor(target)
	if !strings.HasPrefix(second, first+".") {
		t.Fatalf("collision backup path = %q, want suffix after %q", second, first)
	}

	if err := backupExisting(target); err != nil {
		t.Fatalf("backupExisting: %v", err)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Errorf("target stat = %v, want not exist", err)
	}
	if got, err := os.ReadFile(second); err != nil || string(got) != "original" {
		t.Errorf("backup contents = %q, err = %v", got, err)
	}
}

func TestApplyDir_CreateSkipMismatchAndForce(t *testing.T) {
	dir := t.TempDir()

	created := filepath.Join(dir, "newdir")
	res := &ApplyResult{}
	if err := applyDir(context.Background(), schema.FileDir{Target: created}, ApplyOptions{}, res); err != nil {
		t.Fatalf("create dir: %v", err)
	}
	if len(res.Created) != 1 {
		t.Fatalf("Created = %v", res.Created)
	}
	res = &ApplyResult{}
	if err := applyDir(context.Background(), schema.FileDir{Target: created}, ApplyOptions{}, res); err != nil {
		t.Fatalf("skip existing dir: %v", err)
	}
	if len(res.Skipped) != 1 {
		t.Fatalf("Skipped = %v", res.Skipped)
	}

	fileTarget := filepath.Join(dir, "file-as-dir")
	if err := os.WriteFile(fileTarget, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	res = &ApplyResult{}
	if err := applyDir(context.Background(), schema.FileDir{Target: fileTarget}, ApplyOptions{}, res); err != nil {
		t.Fatalf("mismatch: %v", err)
	}
	if len(res.Mismatched) != 1 {
		t.Fatalf("Mismatched = %v", res.Mismatched)
	}

	res = &ApplyResult{}
	if err := applyDir(context.Background(), schema.FileDir{Target: fileTarget}, ApplyOptions{Force: true}, res); err != nil {
		t.Fatalf("force replace: %v", err)
	}
	if info, err := os.Stat(fileTarget); err != nil || !info.IsDir() {
		t.Fatalf("force replace result: info=%v err=%v", info, err)
	}

	fileTarget2 := filepath.Join(dir, "file-as-dir-backup")
	if err := os.WriteFile(fileTarget2, []byte("y"), 0o644); err != nil {
		t.Fatal(err)
	}
	res = &ApplyResult{}
	if err := applyDir(context.Background(), schema.FileDir{Target: fileTarget2}, ApplyOptions{Force: true, Backup: true}, res); err != nil {
		t.Fatalf("force+backup: %v", err)
	}
	if info, err := os.Stat(fileTarget2); err != nil || !info.IsDir() {
		t.Fatalf("force+backup result: info=%v err=%v", info, err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := applyDir(ctx, schema.FileDir{Target: filepath.Join(dir, "canceled")}, ApplyOptions{}, &ApplyResult{}); err == nil {
		t.Fatal("expected canceled context error")
	}
}
