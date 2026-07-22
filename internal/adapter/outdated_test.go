package adapter

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

type nativeOutdatedCase struct {
	name      string
	bin       string
	script    string
	pkgNames  []string
	want      map[string]string
	wantNil   bool
	wantError bool
	list      func(pkgNames []string) (map[string]string, error)
}

func TestNativeListOutdated(t *testing.T) {
	wingetTable := `Name                  Id                      Version     Available    Source
--------------------------------------------------------------------------
Git                   Git.Git                 2.47.0      2.48.0       winget
Neovim                Neovim.Neovim           0.10.0      0.11.0       winget
`
	scoopStatus := `Name    Version  Available  Info
----    -------  ---------  ----
git     2.47.0   2.48.0
neovim  0.10.0   0.11.0
`
	quLines := "git 2.47.0 -> 2.48.0\nneovim 0.10.0 -> 0.11.0\n"
	snapList := `Name           Version  Rev   Publisher     Notes
firefox        139.0-1  1235  mozilla       -
code           1.95.0   200   vscode        classic
`

	cases := []nativeOutdatedCase{
		{
			name: "winget/parse", bin: "winget",
			script: `if [ "$1" = "upgrade" ]; then cat <<'EOF'
` + wingetTable + `EOF
exit 0
fi
exit 1`,
			want: map[string]string{"Git.Git": "2.48.0", "Neovim.Neovim": "0.11.0"},
			list: (Winget{}).ListOutdated,
		},
		{
			name: "winget/intersect", bin: "winget", script: "cat <<'EOF'\n" + wingetTable + "EOF\n",
			pkgNames: []string{"Neovim.Neovim"},
			want:     map[string]string{"Neovim.Neovim": "0.11.0"},
			list:     (Winget{}).ListOutdated,
		},
		{
			name: "winget/empty", bin: "winget", script: `printf "No installed package has an available upgrade.\n"`,
			wantNil: true, list: (Winget{}).ListOutdated,
		},
		{
			name: "winget/error", bin: "winget", script: `echo "boom" >&2; exit 1`,
			wantError: true, list: (Winget{}).ListOutdated,
		},
		{
			name: "scoop/parse", bin: "scoop",
			script: `if [ "$1" = "status" ]; then cat <<'EOF'
` + scoopStatus + `EOF
exit 0
fi
exit 1`,
			want: map[string]string{"git": "2.48.0", "neovim": "0.11.0"},
			list: (Scoop{}).ListOutdated,
		},
		{
			name: "scoop/intersect", bin: "scoop", script: "cat <<'EOF'\n" + scoopStatus + "EOF\n",
			pkgNames: []string{"git"}, want: map[string]string{"git": "2.48.0"},
			list: (Scoop{}).ListOutdated,
		},
		{
			name: "scoop/empty", bin: "scoop", script: `printf "Scoop is up to date.\n"`,
			wantNil: true, list: (Scoop{}).ListOutdated,
		},
		{
			name: "scoop/error", bin: "scoop", script: `echo "boom" >&2; exit 1`,
			wantError: true, list: (Scoop{}).ListOutdated,
		},
		{
			name: "choco/parse", bin: "choco",
			script: `if [ "$1" = "outdated" ] && [ "$2" = "-r" ]; then
echo "git|2.47.0|2.48.0|false"
echo "nodejs|22.0.0|22.0.0|false"
exit 0
fi
exit 1`,
			want: map[string]string{"git": "2.48.0"}, list: (Choco{}).ListOutdated,
		},
		{
			name: "choco/exit2", bin: "choco",
			script: `if [ "$1" = "outdated" ] && [ "$2" = "-r" ]; then
echo "git|2.47.0|2.48.0|false"
echo "nodejs|22.0.0|22.0.0|false"
exit 2
fi
exit 1`,
			want: map[string]string{"git": "2.48.0"}, list: (Choco{}).ListOutdated,
		},
		{
			name: "choco/intersect", bin: "choco",
			script:   "cat <<'EOF'\ngit|2.47.0|2.48.0|false\nneovim|0.10.0|0.11.0|false\nEOF\n",
			pkgNames: []string{"neovim"}, want: map[string]string{"neovim": "0.11.0"},
			list: (Choco{}).ListOutdated,
		},
		{
			name: "choco/empty", bin: "choco", script: `printf ""`,
			wantNil: true, list: (Choco{}).ListOutdated,
		},
		{
			name: "choco/error", bin: "choco", script: `echo "boom" >&2; exit 1`,
			wantError: true, list: (Choco{}).ListOutdated,
		},
		{
			name: "brew/parse", bin: "brew",
			script: `if [ "$1" = "outdated" ] && [ "$2" = "--json=v2" ]; then
	cat <<'JSON'
{"formulae":[{"name":"wget","current_version":"1.21.4"},{"name":"jq","current_version":"1.7"}],"casks":[{"name":"firefox","current_version":"121.0"}]}
JSON
	exit 0
fi
echo "unexpected args: $*" >&2
exit 1`,
			want: map[string]string{"wget": "1.21.4", "jq": "1.7", "firefox": "121.0"},
			list: Brew{}.ListOutdated,
		},
		{
			name: "brew/intersect", bin: "brew",
			script: `cat <<'JSON'
{"formulae":[{"name":"wget","current_version":"1.21.4"},{"name":"jq","current_version":"1.7"}],"casks":[]}
JSON`,
			pkgNames: []string{"jq"}, want: map[string]string{"jq": "1.7"},
			list: Brew{}.ListOutdated,
		},
		{
			name: "brew/empty", bin: "brew", script: `echo '{"formulae": [], "casks": []}'`,
			wantNil: true, list: Brew{}.ListOutdated,
		},
		{
			name: "brew/error", bin: "brew", script: `echo "boom" >&2; exit 1`,
			wantError: true, list: Brew{}.ListOutdated,
		},
		{
			name: "mas/parse", bin: "mas",
			script: `if [ "$1" = "outdated" ]; then
	echo "497799835 Xcode (14.0 -> 14.1)"
	echo "409201541 Pages (13.1)"
	exit 0
fi
echo "unexpected args: $*" >&2
exit 1`,
			want: map[string]string{"497799835": "14.1", "409201541": "13.1"},
			list: Mas{}.ListOutdated,
		},
		{
			name: "mas/intersect", bin: "mas",
			script:   "echo \"497799835 Xcode (14.1)\"\necho \"409201541 Pages (13.1)\"\n",
			pkgNames: []string{"497799835"}, want: map[string]string{"497799835": "14.1"},
			list: Mas{}.ListOutdated,
		},
		{
			name: "pacman/parse", bin: "pacman",
			script: `if [ "$1" = "-Qu" ]; then cat <<'EOF'
` + quLines + `EOF
exit 0
fi
exit 1`,
			want: map[string]string{"git": "2.48.0", "neovim": "0.11.0"},
			list: (Pacman{}).ListOutdated,
		},
		{
			name: "pacman/intersect", bin: "pacman", script: "cat <<'EOF'\n" + quLines + "EOF\n",
			pkgNames: []string{"neovim"}, want: map[string]string{"neovim": "0.11.0"},
			list: (Pacman{}).ListOutdated,
		},
		{
			name: "pacman/empty", bin: "pacman", script: `exit 1`,
			wantNil: true, list: (Pacman{}).ListOutdated,
		},
		{
			name: "pacman/error", bin: "pacman", script: `echo "boom" >&2; exit 2`,
			wantError: true, list: (Pacman{}).ListOutdated,
		},
		{
			name: "paru/parse", bin: "paru",
			script: `if [ "$1" = "-Qu" ]; then cat <<'EOF'
` + quLines + `EOF
exit 0
fi
exit 1`,
			want: map[string]string{"git": "2.48.0", "neovim": "0.11.0"},
			list: (Paru{}).ListOutdated,
		},
		{
			name: "paru/intersect", bin: "paru", script: "cat <<'EOF'\n" + quLines + "EOF\n",
			pkgNames: []string{"git"}, want: map[string]string{"git": "2.48.0"},
			list: (Paru{}).ListOutdated,
		},
		{
			name: "paru/empty", bin: "paru", script: `exit 1`,
			wantNil: true, list: (Paru{}).ListOutdated,
		},
		{
			name: "paru/error", bin: "paru", script: `echo "boom" >&2; exit 2`,
			wantError: true, list: (Paru{}).ListOutdated,
		},
		{
			name: "yay/parse", bin: "yay",
			script: `if [ "$1" = "-Qu" ]; then cat <<'EOF'
` + quLines + `EOF
exit 0
fi
exit 1`,
			want: map[string]string{"git": "2.48.0", "neovim": "0.11.0"},
			list: (Yay{}).ListOutdated,
		},
		{
			name: "yay/intersect", bin: "yay", script: "cat <<'EOF'\n" + quLines + "EOF\n",
			pkgNames: []string{"neovim"}, want: map[string]string{"neovim": "0.11.0"},
			list: (Yay{}).ListOutdated,
		},
		{
			name: "yay/empty", bin: "yay", script: `exit 1`,
			wantNil: true, list: (Yay{}).ListOutdated,
		},
		{
			name: "yay/error", bin: "yay", script: `echo "boom" >&2; exit 2`,
			wantError: true, list: (Yay{}).ListOutdated,
		},
		{
			name: "snap/parse", bin: "snap",
			script: `if [ "$1" = "refresh" ] && [ "$2" = "--list" ]; then cat <<'EOF'
` + snapList + `EOF
exit 0
fi
exit 1`,
			want: map[string]string{"firefox": "139.0-1", "code": "1.95.0"},
			list: (Snap{}).ListOutdated,
		},
		{
			name: "snap/intersect", bin: "snap",
			script: `if [ "$1" = "refresh" ] && [ "$2" = "--list" ]; then cat <<'EOF'
` + snapList + `EOF
exit 0
fi
exit 1`,
			pkgNames: []string{"code"}, want: map[string]string{"code": "1.95.0"},
			list: (Snap{}).ListOutdated,
		},
		{
			name: "snap/empty", bin: "snap",
			script:  `if [ "$1" = "refresh" ] && [ "$2" = "--list" ]; then printf ""; exit 0; fi; exit 1`,
			wantNil: true, list: (Snap{}).ListOutdated,
		},
		{
			name: "snap/error", bin: "snap", script: `echo "boom" >&2; exit 2`,
			wantError: true, list: (Snap{}).ListOutdated,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			installFakeBinary(t, tc.bin, tc.script)
			got, err := tc.list(tc.pkgNames)
			if tc.wantError {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.wantNil {
				if got != nil {
					t.Fatalf("got %v, want nil", got)
				}
				return
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestRegistryListOutdated_ReportsOnlyDiffering(t *testing.T) {
	cases := []struct {
		name     string
		bin      string
		script   string
		swap     func(*testing.T) func()
		pkgNames []string
		want     map[string]string
		list     func([]string) (map[string]string, error)
	}{
		{
			name: "bun", bin: "bun",
			script: `if [ "$1" = "pm" ] && [ "$2" = "ls" ] && [ "$3" = "--global" ]; then
	echo "/Users/x/.bun/install/global node_modules"
	echo "├── cf@1.2.0"
	echo "└── typescript@5.0.0"
	exit 0
fi
echo "unexpected args: $*" >&2
exit 1`,
			swap: func(t *testing.T) func() {
				return swapNpmLatest(t, map[string]string{"cf": "1.3.0", "typescript": "5.0.0"}, nil)
			},
			pkgNames: []string{"cf", "typescript"}, want: map[string]string{"cf": "1.3.0"},
			list: Bun{}.ListOutdated,
		},
		{
			name: "npm", bin: "npm",
			script: `if [ "$1" = "list" ]; then
	cat <<'JSON'
{"dependencies":{"cf":{"version":"1.2.0"},"typescript":{"version":"5.0.0"}}}
JSON
	exit 0
fi
echo "unexpected: $*" >&2; exit 1`,
			swap: func(t *testing.T) func() {
				return swapNpmLatest(t, map[string]string{"cf": "1.3.0", "typescript": "5.0.0"}, nil)
			},
			pkgNames: []string{"cf", "typescript"}, want: map[string]string{"cf": "1.3.0"},
			list: Npm{}.ListOutdated,
		},
		{
			name: "pnpm", bin: "pnpm",
			script: `if [ "$1" = "list" ]; then
	cat <<'JSON'
[{"dependencies":{"cf":{"version":"1.2.0"},"typescript":{"version":"5.0.0"}}}]
JSON
	exit 0
fi
echo "unexpected: $*" >&2; exit 1`,
			swap: func(t *testing.T) func() {
				return swapNpmLatest(t, map[string]string{"cf": "1.3.0", "typescript": "5.0.0"}, nil)
			},
			pkgNames: []string{"cf", "typescript"}, want: map[string]string{"cf": "1.3.0"},
			list: Pnpm{}.ListOutdated,
		},
		{
			name: "yarn", bin: "yarn",
			script: `if [ "$1" = "global" ] && [ "$2" = "list" ]; then
	echo 'yarn global v1.22.22'
	echo 'info "cf@1.2.0" has binaries:'
	echo 'info "typescript@5.0.0" has binaries:'
	exit 0
fi
echo "unexpected: $*" >&2; exit 1`,
			swap: func(t *testing.T) func() {
				return swapNpmLatest(t, map[string]string{"cf": "1.3.0", "typescript": "5.0.0"}, nil)
			},
			pkgNames: []string{"cf", "typescript"}, want: map[string]string{"cf": "1.3.0"},
			list: Yarn{}.ListOutdated,
		},
		{
			name: "uv", bin: "uv",
			script: `if [ "$1" = "tool" ] && [ "$2" = "list" ]; then
	echo "ruff v0.6.0"
	echo "black v24.0.0"
	exit 0
fi
echo "unexpected args: $*" >&2
exit 1`,
			swap: func(t *testing.T) func() {
				return swapPypiLatest(t, map[string]string{"ruff": "0.7.0", "black": "24.0.0"}, nil)
			},
			pkgNames: []string{"ruff", "black"}, want: map[string]string{"ruff": "0.7.0"},
			list: Uv{}.ListOutdated,
		},
		{
			name: "pipx", bin: "pipx",
			script: `if [ "$1" = "list" ] && [ "$2" = "--json" ]; then
	cat <<'JSON'
{"venvs":{"ruff":{"metadata":{"main_package":{"package":"ruff","package_version":"0.6.0"}}},"black":{"metadata":{"main_package":{"package":"black","package_version":"24.0.0"}}}}}
JSON
	exit 0
fi
echo "unexpected args: $*" >&2
exit 1`,
			swap: func(t *testing.T) func() {
				return swapPypiLatest(t, map[string]string{"ruff": "0.7.0", "black": "24.0.0"}, nil)
			},
			pkgNames: []string{"ruff", "black"}, want: map[string]string{"ruff": "0.7.0"},
			list: Pipx{}.ListOutdated,
		},
		{
			name: "cargo", bin: "cargo",
			script: `if [ "$1" = "install" ] && [ "$2" = "--list" ]; then
	echo "ripgrep v14.0.0:"
	echo "    rg"
	echo "fd-find v9.0.0:"
	echo "    fd"
	exit 0
fi
echo "unexpected args: $*" >&2
exit 1`,
			swap: func(t *testing.T) func() {
				return swapCratesLatest(t, map[string]string{"ripgrep": "14.1.0", "fd-find": "9.0.0"}, nil)
			},
			pkgNames: []string{"ripgrep", "fd-find"}, want: map[string]string{"ripgrep": "14.1.0"},
			list: Cargo{}.ListOutdated,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			installFakeBinary(t, tc.bin, tc.script)
			defer tc.swap(t)()
			got, err := tc.list(tc.pkgNames)
			if err != nil {
				t.Fatalf("ListOutdated: %v", err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
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
	if gotUA != cratesUserAgent {
		t.Fatalf("User-Agent = %q, want %q", gotUA, cratesUserAgent)
	}

	v, err = fetchCratesLatest("does-not-exist")
	if err != nil || v != "" {
		t.Fatalf("fetchCratesLatest(does-not-exist) = %q, %v; want \"\", nil (404)", v, err)
	}
}

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
