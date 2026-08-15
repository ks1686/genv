package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ks1686/genv/internal/genvfile"
	"github.com/ks1686/genv/internal/output"
	"github.com/ks1686/genv/internal/profile"
	"github.com/ks1686/genv/internal/schema"
)

func writeLegacyProfileBase(t *testing.T, specPath string, pkgs ...schema.Package) {
	t.Helper()
	base := &schema.GenvFile{SchemaVersion: schema.Version6, Packages: pkgs}
	if base.Packages == nil {
		base.Packages = []schema.Package{}
	}
	if err := genvfile.Write(specPath, base); err != nil {
		t.Fatal(err)
	}
}

func TestProfileList(t *testing.T) {
	dir := t.TempDir()
	specPath := filepath.Join(dir, "genv.json")
	lockPath := filepath.Join(dir, "genv.lock.json")

	writeLegacyProfileBase(t, specPath)
	if err := profile.Create(specPath, "work"); err != nil {
		t.Fatal(err)
	}

	// Test list with no active profile
	out := captureStdout(t, func() {
		code := run([]string{"profile", "list", "-file", specPath, "-lock-file", lockPath})
		if code != exitOK {
			t.Errorf("expected exitOK, got %d", code)
		}
	})
	if !strings.Contains(out, "* base") {
		t.Errorf("expected '* base', got %q", out)
	}
	if !strings.Contains(out, "  work") {
		t.Errorf("expected '  work', got %q", out)
	}

	// Set active profile to "work"
	lf := &genvfile.LockFile{SchemaVersion: schema.Version6, ActiveProfile: "work"}
	if err := genvfile.WriteLock(lockPath, lf); err != nil {
		t.Fatal(err)
	}

	out = captureStdout(t, func() {
		code := run([]string{"profile", "list", "-file", specPath, "-lock-file", lockPath})
		if code != exitOK {
			t.Errorf("expected exitOK, got %d", code)
		}
	})
	if !strings.Contains(out, "  base") {
		t.Errorf("expected '  base', got %q", out)
	}
	if !strings.Contains(out, "* work") {
		t.Errorf("expected '* work', got %q", out)
	}
}

func TestProfileCreate(t *testing.T) {
	dir := t.TempDir()
	specPath := filepath.Join(dir, "genv.json")

	writeLegacyProfileBase(t, specPath)

	out := captureStdout(t, func() {
		code := run([]string{"profile", "create", "dev", "-file", specPath})
		if code != exitOK {
			t.Errorf("expected exitOK, got %d", code)
		}
	})
	if !strings.Contains(out, "Created profile \"dev\"") {
		t.Errorf("expected success message, got %q", out)
	}

	// Create existing should fail
	errOut := captureStderr(t, func() {
		code := run([]string{"profile", "create", "dev", "-file", specPath})
		if code != exitIO {
			t.Errorf("expected exitIO, got %d", code)
		}
	})
	if !strings.Contains(errOut, "already exists") {
		t.Errorf("expected already exists error, got %q", errOut)
	}
}

func TestProfileSwitch(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)

	specPath := filepath.Join(dir, "genv.json")
	lockPath := filepath.Join(dir, "genv.lock.json")

	fakeBin := filepath.Join(dir, "bin")
	if err := os.MkdirAll(fakeBin, 0755); err != nil {
		t.Fatal(err)
	}
	fakeBrew := filepath.Join(fakeBin, "brew")
	if err := os.WriteFile(fakeBrew, []byte("#!/bin/sh\nexit 0\n"), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", fakeBin+":"+os.Getenv("PATH"))

	writeLegacyProfileBase(t, specPath, schema.Package{ID: "base-pkg", Prefer: "brew"})

	if err := profile.Create(specPath, "A"); err != nil {
		t.Fatal(err)
	}
	profA, _ := profile.Load(specPath, "A")
	profA.Packages = append(profA.Packages, schema.Package{ID: "pkg-A", Prefer: "brew"})
	if err := genvfile.Write(profile.Path(specPath, "A"), profA); err != nil {
		t.Fatal(err)
	}

	if err := profile.Create(specPath, "B"); err != nil {
		t.Fatal(err)
	}
	profB, _ := profile.Load(specPath, "B")
	profB.Packages = append(profB.Packages, schema.Package{ID: "pkg-B", Prefer: "brew"})
	if err := genvfile.Write(profile.Path(specPath, "B"), profB); err != nil {
		t.Fatal(err)
	}

	// Switch to A
	_ = captureStdout(t, func() {
		code := run([]string{"profile", "switch", "A", "-file", specPath, "-lock-file", lockPath, "-yes"})
		if code != exitOK {
			t.Errorf("expected exitOK, got %d", code)
		}
	})

	lf, _ := genvfile.ReadLock(lockPath)
	if lf.ActiveProfile != "A" {
		t.Errorf("expected active profile A, got %q", lf.ActiveProfile)
	}
	if len(lf.Packages) != 2 {
		t.Errorf("expected 2 packages in lock, got %d", len(lf.Packages))
	}

	// Switch to B
	_ = captureStdout(t, func() {
		code := run([]string{"profile", "switch", "B", "-file", specPath, "-lock-file", lockPath, "-yes"})
		if code != exitOK {
			t.Errorf("expected exitOK, got %d", code)
		}
	})

	lf, _ = genvfile.ReadLock(lockPath)
	if lf.ActiveProfile != "B" {
		t.Errorf("expected active profile B, got %q", lf.ActiveProfile)
	}

	// Check that pkg-A was removed and pkg-B was installed, base-pkg remains
	pkgMap := make(map[string]bool)
	for _, p := range lf.Packages {
		pkgMap[p.ID] = true
	}
	if !pkgMap["base-pkg"] {
		t.Error("base-pkg was removed")
	}
	if !pkgMap["pkg-B"] {
		t.Error("pkg-B was not installed")
	}
	if pkgMap["pkg-A"] {
		t.Error("pkg-A was not removed")
	}
}

func TestStatusJSONActiveProfile(t *testing.T) {
	dir := t.TempDir()
	specPath := filepath.Join(dir, "genv.json")
	lockPath := filepath.Join(dir, "genv.lock.json")

	writeLegacyProfileBase(t, specPath)
	if err := profile.Create(specPath, "work"); err != nil {
		t.Fatal(err)
	}

	lf := &genvfile.LockFile{SchemaVersion: schema.Version6, ActiveProfile: "work"}
	if err := genvfile.WriteLock(lockPath, lf); err != nil {
		t.Fatal(err)
	}

	out := captureStdout(t, func() {
		code := run([]string{"status", "-json", "-file", specPath, "-lock-file", lockPath})
		if code != exitOK {
			t.Errorf("expected exitOK, got %d", code)
		}
	})

	var env output.Envelope
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatal(err)
	}

	dataBytes, _ := json.Marshal(env.Data)
	var res output.StatusResult
	if err := json.Unmarshal(dataBytes, &res); err != nil {
		t.Fatal(err)
	}

	if res.ActiveProfile != "work" {
		t.Errorf("expected active profile 'work', got %q", res.ActiveProfile)
	}
}

func TestProfileSwitchFailure(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)

	specPath := filepath.Join(dir, "genv.json")
	lockPath := filepath.Join(dir, "genv.lock.json")

	// Install a fake brew that succeeds for every package except pkg-B, and put
	// it first on PATH before any switch. Packages explicitly prefer brew so
	// resolution is deterministic on every platform (the automatic brew/linuxbrew
	// suggestion is platform-specific, but explicit selection is not).
	fakeBin := filepath.Join(dir, "bin")
	if err := os.MkdirAll(fakeBin, 0755); err != nil {
		t.Fatal(err)
	}
	fakeBrew := filepath.Join(fakeBin, "brew")
	if err := os.WriteFile(fakeBrew, []byte("#!/bin/sh\nif [ \"$2\" = \"pkg-B\" ]; then echo 'mock install failure'; exit 1; fi\nexit 0\n"), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", fakeBin+":"+os.Getenv("PATH"))

	writeLegacyProfileBase(t, specPath, schema.Package{ID: "base-pkg", Prefer: "brew"})

	if err := profile.Create(specPath, "A"); err != nil {
		t.Fatal(err)
	}
	profA, _ := profile.LoadMerged(specPath, "A")
	profA.Packages = append(profA.Packages, schema.Package{ID: "pkg-A", Prefer: "brew"})
	if err := genvfile.Write(profile.Path(specPath, "A"), profA); err != nil {
		t.Fatal(err)
	}

	if err := profile.Create(specPath, "B"); err != nil {
		t.Fatal(err)
	}
	profB, _ := profile.LoadMerged(specPath, "B")
	profB.Packages = append(profB.Packages, schema.Package{ID: "pkg-B", Prefer: "brew"})
	if err := genvfile.Write(profile.Path(specPath, "B"), profB); err != nil {
		t.Fatal(err)
	}

	// Switch to A successfully
	run([]string{"profile", "switch", "A", "-file", specPath, "-lock-file", lockPath, "-yes"})

	// Switch to B should fail because installing pkg-B via the fake brew exits 1
	out := captureStderr(t, func() {
		code := run([]string{"profile", "switch", "B", "-file", specPath, "-lock-file", lockPath, "-yes"})
		if code == exitOK {
			t.Errorf("expected nonzero exit, got %d", code)
		}
	})

	if !strings.Contains(out, "exit status 1") {
		t.Errorf("expected exit status 1, got %q", out)
	}

	// Lock file should still have ActiveProfile = A
	lf, _ := genvfile.ReadLock(lockPath)
	if lf.ActiveProfile != "A" {
		t.Errorf("expected active profile A, got %q", lf.ActiveProfile)
	}
}

func TestProfileSwitchMissing(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)

	specPath := filepath.Join(dir, "genv.json")
	lockPath := filepath.Join(dir, "genv.lock.json")

	writeLegacyProfileBase(t, specPath)

	out := captureStderr(t, func() {
		code := run([]string{"profile", "switch", "missing", "-file", specPath, "-lock-file", lockPath, "-yes"})
		if code == exitOK {
			t.Errorf("expected nonzero exit, got %d", code)
		}
	})

	if !strings.Contains(out, "profile not found: missing") {
		t.Errorf("expected profile not found error, got %q", out)
	}

	// Lock file should not be created
	if _, err := os.Stat(lockPath); !os.IsNotExist(err) {
		t.Errorf("expected lock file to not exist, got %v", err)
	}
}

func TestProfileSwitchInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)

	specPath := filepath.Join(dir, "genv.json")
	lockPath := filepath.Join(dir, "genv.lock.json")

	writeLegacyProfileBase(t, specPath)

	if err := profile.Create(specPath, "bad"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(profile.Path(specPath, "bad"), []byte("{bad json"), 0644); err != nil {
		t.Fatal(err)
	}

	out := captureStderr(t, func() {
		code := run([]string{"profile", "switch", "bad", "-file", specPath, "-lock-file", lockPath, "-yes"})
		if code == exitOK {
			t.Errorf("expected nonzero exit, got %d", code)
		}
	})

	if !strings.Contains(out, "invalid character") {
		t.Errorf("expected json parse error, got %q", out)
	}
}

func TestProfileSwitch_V8Refused(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	specPath := filepath.Join(dir, "genv.json")
	lockPath := filepath.Join(dir, "genv.lock.json")
	if err := genvfile.Write(specPath, genvfile.New()); err != nil {
		t.Fatal(err)
	}
	lf := &genvfile.LockFile{SchemaVersion: schema.Version8, ActiveProfile: "work"}
	if err := genvfile.WriteLock(lockPath, lf); err != nil {
		t.Fatal(err)
	}
	errOut := captureStderr(t, func() {
		code := run([]string{"profile", "switch", "work", "-file", specPath, "-lock-file", lockPath, "-yes"})
		if code == exitOK {
			t.Fatalf("expected v8 profile switch to fail")
		}
	})
	if !strings.Contains(errOut, "not supported") && !strings.Contains(errOut, "schemaVersion 8") {
		t.Fatalf("expected v8 refuse, got %q", errOut)
	}
}

func TestProfileCreate_V8Refused(t *testing.T) {
	dir := t.TempDir()
	specPath := filepath.Join(dir, "genv.json")
	if err := genvfile.Write(specPath, genvfile.New()); err != nil {
		t.Fatal(err)
	}
	errOut := captureStderr(t, func() {
		code := run([]string{"profile", "create", "work", "-file", specPath})
		if code == exitOK {
			t.Fatalf("expected v8 profile create to fail")
		}
	})
	if !strings.Contains(errOut, "schemaVersion 8") && !strings.Contains(errOut, "not supported") {
		t.Fatalf("expected v8 refuse message, got %q", errOut)
	}
}
