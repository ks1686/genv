package adapter

import (
	"errors"
	"os"
	"testing"
)

func TestPipUser_Available_requiresPythonAndPip(t *testing.T) {
	origLookPath := lookPath
	origProbe := pipUserProbe
	t.Cleanup(func() {
		lookPath = origLookPath
		pipUserProbe = origProbe
	})

	a := PipUser{}

	lookPath = func(string) (string, error) { return "/usr/bin/python3", nil }
	pipUserProbe = func() error { return nil }
	if !a.Available() {
		t.Error("PipUser.Available() = false when python3 and pip are usable")
	}

	lookPath = func(string) (string, error) { return "/usr/bin/python3", nil }
	pipUserProbe = func() error { return errors.New("No module named pip") }
	if a.Available() {
		t.Error("PipUser.Available() = true when python3 exists but pip is unavailable")
	}

	lookPath = func(string) (string, error) {
		return "", &os.PathError{Op: "lookpath", Path: "python3", Err: os.ErrNotExist}
	}
	pipUserProbe = func() error { return nil }
	if a.Available() {
		t.Error("PipUser.Available() = true when python3 is missing")
	}
}
