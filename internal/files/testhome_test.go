package files

import "testing"

// setTestHome isolates os.UserHomeDir on both Unix (HOME) and Windows
// (USERPROFILE). Tests that only set HOME leak into C:\Users\runneradmin.
func setTestHome(t *testing.T, home string) {
	t.Helper()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("HOMEDRIVE", "")
	t.Setenv("HOMEPATH", "")
}
