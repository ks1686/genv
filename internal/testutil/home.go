package testutil

import "testing"

// SetHome isolates os.UserHomeDir on Unix (HOME) and Windows (USERPROFILE).
// Tests that only set HOME leak into the real Windows profile.
func SetHome(t *testing.T, home string) {
	t.Helper()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("HOMEDRIVE", "")
	t.Setenv("HOMEPATH", "")
}
