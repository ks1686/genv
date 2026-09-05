package main

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/ks1686/genv/internal/genvfile"
	"github.com/ks1686/genv/internal/schema"
	"github.com/ks1686/genv/internal/testutil"
)

func TestApplyCmd_File_LeavesDefaultConfigDirUnchanged(t *testing.T) {
	live := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", live)
	testutil.SetHome(t, live)

	liveGenv := filepath.Join(live, "genv")
	if err := os.MkdirAll(liveGenv, 0o700); err != nil {
		t.Fatal(err)
	}
	liveSpec := filepath.Join(liveGenv, "genv.json")
	writeTestFile(t, liveSpec, `{
		"schemaVersion": "8",
		"defaults": {
			"env": {
				"EDITOR": {"value": "nvim"},
				"VISUAL": {"value": "nvim"},
				"HOMEBREW_NO_ENV_HINTS": {"value": "1"}
			}
		},
		"targets": {"linux": {}}
	}`)
	writeTestFile(t, filepath.Join(liveGenv, "env.sh"), "# live env\nexport EDITOR=nvim\n")
	writeTestFile(t, filepath.Join(liveGenv, "shell.sh"), "# live shell\nalias ll='ls -la'\n")
	liveLock := &genvfile.LockFile{
		SchemaVersion: schema.Version8,
		Target:        "linux",
		GOOS:          runtime.GOOS,
		Env: []genvfile.LockedEnvVar{
			{Name: "EDITOR", Value: "nvim"},
			{Name: "VISUAL", Value: "nvim"},
			{Name: "HOMEBREW_NO_ENV_HINTS", Value: "1"},
		},
	}
	if err := genvfile.WriteLock(filepath.Join(liveGenv, "genv.lock.json"), liveLock); err != nil {
		t.Fatal(err)
	}

	before := snapshotDir(t, liveGenv)

	sandbox := t.TempDir()
	sandboxSpec := filepath.Join(sandbox, "genv.json")
	writeTestFile(t, sandboxSpec, `{
		"schemaVersion": "8",
		"defaults": {
			"env": {
				"SANDBOX_ONLY": {"value": "1"}
			}
		},
		"targets": {"linux": {}}
	}`)

	code := run([]string{"apply", "--file", sandboxSpec, "--skip-packages", "--no-hooks", "--yes", "--target", "linux"})
	if code != exitOK {
		t.Fatalf("sandbox apply: got exit %d, want %d", code, exitOK)
	}

	after := snapshotDir(t, liveGenv)
	if diff := dirDiff(before, after); diff != "" {
		t.Fatalf("default config dir changed after --file apply:\n%s", diff)
	}

	if _, err := os.Stat(filepath.Join(sandbox, "genv.lock.json")); err != nil {
		t.Fatalf("expected lock next to sandbox spec: %v", err)
	}
	envName := "env.sh"
	if runtime.GOOS == "windows" {
		envName = "env.ps1"
	}
	if _, err := os.Stat(filepath.Join(sandbox, envName)); err != nil {
		t.Fatalf("expected env fragment next to sandbox spec: %v", err)
	}
}

func TestApplyCmd_DryRun_NamesStatePaths(t *testing.T) {
	live := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", live)
	testutil.SetHome(t, live)

	sandbox := t.TempDir()
	spec := filepath.Join(sandbox, "genv.json")
	writeTestFile(t, spec, `{
		"schemaVersion": "8",
		"defaults": {
			"env": {"SANDBOX_ONLY": {"value": "1"}},
			"shell": {"aliases": {"ll": {"value": "ls -la"}}}
		},
		"targets": {"linux": {}}
	}`)

	var code int
	out := captureStdout(t, func() {
		code = run([]string{"apply", "--file", spec, "--dry-run", "--skip-packages", "--target", "linux"})
	})
	if code != exitOK {
		t.Fatalf("dry-run apply: got exit %d, want %d\n%s", code, exitOK, out)
	}

	lockWant := filepath.Join(sandbox, "genv.lock.json")
	if !strings.Contains(out, lockWant) {
		t.Fatalf("plan missing lock path %q\n%s", lockWant, out)
	}
	if !strings.Contains(out, filepath.Join(sandbox, "env.sh")) && !strings.Contains(out, filepath.Join(sandbox, "env.ps1")) {
		t.Fatalf("plan missing env fragment path under %s\n%s", sandbox, out)
	}
	if !strings.Contains(out, filepath.Join(sandbox, "shell.sh")) && !strings.Contains(out, filepath.Join(sandbox, "shell.ps1")) {
		t.Fatalf("plan missing shell fragment path under %s\n%s", sandbox, out)
	}

	jsonOut := captureStdout(t, func() {
		code = run([]string{"apply", "--file", spec, "--dry-run", "--json", "--skip-packages", "--target", "linux"})
	})
	if code != exitOK {
		t.Fatalf("json dry-run: got exit %d\n%s", code, jsonOut)
	}
	var env map[string]any
	if err := json.Unmarshal([]byte(jsonOut), &env); err != nil {
		t.Fatalf("json: %v\n%s", err, jsonOut)
	}
	data, _ := env["data"].(map[string]any)
	state, _ := data["state"].(map[string]any)
	if state == nil {
		t.Fatalf("json plan missing data.state: %s", jsonOut)
	}
	if lock, _ := state["lock"].(string); lock != lockWant {
		t.Fatalf("json state.lock = %q, want %q", lock, lockWant)
	}
}

func TestAdoptCmd_File_ReadsLockNextToSpec(t *testing.T) {
	live := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", live)
	testutil.SetHome(t, live)
	withInstalledBrew(t)

	liveGenv := filepath.Join(live, "genv")
	if err := os.MkdirAll(liveGenv, 0o700); err != nil {
		t.Fatal(err)
	}
	writeLock(t, filepath.Join(liveGenv, "genv.lock.json"), []genvfile.LockedPackage{
		{ID: "git", Manager: "brew", PkgName: "git"},
	})

	sandbox := t.TempDir()
	spec := filepath.Join(sandbox, "genv.json")
	writeTestFile(t, spec, `{
		"schemaVersion": "8",
		"defaults": {},
		"targets": {"linux": {}}
	}`)

	code := run([]string{"adopt", "--file", spec, "--prefer", "brew", "--target", "linux", "git"})
	if code != exitOK {
		t.Fatalf("adopt --file: got exit %d, want %d (must not consult the default lock)", code, exitOK)
	}

	sandboxLock := filepath.Join(sandbox, "genv.lock.json")
	lf, err := genvfile.ReadLock(sandboxLock)
	if err != nil {
		t.Fatalf("read sandbox lock: %v", err)
	}
	if len(lf.Packages) != 1 || lf.Packages[0].ID != "git" {
		t.Fatalf("sandbox lock packages = %+v, want git", lf.Packages)
	}

	liveLF, err := genvfile.ReadLock(filepath.Join(liveGenv, "genv.lock.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(liveLF.Packages) != 1 || liveLF.Packages[0].ID != "git" {
		t.Fatalf("live lock mutated: %+v", liveLF.Packages)
	}
}

func TestApplyCmd_StateDir_OverridesLockAndFragments(t *testing.T) {
	live := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", live)
	testutil.SetHome(t, live)
	liveGenv := filepath.Join(live, "genv")
	if err := os.MkdirAll(liveGenv, 0o700); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(liveGenv, "env.sh"), "# live\n")
	before := snapshotDir(t, liveGenv)

	specDir := t.TempDir()
	stateDir := t.TempDir()
	spec := filepath.Join(specDir, "genv.json")
	writeTestFile(t, spec, `{
		"schemaVersion": "8",
		"defaults": {"env": {"FROM_STATE_DIR": {"value": "yes"}}},
		"targets": {"linux": {}}
	}`)

	code := run([]string{
		"apply", "--file", spec, "--state-dir", stateDir,
		"--skip-packages", "--no-hooks", "--yes", "--target", "linux",
	})
	if code != exitOK {
		t.Fatalf("apply --state-dir: got exit %d", code)
	}

	if _, err := os.Stat(filepath.Join(stateDir, "genv.lock.json")); err != nil {
		t.Fatalf("lock not in --state-dir: %v", err)
	}
	envName := "env.sh"
	if runtime.GOOS == "windows" {
		envName = "env.ps1"
	}
	if _, err := os.Stat(filepath.Join(stateDir, envName)); err != nil {
		t.Fatalf("env fragment not in --state-dir: %v", err)
	}
	if _, err := os.Stat(filepath.Join(specDir, "genv.lock.json")); err == nil {
		t.Fatal("lock written next to spec despite --state-dir")
	}
	if diff := dirDiff(before, snapshotDir(t, liveGenv)); diff != "" {
		t.Fatalf("live config dir changed:\n%s", diff)
	}
}

func TestApplyCmd_File_WetApplyDoesNotWriteDefaultLock(t *testing.T) {
	live := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", live)
	testutil.SetHome(t, live)

	sandbox := t.TempDir()
	spec := filepath.Join(sandbox, "genv.json")
	writeTestFile(t, spec, `{
		"schemaVersion": "8",
		"defaults": {"env": {"SANDBOX_ONLY": {"value": "1"}}},
		"targets": {"linux": {}}
	}`)

	code := run([]string{"apply", "--file", spec, "--skip-packages", "--no-hooks", "--yes", "--target", "linux"})
	if code != exitOK {
		t.Fatalf("apply: got exit %d", code)
	}

	defaultLock := filepath.Join(live, "genv", "genv.lock.json")
	if _, err := os.Stat(defaultLock); err == nil {
		t.Fatalf("wet apply wrote default lock %s", defaultLock)
	}
}

func snapshotDir(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if d.IsDir() {
			out[rel+"/"] = ""
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		out[rel] = string(data)
		return nil
	})
	if err != nil {
		t.Fatalf("snapshot %s: %v", root, err)
	}
	return out
}

func dirDiff(before, after map[string]string) string {
	var b strings.Builder
	for k, v := range before {
		got, ok := after[k]
		if !ok {
			b.WriteString("removed " + k + "\n")
			continue
		}
		if got != v {
			b.WriteString("changed " + k + "\n")
		}
	}
	for k := range after {
		if _, ok := before[k]; !ok {
			b.WriteString("added " + k + "\n")
		}
	}
	return b.String()
}
