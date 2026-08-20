package adapter

import "testing"

func TestExternal_Name(t *testing.T) {
	if got := (External{}).Name(); got != "external" {
		t.Errorf("Name() = %q, want external", got)
	}
}

func TestExternal_QueryUsesLookPath(t *testing.T) {
	installFakeBinary(t, "hermes", "exit 0")
	ok, err := (External{}).Query("hermes")
	if err != nil || !ok {
		t.Fatalf("Query(hermes)=%v %v, want true", ok, err)
	}
	ok, err = (External{}).Query("definitely-not-installed-xyz")
	if err != nil || ok {
		t.Fatalf("Query(missing)=%v %v, want false", ok, err)
	}
}

func TestExternal_NotDefaultFallback(t *testing.T) {
	if IsDefaultFallbackEligible(External{}) {
		t.Fatal("external must not steal genv add git")
	}
}

func TestExternal_AvailableAlways(t *testing.T) {
	if !(External{}).Available() {
		t.Fatal("external should always be available")
	}
}

func TestExternal_PlanInstallEmpty(t *testing.T) {
	if cmd := (External{}).PlanInstall("hermes"); len(cmd) != 0 {
		t.Fatalf("PlanInstall = %v, want empty", cmd)
	}
}
