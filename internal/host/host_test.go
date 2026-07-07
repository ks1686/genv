package host

import (
	"runtime"
	"testing"

	"github.com/ks1686/genv/internal/schema"
)

func TestCurrent_UsesGenvHost(t *testing.T) {
	t.Setenv("GENV_HOST", "testbox")

	host, err := Current()
	if err != nil {
		t.Fatalf("Current() error = %v, want nil", err)
	}
	if host != "testbox" {
		t.Fatalf("Current() = %q, want %q", host, "testbox")
	}
}

func TestCurrent_FallsBackToHostname(t *testing.T) {
	t.Setenv("GENV_HOST", "")

	host, err := Current()
	if err != nil {
		t.Fatalf("Current() error = %v, want nil", err)
	}
	if host == "" {
		t.Fatal("Current() returned empty host when GENV_HOST is unset")
	}
}

func TestMatch_EmptyPredicateMatchesEveryHost(t *testing.T) {
	cases := []schema.HostPredicate{nil, {}}
	for _, pred := range cases {
		if !Match(pred, "any") {
			t.Fatalf("Match(%v, %q) = false, want true", pred, "any")
		}
	}
}

func TestMatch_MatchesWhenHostInPredicate(t *testing.T) {
	pred := schema.HostPredicate{"macos", "arch"}

	if !Match(pred, "arch") {
		t.Fatalf("Match(%v, %q) = false, want true", pred, "arch")
	}
}

func TestMatch_DoesNotMatchWhenHostNotInPredicate(t *testing.T) {
	pred := schema.HostPredicate{"macos"}

	if Match(pred, "arch") {
		t.Fatalf("Match(%v, %q) = true, want false", pred, "arch")
	}
}

func TestMatch_EmptyHostWithNonEmptyPredicateIsFalse(t *testing.T) {
	pred := schema.HostPredicate{"macos"}

	if Match(pred, "") {
		t.Fatalf("Match(%v, %q) = true, want false", pred, "")
	}
}

func TestMatch_WslInheritsArch(t *testing.T) {
	pred := schema.HostPredicate{"arch"}

	if !Match(pred, "wsl2") {
		t.Fatalf("Match(%v, %q) = false, want true", pred, "wsl2")
	}
}

func TestMatch_ArchDoesNotInheritWsl(t *testing.T) {
	pred := schema.HostPredicate{"wsl2"}

	if Match(pred, "arch") {
		t.Fatalf("Match(%v, %q) = true, want false", pred, "arch")
	}
}

func TestClassify_UsesGenvHost(t *testing.T) {
	t.Setenv("GENV_HOST", "testbox")

	h, err := Classify()
	if err != nil {
		t.Fatalf("Classify() error = %v, want nil", err)
	}
	if h != "testbox" {
		t.Fatalf("Classify() = %q, want %q", h, "testbox")
	}
}

func TestClassify_ReturnsKnownClassForCurrentPlatform(t *testing.T) {
	t.Setenv("GENV_HOST", "")

	h, err := Classify()
	if err != nil {
		if runtime.GOOS == "linux" {
			t.Skipf("Classify() error = %v; host is neither Arch nor WSL2 (e.g. a generic CI runner) — skipping", err)
		}
		t.Fatalf("Classify() error = %v, want nil", err)
	}
	switch runtime.GOOS {
	case "darwin":
		if h != "macos" {
			t.Fatalf("Classify() = %q, want %q", h, "macos")
		}
	case "linux":
		if h != "arch" && h != "wsl2" {
			t.Fatalf("Classify() = %q, want arch or wsl2", h)
		}
	default:
		t.Fatalf("unexpected classified host %q on GOOS %q", h, runtime.GOOS)
	}
}
