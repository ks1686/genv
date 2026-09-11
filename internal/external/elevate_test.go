package external

import (
	"errors"
	"os"
	"testing"
)

func TestElevationHintIsExplicitForSystemScope(t *testing.T) {
	if ElevationHint("user") != "" || ElevationHint("") != "" {
		t.Fatal("user scope must not request elevation")
	}
	if ElevationHint("system") == "" {
		t.Fatal("system scope must describe elevation")
	}
}

func TestWrapSystemScopeErrorDoesNotRetryPermissionFailure(t *testing.T) {
	err := wrapSystemScopeError("system", "/usr/local/bin/tool", os.ErrPermission)
	if err == nil || !errors.Is(err, os.ErrPermission) {
		t.Fatalf("error = %v", err)
	}
	if wrapSystemScopeError("user", "/usr/local/bin/tool", os.ErrPermission) != os.ErrPermission {
		t.Fatal("user-scope permission errors must not be rewritten as elevation")
	}
}
