package adapter

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestWinget_ListOutdated_ParsesUpgradeTable(t *testing.T) {
	installFakeBinary(t, "winget", `if [ "$1" = "upgrade" ]; then
cat <<'EOF'
Name                  Id                      Version     Available    Source
--------------------------------------------------------------------------
Git                   Git.Git                 2.47.0      2.48.0       winget
Neovim                Neovim.Neovim           0.10.0      0.11.0       winget
EOF
exit 0
fi
exit 1`)

	got, err := (Winget{}).ListOutdated(nil)
	if err != nil {
		t.Fatalf("ListOutdated: unexpected error: %v", err)
	}
	want := map[string]string{"Git.Git": "2.48.0", "Neovim.Neovim": "0.11.0"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ListOutdated: got %v, want %v", got, want)
	}
}

func TestWinget_ListOutdated_IntersectsWithPkgNames(t *testing.T) {
	installFakeBinary(t, "winget", `cat <<'EOF'
Name                  Id                      Version     Available    Source
--------------------------------------------------------------------------
Git                   Git.Git                 2.47.0      2.48.0       winget
Neovim                Neovim.Neovim           0.10.0      0.11.0       winget
EOF`)

	got, err := (Winget{}).ListOutdated([]string{"Neovim.Neovim"})
	if err != nil {
		t.Fatalf("ListOutdated: unexpected error: %v", err)
	}
	want := map[string]string{"Neovim.Neovim": "0.11.0"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ListOutdated: got %v, want %v", got, want)
	}
}

func TestWinget_ListOutdated_NothingOutdated(t *testing.T) {
	installFakeBinary(t, "winget", `printf "No installed package has an available upgrade.\n"`)

	got, err := (Winget{}).ListOutdated(nil)
	if err != nil {
		t.Fatalf("ListOutdated: unexpected error: %v", err)
	}
	if got != nil {
		t.Fatalf("ListOutdated: got %v, want nil", got)
	}
}

func TestWinget_ListOutdated_CommandFailureIsError(t *testing.T) {
	installFakeBinary(t, "winget", `echo "boom" >&2; exit 1`)

	if _, err := (Winget{}).ListOutdated(nil); err == nil {
		t.Fatal("ListOutdated: expected error on command failure, got nil")
	}
}

func TestScoop_ListOutdated_ParsesStatus(t *testing.T) {
	installFakeBinary(t, "scoop", `if [ "$1" = "status" ]; then
cat <<'EOF'
Name    Version  Available  Info
----    -------  ---------  ----
git     2.47.0   2.48.0
neovim  0.10.0   0.11.0
EOF
exit 0
fi
exit 1`)

	got, err := (Scoop{}).ListOutdated(nil)
	if err != nil {
		t.Fatalf("ListOutdated: unexpected error: %v", err)
	}
	want := map[string]string{"git": "2.48.0", "neovim": "0.11.0"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ListOutdated: got %v, want %v", got, want)
	}
}

func TestScoop_ListOutdated_IntersectsWithPkgNames(t *testing.T) {
	installFakeBinary(t, "scoop", `cat <<'EOF'
Name    Version  Available  Info
----    -------  ---------  ----
git     2.47.0   2.48.0
neovim  0.10.0   0.11.0
EOF`)

	got, err := (Scoop{}).ListOutdated([]string{"git"})
	if err != nil {
		t.Fatalf("ListOutdated: unexpected error: %v", err)
	}
	want := map[string]string{"git": "2.48.0"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ListOutdated: got %v, want %v", got, want)
	}
}

func TestScoop_ListOutdated_NothingOutdated(t *testing.T) {
	installFakeBinary(t, "scoop", `printf "Scoop is up to date.\n"`)

	got, err := (Scoop{}).ListOutdated(nil)
	if err != nil {
		t.Fatalf("ListOutdated: unexpected error: %v", err)
	}
	if got != nil {
		t.Fatalf("ListOutdated: got %v, want nil", got)
	}
}

func TestScoop_ListOutdated_CommandFailureIsError(t *testing.T) {
	installFakeBinary(t, "scoop", `echo "boom" >&2; exit 1`)

	if _, err := (Scoop{}).ListOutdated(nil); err == nil {
		t.Fatal("ListOutdated: expected error on command failure, got nil")
	}
}

func TestChoco_ListOutdated_ParsesRemoteOutput(t *testing.T) {
	installFakeBinary(t, "choco", `if [ "$1" = "outdated" ] && [ "$2" = "-r" ]; then
echo "git|2.47.0|2.48.0|false"
echo "nodejs|22.0.0|22.0.0|false"
exit 0
fi
exit 1`)

	got, err := (Choco{}).ListOutdated(nil)
	if err != nil {
		t.Fatalf("ListOutdated: unexpected error: %v", err)
	}
	want := map[string]string{"git": "2.48.0"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ListOutdated: got %v, want %v", got, want)
	}
}

func TestChoco_ListOutdated_ParsesRemoteOutputOnExitCode2(t *testing.T) {
	installFakeBinary(t, "choco", `if [ "$1" = "outdated" ] && [ "$2" = "-r" ]; then
echo "git|2.47.0|2.48.0|false"
echo "nodejs|22.0.0|22.0.0|false"
exit 2
fi
exit 1`)

	got, err := (Choco{}).ListOutdated(nil)
	if err != nil {
		t.Fatalf("ListOutdated: unexpected error: %v", err)
	}
	want := map[string]string{"git": "2.48.0"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ListOutdated: got %v, want %v", got, want)
	}
}

func TestChoco_ListOutdated_IntersectsWithPkgNames(t *testing.T) {
	installFakeBinary(t, "choco", `cat <<'EOF'
git|2.47.0|2.48.0|false
neovim|0.10.0|0.11.0|false
EOF`)

	got, err := (Choco{}).ListOutdated([]string{"neovim"})
	if err != nil {
		t.Fatalf("ListOutdated: unexpected error: %v", err)
	}
	want := map[string]string{"neovim": "0.11.0"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ListOutdated: got %v, want %v", got, want)
	}
}

func TestChoco_ListOutdated_NothingOutdated(t *testing.T) {
	installFakeBinary(t, "choco", `printf ""`)

	got, err := (Choco{}).ListOutdated(nil)
	if err != nil {
		t.Fatalf("ListOutdated: unexpected error: %v", err)
	}
	if got != nil {
		t.Fatalf("ListOutdated: got %v, want nil", got)
	}
}

func TestChoco_ListOutdated_CommandFailureIsError(t *testing.T) {
	installFakeBinary(t, "choco", `echo "boom" >&2; exit 1`)

	if _, err := (Choco{}).ListOutdated(nil); err == nil {
		t.Fatal("ListOutdated: expected error on command failure, got nil")
	}
}

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

func TestUv_ListOutdated_ReportsOnlyDiffering(t *testing.T) {
	installFakeBinary(t, "uv",
		`if [ "$1" = "tool" ] && [ "$2" = "list" ]; then
	echo "ruff v0.6.0"
	echo "black v24.0.0"
	exit 0
fi
echo "unexpected args: $*" >&2
exit 1`)

	restore := swapPypiLatest(t, map[string]string{"ruff": "0.7.0", "black": "24.0.0"}, nil)
	defer restore()

	got, err := Uv{}.ListOutdated([]string{"ruff", "black"})
	if err != nil {
		t.Fatalf("ListOutdated: %v", err)
	}
	want := map[string]string{"ruff": "0.7.0"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestPipx_ListOutdated_ReportsOnlyDiffering(t *testing.T) {
	installFakeBinary(t, "pipx",
		`if [ "$1" = "list" ] && [ "$2" = "--json" ]; then
	cat <<'JSON'
{"venvs":{"ruff":{"metadata":{"main_package":{"package":"ruff","package_version":"0.6.0"}}},"black":{"metadata":{"main_package":{"package":"black","package_version":"24.0.0"}}}}}
JSON
	exit 0
fi
echo "unexpected args: $*" >&2
exit 1`)

	restore := swapPypiLatest(t, map[string]string{"ruff": "0.7.0", "black": "24.0.0"}, nil)
	defer restore()

	got, err := Pipx{}.ListOutdated([]string{"ruff", "black"})
	if err != nil {
		t.Fatalf("ListOutdated: %v", err)
	}
	want := map[string]string{"ruff": "0.7.0"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
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

func TestFetchPypiLatest_DecodesVersionAndEscapesName(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.EscapedPath()
		switch r.URL.Path {
		case "/pypi/ruff/json":
			w.Write([]byte(`{"info":{"version":"0.7.0"}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	orig := pypiRegistryBase
	pypiRegistryBase = srv.URL
	defer func() { pypiRegistryBase = orig }()

	v, err := fetchPypiLatest("ruff")
	if err != nil || v != "0.7.0" {
		t.Fatalf("fetchPypiLatest(ruff) = %q, %v; want 0.7.0, nil", v, err)
	}
	if gotPath != "/pypi/ruff/json" {
		t.Fatalf("request path = %q, want /pypi/ruff/json", gotPath)
	}

	v, err = fetchPypiLatest("does-not-exist")
	if err != nil || v != "" {
		t.Fatalf("fetchPypiLatest(does-not-exist) = %q, %v; want \"\", nil (404)", v, err)
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

// swapPypiLatest replaces the pypiLatestVersion seam with a fake returning
// canned versions (or a transport error for names in errNames).
func swapPypiLatest(t *testing.T, versions map[string]string, errNames map[string]bool) func() {
	t.Helper()
	orig := pypiLatestVersion
	pypiLatestVersion = func(name string) (string, error) {
		if errNames[name] {
			return "", http.ErrHandlerTimeout
		}
		return versions[name], nil
	}
	return func() { pypiLatestVersion = orig }
}

func TestCargo_ListOutdated_ReportsOnlyDiffering(t *testing.T) {
	installFakeBinary(t, "cargo",
		`if [ "$1" = "install" ] && [ "$2" = "--list" ]; then
	echo "ripgrep v14.0.0:"
	echo "    rg"
	echo "fd-find v9.0.0:"
	echo "    fd"
	exit 0
fi
echo "unexpected args: $*" >&2
exit 1`)

	restore := swapCratesLatest(t, map[string]string{"ripgrep": "14.1.0", "fd-find": "9.0.0"}, nil)
	defer restore()

	got, err := Cargo{}.ListOutdated([]string{"ripgrep", "fd-find"})
	if err != nil {
		t.Fatalf("ListOutdated: %v", err)
	}
	want := map[string]string{"ripgrep": "14.1.0"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestFetchCratesLatest_DecodesVersionAndSetsUserAgent(t *testing.T) {
	var gotPath, gotUA string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.EscapedPath()
		gotUA = r.Header.Get("User-Agent")
		switch r.URL.Path {
		case "/api/v1/crates/ripgrep":
			w.Write([]byte(`{"crate":{"max_version":"14.1.0"}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	orig := cratesRegistryBase
	cratesRegistryBase = srv.URL
	defer func() { cratesRegistryBase = orig }()

	v, err := fetchCratesLatest("ripgrep")
	if err != nil || v != "14.1.0" {
		t.Fatalf("fetchCratesLatest(ripgrep) = %q, %v; want 14.1.0, nil", v, err)
	}
	if gotPath != "/api/v1/crates/ripgrep" {
		t.Fatalf("request path = %q, want /api/v1/crates/ripgrep", gotPath)
	}
	if gotUA != "genv (https://github.com/ks1686/genv)" {
		t.Fatalf("User-Agent = %q, want genv (https://github.com/ks1686/genv)", gotUA)
	}

	v, err = fetchCratesLatest("does-not-exist")
	if err != nil || v != "" {
		t.Fatalf("fetchCratesLatest(does-not-exist) = %q, %v; want \"\", nil (404)", v, err)
	}
}

// swapCratesLatest replaces the cratesLatestVersion seam with a fake returning
// canned versions (or a transport error for names in errNames).
func swapCratesLatest(t *testing.T, versions map[string]string, errNames map[string]bool) func() {
	t.Helper()
	orig := cratesLatestVersion
	cratesLatestVersion = func(name string) (string, error) {
		if errNames[name] {
			return "", http.ErrHandlerTimeout
		}
		return versions[name], nil
	}
	return func() { cratesLatestVersion = orig }
}
