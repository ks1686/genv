package target

import (
	"errors"
	"strings"
	"testing"
)

func TestResolve_PrefersFlagThenEnv(t *testing.T) {
	t.Setenv("GENV_TARGET", "ubuntu")

	got, err := Resolve("arch")
	if err != nil || got != "arch" {
		t.Fatalf("Resolve(flag) = %q, %v; want arch, nil", got, err)
	}

	got, err = Resolve("")
	if err != nil || got != "ubuntu" {
		t.Fatalf("Resolve(env) = %q, %v; want ubuntu, nil", got, err)
	}
}

func TestResolve_FallsBackToClassify(t *testing.T) {
	t.Setenv("GENV_TARGET", "")

	got, err := Resolve("")
	if err != nil {
		t.Skipf("Resolve() cannot classify this runner: %v", err)
	}

	switch got {
	case "macos", "windows", "arch", "ubuntu", "wsl-arch":
	default:
		t.Fatalf("Resolve() = %q, want known target class", got)
	}
}

func TestResolve_ErrorIncludesGuidance(t *testing.T) {
	got, err := resolve("", "", func() (string, error) {
		return "", errors.New("unsupported host")
	})
	if err == nil {
		t.Fatalf("resolve() = %q, nil error; want guidance error", got)
	}
	msg := err.Error()
	if !strings.Contains(msg, "--target") || !strings.Contains(msg, "GENV_TARGET") {
		t.Fatalf("resolve() error = %q, want --target and GENV_TARGET guidance", msg)
	}
}
