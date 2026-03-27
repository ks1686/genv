Name:           genv
Version:        2.1.0
Release:        1%{?dist}
Summary:        Track, sync, and reproduce your software environment

License:        MIT
URL:            https://github.com/ks1686/genv
Source0:        https://github.com/ks1686/genv/archive/refs/tags/v%{version}.tar.gz

BuildRequires:  golang
%global debug_package %{nil}

%description
genv is a command-line tool to track, sync, and reproduce your software
environment across Linux, macOS, and WSL2. It manages packages, environment
variables, shell aliases, and services from a single declarative genv.json file.

%prep
%autosetup -n genv-%{version}

%build
export CGO_ENABLED=0
go build -o genv -ldflags "-s -w -X main.version=%{version}"

%install
install -Dpm 0755 genv                  %{buildroot}%{_bindir}/genv
install -Dpm 0644 completions/genv.bash %{buildroot}%{_datadir}/bash-completion/completions/genv
install -Dpm 0644 completions/genv.zsh  %{buildroot}%{_datadir}/zsh/site-functions/_genv
install -Dpm 0644 completions/genv.fish %{buildroot}%{_datadir}/fish/vendor_completions.d/genv.fish

%files
%{_bindir}/genv
%{_datadir}/bash-completion/completions/genv
%{_datadir}/zsh/site-functions/_genv
%{_datadir}/fish/vendor_completions.d/genv.fish

%changelog
* Fri Mar 27 2026 ks1686 <ks1686@users.noreply.github.com> - 2.1.0-1
- Update to v2.1.0 (M10: services management, new adapters: apk/zypper/xbps/emerge)
- Assisted-by: Claude Sonnet 4.6

* Fri Mar 27 2026 ks1686 <ks1686@users.noreply.github.com> - 2.0.1-1
- Initial package
