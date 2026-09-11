package external

import (
	"path/filepath"
	"testing"

	"github.com/ks1686/genv/internal/schema"
)

func TestValidateInstallScopeRejectsUserDestinationOutsideHome(t *testing.T) {
	home := t.TempDir()
	err := validateInstallScope(schema.ExternalInstall{Type: "direct", Destination: filepath.Join(filepath.Dir(home), "outside", "tool")}, home)
	if err == nil {
		t.Fatal("user-scope destination outside home accepted")
	}
}

func TestValidateInstallScopeAllowsDeclaredSystemDestination(t *testing.T) {
	home := t.TempDir()
	err := validateInstallScope(schema.ExternalInstall{Type: "direct", Scope: "system", Destination: filepath.Join(filepath.Dir(home), "outside", "tool")}, home)
	if err != nil {
		t.Fatalf("system-scope destination rejected: %v", err)
	}
}
