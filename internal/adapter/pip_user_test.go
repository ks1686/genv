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

func TestPipUser_ListForScan_UsesNotRequiredAndSkipsNoise(t *testing.T) {
	installFakeBinary(t, "python3",
		`if [ "$1" = "-m" ] && [ "$2" = "pip" ] && [ "$3" = "list" ]; then
  not_required=0
  for arg in "$@"; do
    if [ "$arg" = "--not-required" ]; then
      not_required=1
    fi
  done
  if [ "$not_required" = "1" ]; then
    echo '[{"name":"trafilatura","version":"1.6.0"},{"name":"numpy","version":"2.0.0"},{"name":"certifi","version":"2024.8.30"},{"name":"setuptools","version":"70.0.0"}]'
    exit 0
  fi
  echo '[{"name":"trafilatura","version":"1.6.0"},{"name":"htmldate","version":"1.9.0"},{"name":"numpy","version":"2.0.0"},{"name":"certifi","version":"2024.8.30"}]'
  exit 0
fi
echo "unexpected args: $*" >&2
exit 1`)

	got, err := PipUser{}.ListForScan()
	if err != nil {
		t.Fatalf("ListForScan: %v", err)
	}
	want := map[string]bool{"trafilatura": true, "numpy": true}
	if len(got) != len(want) {
		t.Fatalf("ListForScan = %v, want trafilatura and numpy (not deps or installer noise)", got)
	}
	for _, name := range got {
		if !want[name] {
			t.Errorf("ListForScan included %q", name)
		}
	}
	full, err := PipUser{}.ListInstalled()
	if err != nil {
		t.Fatalf("ListInstalled: %v", err)
	}
	hasDep := false
	for _, name := range full {
		if name == "htmldate" {
			hasDep = true
		}
	}
	if !hasDep {
		t.Fatal("precondition: ListInstalled should still report pip-user transitives")
	}
}

func TestPipUser_ListOutdated_empty(t *testing.T) {
	got, err := PipUser{}.ListOutdated(nil)
	if err != nil {
		t.Fatalf("ListOutdated: %v", err)
	}
	_ = got
}
