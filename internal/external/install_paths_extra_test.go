package external

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestInstallDirectReplacesExistingAndRestores(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	dst := filepath.Join(dir, "bin", "tool")
	if err := os.WriteFile(src, []byte("new"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	restore, _, err := installDirect(src, dst, installPolicy{})
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(dst); string(got) != "new" {
		t.Fatalf("installed = %q", got)
	}
	restore()
	restore() // idempotent
	if got, _ := os.ReadFile(dst); string(got) != "old" {
		t.Fatalf("restore left %q, want old", got)
	}
	_, finish, err := installDirect(src, dst, installPolicy{})
	if err != nil {
		t.Fatal(err)
	}
	if err := finish(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dst + ".genv-backup"); !os.IsNotExist(err) {
		t.Fatalf("backup should be gone after finish: %v", err)
	}
}

func TestInstallDirectMissingSource(t *testing.T) {
	dir := t.TempDir()
	if _, _, err := installDirect(filepath.Join(dir, "nope"), filepath.Join(dir, "out"), installPolicy{}); err == nil {
		t.Fatal("missing source accepted")
	}
}

func TestSystemScopeUnattendedNeverElevates(t *testing.T) {
	old := destinationRequiresElevation
	destinationRequiresElevation = func(string) bool { return true }
	t.Cleanup(func() { destinationRequiresElevation = old })
	_, _, err := installDirectWithMode("src", "/nonexistent/dst", 0o755, installPolicy{Scope: "system", Mode: ExecutionUnattended})
	if err == nil || !isErr(err, ErrUnattendedElevation) {
		t.Fatalf("err = %v, want ErrUnattendedElevation", err)
	}
}

func TestDefaultRunElevatedRunsCommandAndRejectsEmpty(t *testing.T) {
	if err := defaultRunElevated(nil, nil, nil); err == nil {
		t.Fatal("empty argv accepted")
	}
	if runtime.GOOS == "windows" {
		t.Skip("POSIX shell required")
	}
	var out bytes.Buffer
	if err := defaultRunElevated(nil, &out, []string{"sh", "-c", "echo hi"}); err != nil {
		t.Fatal(err)
	}
	if out.String() != "hi\n" {
		t.Fatalf("output = %q", out.String())
	}
	if err := defaultRunElevated(bytes.NewReader(nil), &out, []string{"sh", "-c", "exit 3"}); err == nil {
		t.Fatal("failing command reported success")
	}
}

func TestCurrentHostAndSchemeAllowed(t *testing.T) {
	h := CurrentHost()
	if h.OS != runtime.GOOS || h.Arch != runtime.GOARCH {
		t.Fatalf("host = %+v", h)
	}
}

func isErr(err, target error) bool {
	for err != nil {
		if err == target {
			return true
		}
		u, ok := err.(interface{ Unwrap() error })
		if !ok {
			if m, ok := err.(interface{ Unwrap() []error }); ok {
				for _, e := range m.Unwrap() {
					if isErr(e, target) {
						return true
					}
				}
			}
			return false
		}
		err = u.Unwrap()
	}
	return false
}
