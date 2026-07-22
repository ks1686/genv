package adapter

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestBrew_ListOutdated_ParsesFormulaeAndCasks(t *testing.T) {
	installFakeBinary(t, "brew",
		`if [ "$1" = "outdated" ] && [ "$2" = "--json=v2" ]; then
	cat <<'JSON'
{
  "formulae": [
    {"name": "wget", "installed_versions": ["1.21.3"], "current_version": "1.21.4"},
    {"name": "jq", "installed_versions": ["1.6"], "current_version": "1.7"}
  ],
  "casks": [
    {"name": "firefox", "installed_versions": ["120.0"], "current_version": "121.0"}
  ]
}
JSON
	exit 0
fi
echo "unexpected args: $*" >&2
exit 1`)

	got, err := Brew{}.ListOutdated(nil)
	if err != nil {
		t.Fatalf("ListOutdated: unexpected error: %v", err)
	}
	want := map[string]string{"wget": "1.21.4", "jq": "1.7", "firefox": "121.0"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ListOutdated: got %v, want %v", got, want)
	}
}

func TestBrew_ListOutdated_IntersectsWithPkgNames(t *testing.T) {
	installFakeBinary(t, "brew",
		`cat <<'JSON'
{"formulae": [{"name": "wget", "current_version": "1.21.4"}, {"name": "jq", "current_version": "1.7"}], "casks": []}
JSON`)

	got, err := Brew{}.ListOutdated([]string{"jq"})
	if err != nil {
		t.Fatalf("ListOutdated: unexpected error: %v", err)
	}
	want := map[string]string{"jq": "1.7"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ListOutdated: got %v, want %v", got, want)
	}
}

func TestBrew_ListOutdated_NothingOutdated(t *testing.T) {
	installFakeBinary(t, "brew", `echo '{"formulae": [], "casks": []}'`)
	got, err := Brew{}.ListOutdated(nil)
	if err != nil {
		t.Fatalf("ListOutdated: unexpected error: %v", err)
	}
	if got != nil {
		t.Fatalf("ListOutdated: got %v, want nil", got)
	}
}

func TestBrew_ListOutdated_CommandFailureIsError(t *testing.T) {
	installFakeBinary(t, "brew", `echo "boom" >&2; exit 1`)
	if _, err := (Brew{}).ListOutdated(nil); err == nil {
		t.Fatal("ListOutdated: expected error on command failure, got nil")
	}
}

func TestMas_ListOutdated_ParsesArrowAndPlainForms(t *testing.T) {
	installFakeBinary(t, "mas",
		`if [ "$1" = "outdated" ]; then
	echo "497799835 Xcode (14.0 -> 14.1)"
	echo "409201541 Pages (13.1)"
	exit 0
fi
echo "unexpected args: $*" >&2
exit 1`)

	got, err := Mas{}.ListOutdated(nil)
	if err != nil {
		t.Fatalf("ListOutdated: unexpected error: %v", err)
	}
	want := map[string]string{"497799835": "14.1", "409201541": "13.1"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ListOutdated: got %v, want %v", got, want)
	}
}

func TestMas_ListOutdated_IntersectsWithPkgNames(t *testing.T) {
	installFakeBinary(t, "mas",
		`echo "497799835 Xcode (14.1)"
echo "409201541 Pages (13.1)"`)

	got, err := Mas{}.ListOutdated([]string{"497799835"})
	if err != nil {
		t.Fatalf("ListOutdated: unexpected error: %v", err)
	}
	want := map[string]string{"497799835": "14.1"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ListOutdated: got %v, want %v", got, want)
	}
}

func TestBun_ListOutdated_ReportsOnlyDiffering(t *testing.T) {
	installFakeBinary(t, "bun",
		`if [ "$1" = "pm" ] && [ "$2" = "ls" ] && [ "$3" = "--global" ]; then
	echo "/Users/x/.bun/install/global node_modules"
	echo "├── cf@1.2.0"
	echo "└── typescript@5.0.0"
	exit 0
fi
echo "unexpected args: $*" >&2
exit 1`)

	restore := swapNpmLatest(t, map[string]string{"cf": "1.3.0", "typescript": "5.0.0"}, nil)
	defer restore()

	got, err := Bun{}.ListOutdated([]string{"cf", "typescript"})
	if err != nil {
		t.Fatalf("ListOutdated: unexpected error: %v", err)
	}
	want := map[string]string{"cf": "1.3.0"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ListOutdated: got %v, want %v", got, want)
	}
}

func TestNpm_ListOutdated_ReportsOnlyDiffering(t *testing.T) {
	installFakeBinary(t, "npm",
		`if [ "$1" = "list" ]; then
	cat <<'JSON'
{"dependencies":{"cf":{"version":"1.2.0"},"typescript":{"version":"5.0.0"}}}
JSON
	exit 0
fi
echo "unexpected: $*" >&2; exit 1`)

	restore := swapNpmLatest(t, map[string]string{"cf": "1.3.0", "typescript": "5.0.0"}, nil)
	defer restore()

	got, err := Npm{}.ListOutdated([]string{"cf", "typescript"})
	if err != nil {
		t.Fatalf("ListOutdated: %v", err)
	}
	want := map[string]string{"cf": "1.3.0"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestPnpm_ListOutdated_ReportsOnlyDiffering(t *testing.T) {
	installFakeBinary(t, "pnpm",
		`if [ "$1" = "list" ]; then
	cat <<'JSON'
[{"dependencies":{"cf":{"version":"1.2.0"},"typescript":{"version":"5.0.0"}}}]
JSON
	exit 0
fi
echo "unexpected: $*" >&2; exit 1`)

	restore := swapNpmLatest(t, map[string]string{"cf": "1.3.0", "typescript": "5.0.0"}, nil)
	defer restore()

	got, err := Pnpm{}.ListOutdated([]string{"cf", "typescript"})
	if err != nil {
		t.Fatalf("ListOutdated: %v", err)
	}
	want := map[string]string{"cf": "1.3.0"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestYarn_ListOutdated_ReportsOnlyDiffering(t *testing.T) {
	installFakeBinary(t, "yarn",
		`if [ "$1" = "global" ] && [ "$2" = "list" ]; then
	echo 'yarn global v1.22.22'
	echo 'info "cf@1.2.0" has binaries:'
	echo 'info "typescript@5.0.0" has binaries:'
	exit 0
fi
echo "unexpected: $*" >&2; exit 1`)

	restore := swapNpmLatest(t, map[string]string{"cf": "1.3.0", "typescript": "5.0.0"}, nil)
	defer restore()

	got, err := Yarn{}.ListOutdated([]string{"cf", "typescript"})
	if err != nil {
		t.Fatalf("ListOutdated: %v", err)
	}
	want := map[string]string{"cf": "1.3.0"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestBun_ListOutdated_TransportErrorFlagsConservatively(t *testing.T) {
	installFakeBinary(t, "bun",
		`echo "/Users/x/.bun/install/global node_modules"
echo "└── cf@1.2.0"`)

	restore := swapNpmLatest(t, nil, map[string]bool{"cf": true})
	defer restore()

	got, err := Bun{}.ListOutdated([]string{"cf"})
	if err != nil {
		t.Fatalf("ListOutdated: unexpected error: %v", err)
	}
	// Unknown latest (transport error) => keep the installed version so the
	// package is still flagged as potentially outdated.
	want := map[string]string{"cf": "1.2.0"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ListOutdated: got %v, want %v", got, want)
	}
}

func TestFetchNpmLatest_DecodesVersionAndEncodesScopedName(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.EscapedPath()
		switch r.URL.Path {
		case "/@scope/pkg/latest":
			w.Write([]byte(`{"version": "2.1.0"}`))
		case "/typescript/latest":
			w.Write([]byte(`{"version": "5.4.2"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	orig := npmRegistryBase
	npmRegistryBase = srv.URL
	defer func() { npmRegistryBase = orig }()

	v, err := fetchNpmLatest("typescript")
	if err != nil || v != "5.4.2" {
		t.Fatalf("fetchNpmLatest(typescript) = %q, %v; want 5.4.2, nil", v, err)
	}

	v, err = fetchNpmLatest("@scope/pkg")
	if err != nil || v != "2.1.0" {
		t.Fatalf("fetchNpmLatest(@scope/pkg) = %q, %v; want 2.1.0, nil", v, err)
	}
	if gotPath != "/@scope%2Fpkg/latest" {
		t.Fatalf("scoped request path = %q, want /@scope%%2Fpkg/latest", gotPath)
	}

	v, err = fetchNpmLatest("does-not-exist")
	if err != nil || v != "" {
		t.Fatalf("fetchNpmLatest(does-not-exist) = %q, %v; want \"\", nil (404)", v, err)
	}
}

// swapNpmLatest replaces the npmLatestVersion seam with a fake returning canned
// versions (or a transport error for names in errNames). It returns a restore
// function.
func swapNpmLatest(t *testing.T, versions map[string]string, errNames map[string]bool) func() {
	t.Helper()
	orig := npmLatestVersion
	npmLatestVersion = func(name string) (string, error) {
		if errNames[name] {
			return "", http.ErrHandlerTimeout
		}
		return versions[name], nil
	}
	return func() { npmLatestVersion = orig }
}
