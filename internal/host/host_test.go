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

func TestIsWSLAndIsArchDetectors(t *testing.T) {
	// Exercise the detector functions regardless of host; on macOS both
	// typically return false after failing to read Linux-only paths.
	_ = isWSL()
	_ = isArch()

	t.Setenv("GENV_HOST", "")
	got, err := Classify()
	if runtime.GOOS == "darwin" {
		if err != nil || got != "macos" {
			t.Fatalf("Classify on darwin = %q, %v", got, err)
		}
	}
}

func TestClassifyLinux(t *testing.T) {
	cases := []struct {
		name        string
		osRelease   string
		procVersion string
		want        string
		wantErr     bool
	}{
		{
			name:      "arch",
			osRelease: "ID=arch\n",
			want:      "arch",
		},
		{
			name:      "arch like",
			osRelease: "ID=endeavouros\nID_LIKE=\"arch\"\n",
			want:      "arch",
		},
		{
			name:      "ubuntu",
			osRelease: "ID=ubuntu\n",
			want:      "ubuntu",
		},
		{
			name:      "ubuntu like case insensitive",
			osRelease: "ID=pop\nID_LIKE=\"Debian Ubuntu\"\n",
			want:      "ubuntu",
		},
		{
			name:        "wsl arch",
			osRelease:   "ID=arch\n",
			procVersion: "Linux version 5.15.90.1-microsoft-standard-WSL2",
			want:        "wsl-arch",
		},
		{
			name:        "wsl ubuntu remains ubuntu",
			osRelease:   "ID=ubuntu\n",
			procVersion: "Linux version 5.15.90.1-microsoft-standard-WSL2",
			want:        "ubuntu",
		},
		{
			name:        "wsl unsupported",
			osRelease:   "ID=debian\n",
			procVersion: "Linux version 5.15.90.1-microsoft-standard-WSL2",
			wantErr:     true,
		},
		{
			name:      "unsupported native linux",
			osRelease: "ID=debian\n",
			wantErr:   true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := classifyLinux(tc.osRelease, tc.procVersion)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("classifyLinux() error = nil, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("classifyLinux() error = %v, want nil", err)
			}
			if got != tc.want {
				t.Fatalf("classifyLinux() = %q, want %q", got, tc.want)
			}
		})
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

func TestMatch_WslArchDoesNotInheritBareArch(t *testing.T) {
	pred := schema.HostPredicate{"arch"}

	if Match(pred, "wsl-arch") {
		t.Fatalf("Match(%v, %q) = true, want false", pred, "wsl-arch")
	}
	if !Match(schema.HostPredicate{"wsl-arch"}, "wsl-arch") {
		t.Fatalf("Match(%v, %q) = false, want true", schema.HostPredicate{"wsl-arch"}, "wsl-arch")
	}
}

func TestMatch_ArchDoesNotInheritWsl(t *testing.T) {
	pred := schema.HostPredicate{"wsl-arch"}

	if Match(pred, "arch") {
		t.Fatalf("Match(%v, %q) = true, want false", pred, "arch")
	}
}

func TestClassify_DoesNotUseGenvHost(t *testing.T) {
	t.Setenv("GENV_HOST", "testbox")

	h, err := Classify()
	if err == nil && h == "testbox" {
		t.Fatalf("Classify() = %q from GENV_HOST, want detected target class", h)
	}
}

func TestClassify_ReturnsKnownClassForCurrentPlatform(t *testing.T) {
	t.Setenv("GENV_HOST", "")

	h, err := Classify()
	if err != nil {
		if runtime.GOOS == "linux" {
			t.Skipf("Classify() error = %v; host is not a supported Linux target — skipping", err)
		}
		t.Fatalf("Classify() error = %v, want nil", err)
	}
	switch runtime.GOOS {
	case "darwin":
		if h != "macos" {
			t.Fatalf("Classify() = %q, want %q", h, "macos")
		}
	case "linux":
		if h != "arch" && h != "ubuntu" && h != "wsl-arch" {
			t.Fatalf("Classify() = %q, want arch, ubuntu, or wsl-arch", h)
		}
	case "windows":
		if h != "windows" {
			t.Fatalf("Classify() = %q, want %q", h, "windows")
		}
	default:
		t.Fatalf("unexpected classified host %q on GOOS %q", h, runtime.GOOS)
	}
}
