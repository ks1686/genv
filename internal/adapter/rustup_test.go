package adapter

import (
	"slices"
	"testing"
)

func TestParseRustupPackageID_whenNamespacedIDsAreValid(t *testing.T) {
	// Given
	tests := []struct {
		name string
		id   string
		want rustupPackage
	}{
		{name: "toolchain", id: "toolchain:stable-aarch64-apple-darwin", want: rustupPackage{kind: rustupToolchain, name: "stable-aarch64-apple-darwin"}},
		{name: "component", id: "component:rustfmt@stable", want: rustupPackage{kind: rustupComponent, name: "rustfmt", toolchain: "stable"}},
		{name: "target", id: "target:aarch64-unknown-linux-gnu@nightly", want: rustupPackage{kind: rustupTarget, name: "aarch64-unknown-linux-gnu", toolchain: "nightly"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// When
			got, ok := parseRustupPackageID(tt.id)

			// Then
			if !ok {
				t.Fatalf("parseRustupPackageID(%q) ok = false, want true", tt.id)
			}
			if got != tt.want {
				t.Errorf("parseRustupPackageID(%q) = %#v, want %#v", tt.id, got, tt.want)
			}
		})
	}
}

func TestParseRustupPackageID_whenMalformedIDsAreInvalid(t *testing.T) {
	// Given
	ids := []string{"", "stable", "toolchain:", "component:rustfmt", "component:@stable", "component:rustfmt@", "target:aarch64", "target:@stable", "unknown:thing"}

	for _, id := range ids {
		t.Run(id, func(t *testing.T) {
			// When
			_, ok := parseRustupPackageID(id)

			// Then
			if ok {
				t.Errorf("parseRustupPackageID(%q) ok = true, want false", id)
			}
		})
	}
}

func TestRustup_ParseToolchains_whenOutputHasDefaultMarker(t *testing.T) {
	// Given
	lines := []string{"stable-aarch64-apple-darwin (default)", "nightly-x86_64-unknown-linux-gnu", "1.76.0-x86_64-unknown-linux-gnu (override)", ""}

	// When
	got := parseRustupToolchains(lines)

	// Then
	want := []string{"stable-aarch64-apple-darwin", "nightly-x86_64-unknown-linux-gnu", "1.76.0-x86_64-unknown-linux-gnu"}
	if !slices.Equal(got, want) {
		t.Errorf("parseRustupToolchains = %v, want %v", got, want)
	}
}

func TestRustup_PlanCommands_whenIDsAreTrackedAndNamespaced(t *testing.T) {
	// Given
	a := Rustup{}

	// When / Then
	if got, want := a.PlanInstall("toolchain:stable"), []string{"rustup", "toolchain", "install", "stable"}; !slices.Equal(got, want) {
		t.Errorf("toolchain PlanInstall = %v, want %v", got, want)
	}
	if got, want := a.PlanUninstall("toolchain:stable"), []string{"rustup", "toolchain", "uninstall", "stable"}; !slices.Equal(got, want) {
		t.Errorf("toolchain PlanUninstall = %v, want %v", got, want)
	}
	if got, want := a.PlanUpgrade("toolchain:stable"), []string{"rustup", "update", "stable"}; !slices.Equal(got, want) {
		t.Errorf("toolchain PlanUpgrade = %v, want %v", got, want)
	}
	if got, want := a.PlanInstall("component:rustfmt@stable"), []string{"rustup", "component", "add", "rustfmt", "--toolchain", "stable"}; !slices.Equal(got, want) {
		t.Errorf("component PlanInstall = %v, want %v", got, want)
	}
	if got, want := a.PlanUninstall("component:rustfmt@stable"), []string{"rustup", "component", "remove", "rustfmt", "--toolchain", "stable"}; !slices.Equal(got, want) {
		t.Errorf("component PlanUninstall = %v, want %v", got, want)
	}
	if got, want := a.PlanUpgrade("component:rustfmt@stable"), []string{"rustup", "update", "stable"}; !slices.Equal(got, want) {
		t.Errorf("component PlanUpgrade = %v, want %v", got, want)
	}
	if got, want := a.PlanInstall("target:aarch64-unknown-linux-gnu@stable"), []string{"rustup", "target", "add", "aarch64-unknown-linux-gnu", "--toolchain", "stable"}; !slices.Equal(got, want) {
		t.Errorf("target PlanInstall = %v, want %v", got, want)
	}
	if got, want := a.PlanUninstall("target:aarch64-unknown-linux-gnu@stable"), []string{"rustup", "target", "remove", "aarch64-unknown-linux-gnu", "--toolchain", "stable"}; !slices.Equal(got, want) {
		t.Errorf("target PlanUninstall = %v, want %v", got, want)
	}
}

func TestRustup_PlanCommands_whenIDMalformedAreInert(t *testing.T) {
	// Given
	a := Rustup{}

	// When / Then
	for _, args := range [][]string{a.PlanInstall("stable"), a.PlanUninstall("component:rustfmt"), a.PlanUpgrade("target:aarch64")} {
		if !slices.Equal(args, []string{"rustup", "help"}) {
			t.Errorf("malformed plan = %v, want [rustup help]", args)
		}
	}
}

func TestRustup_Query_whenToolchainComponentAndTargetAreInstalled(t *testing.T) {
	// Given
	installFakeBinary(t, "rustup",
		`if [ "$1" = "toolchain" ] && [ "$2" = "list" ]; then
  echo "stable-aarch64-apple-darwin (default)"
  echo "nightly-x86_64-unknown-linux-gnu"
elif [ "$1" = "component" ] && [ "$2" = "list" ] && [ "$3" = "--toolchain" ] && [ "$4" = "stable-aarch64-apple-darwin" ]; then
  echo "cargo-aarch64-apple-darwin (installed)"
  echo "rustfmt-aarch64-apple-darwin (installed)"
elif [ "$1" = "target" ] && [ "$2" = "list" ] && [ "$3" = "--toolchain" ] && [ "$4" = "stable-aarch64-apple-darwin" ]; then
  echo "aarch64-apple-darwin (installed)"
  echo "aarch64-unknown-linux-gnu"
fi`)

	// When / Then
	for _, id := range []string{"toolchain:stable-aarch64-apple-darwin", "component:rustfmt@stable-aarch64-apple-darwin", "target:aarch64-apple-darwin@stable-aarch64-apple-darwin"} {
		ok, err := Rustup{}.Query(id)
		if err != nil {
			t.Fatalf("Rustup.Query(%q): %v", id, err)
		}
		if !ok {
			t.Errorf("Rustup.Query(%q) = false, want true", id)
		}
	}

	ok, err := Rustup{}.Query("target:aarch64-unknown-linux-gnu@stable-aarch64-apple-darwin")
	if err != nil {
		t.Fatalf("Rustup.Query(absent target): %v", err)
	}
	if ok {
		t.Error("Rustup.Query(absent target) = true, want false")
	}
}

func TestRustup_ListInstalledAndQueryVersion_whenToolchainsExist(t *testing.T) {
	// Given
	installFakeBinary(t, "rustup",
		`if [ "$1" = "toolchain" ] && [ "$2" = "list" ]; then
  echo "stable-aarch64-apple-darwin (default)"
  echo "nightly-x86_64-unknown-linux-gnu"
fi`)

	// When
	pkgs, err := Rustup{}.ListInstalled()

	// Then
	if err != nil {
		t.Fatalf("Rustup.ListInstalled: %v", err)
	}
	want := []string{"toolchain:stable-aarch64-apple-darwin", "toolchain:nightly-x86_64-unknown-linux-gnu"}
	if !slices.Equal(pkgs, want) {
		t.Errorf("ListInstalled = %v, want %v", pkgs, want)
	}
	version, err := Rustup{}.QueryVersion("toolchain:stable-aarch64-apple-darwin")
	if err != nil {
		t.Fatalf("Rustup.QueryVersion: %v", err)
	}
	if version != "stable-aarch64-apple-darwin" {
		t.Errorf("QueryVersion = %q, want %q", version, "stable-aarch64-apple-darwin")
	}
}

func TestRustup_Query_whenIDMalformed(t *testing.T) {
	// When
	ok, err := Rustup{}.Query("component:rustfmt")

	// Then
	if err != nil {
		t.Fatalf("Rustup.Query malformed: %v", err)
	}
	if ok {
		t.Error("Rustup.Query malformed = true, want false")
	}
}
