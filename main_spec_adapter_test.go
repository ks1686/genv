package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/ks1686/genv/internal/adapter"
	"github.com/ks1686/genv/internal/genvfile"
)

func TestSpecAdapter_ApplyStatusScanAndUpdates(t *testing.T) {
	t.Cleanup(func() { adapter.SetSpecAdapters(nil) })
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "xdg"))
	bin := filepath.Join(dir, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	stateDir := filepath.Join(dir, "plug-state")
	t.Setenv("PLUGCTL_STATE", stateDir)
	script := "#!/bin/sh\n" +
		"state=${PLUGCTL_STATE:-/tmp/plug-state}\n" +
		"mkdir -p \"$state\"\n" +
		"case \"$1\" in\n" +
		"  list)\n" +
		"    printf '%s\\n' 'slack@official 1.0.0' 'extra@src 0.2.0'\n" +
		"    for f in \"$state\"/*; do\n" +
		"      [ -f \"$f\" ] || continue\n" +
		"      printf '%s\\n' \"$(basename \"$f\")\"\n" +
		"    done\n" +
		"    ;;\n" +
		"  install)\n" +
		"    : > \"$state/$2\"\n" +
		"    ;;\n" +
		"  uninstall|update) exit 0 ;;\n" +
		"  *) exit 0 ;;\n" +
		"esac\n"
	plug := filepath.Join(bin, "plugctl")
	if runtime.GOOS == "windows" {
		t.Skip("spec-adapter CLI test uses a POSIX fake binary")
	}
	if err := os.WriteFile(plug, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	specPath := filepath.Join(dir, "genv.json")
	lockPath := filepath.Join(dir, "genv.lock.json")
	spec := `{
	  "schemaVersion":"8",
	  "adapters":{
	    "plug":{
	      "list":"plugctl list",
	      "install":"plugctl install {{id}}",
	      "remove":"plugctl uninstall {{id}}",
	      "upgrade":"plugctl update {{id}}"
	    }
	  },
	  "targets":{
	    "linux":{
	      "packages":[
	        {"id":"slack@official","prefer":"plug"},
	        {"id":"needed@src","prefer":"plug"}
	      ]
	    }
	  }
	}`
	if err := os.WriteFile(specPath, []byte(spec), 0o644); err != nil {
		t.Fatal(err)
	}

	var applyOut string
	var code int
	applyOut = captureStdout(t, func() {
		code = run([]string{"apply", "--file", specPath, "--lock-file", lockPath, "--target", "linux", "--dry-run"})
	})
	if code != exitOK {
		t.Fatalf("apply --dry-run: exit %d\n%s", code, applyOut)
	}
	if !strings.Contains(applyOut, "via plug") {
		t.Fatalf("apply dry-run should resolve via plug:\n%s", applyOut)
	}
	if !strings.Contains(applyOut, "plugctl install needed@src") {
		t.Fatalf("apply dry-run missing install for uninstalled package:\n%s", applyOut)
	}

	applyOut = captureStdout(t, func() {
		code = run([]string{"apply", "--file", specPath, "--lock-file", lockPath, "--target", "linux", "--yes"})
	})
	if code != exitOK {
		t.Fatalf("apply: exit %d\n%s", code, applyOut)
	}
	lf, err := genvfile.ReadLock(lockPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(lf.Packages) < 2 {
		t.Fatalf("lock packages=%+v, want slack@official and needed@src", lf.Packages)
	}
	for _, lp := range lf.Packages {
		if lp.Manager != "plug" {
			t.Fatalf("lock entry %+v: manager want plug", lp)
		}
	}

	var statusOut string
	statusOut = captureStdout(t, func() {
		code = run([]string{"status", "--file", specPath, "--lock-file", lockPath, "--target", "linux"})
	})
	if code != exitOK {
		t.Fatalf("status: exit %d\n%s", code, statusOut)
	}

	var scanOut string
	scanOut = captureStdout(t, func() {
		code = run([]string{"scan", "--file", specPath, "--lock-file", lockPath, "--target", "linux", "--dry-run"})
	})
	if code != exitOK {
		t.Fatalf("scan: exit %d\n%s", code, scanOut)
	}
	if !strings.Contains(scanOut, "extra@src") || !strings.Contains(scanOut, "via plug") {
		t.Fatalf("scan should propose extra@src via plug:\n%s", scanOut)
	}
	if strings.Contains(scanOut, "slack@official") {
		t.Fatalf("scan should skip already-tracked slack@official:\n%s", scanOut)
	}

	scanOut = captureStdout(t, func() {
		code = run([]string{"scan", "--file", specPath, "--lock-file", lockPath, "--target", "linux", "--yes"})
	})
	if code != exitOK {
		t.Fatalf("scan --yes: exit %d\n%s", code, scanOut)
	}
	f, err := genvfile.Read(specPath)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, p := range f.Targets["linux"].Packages {
		if p.ID == "extra@src" && p.Prefer == "plug" {
			found = true
		}
	}
	if !found {
		t.Fatalf("scan should persist prefer:plug for extra@src: %+v", f.Targets["linux"].Packages)
	}

	var updatesOut string
	updatesOut = captureStdout(t, func() {
		code = run([]string{"updates", "check", "--file", specPath, "--lock-file", lockPath, "--target", "linux"})
	})
	if code != exitOK {
		t.Fatalf("updates check: exit %d\n%s", code, updatesOut)
	}
	if !strings.Contains(updatesOut, "slack@official") && !strings.Contains(updatesOut, "plugctl update") && !strings.Contains(updatesOut, "plug") {
		t.Fatalf("updates check should cover spec-adapter packages:\n%s", updatesOut)
	}
}
