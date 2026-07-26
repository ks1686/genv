package adapter

import "testing"

func TestWindowsAndAURSearchWithFakes(t *testing.T) {
	installFakeBinary(t, "winget", `
if [ "$1" = "search" ]; then
  echo "Name                   Id         Version"
  echo "-----------------------------------------"
  echo "Git                    Git.Git    2.40.0"
  echo "GitHub CLI             GitHub.cli 2.0.0"
fi`)
	names, err := Winget{}.Search("git")
	if err != nil {
		t.Fatalf("Winget.Search: %v", err)
	}
	if len(names) == 0 {
		t.Fatalf("Winget.Search returned no names")
	}

	installFakeBinary(t, "scoop", `
if [ "$1" = "search" ]; then
  echo "Name    Version"
  echo "----    -------"
  echo "git     2.40.0"
  echo "git-lfs 3.0.0"
fi`)
	names, err = Scoop{}.Search("git")
	if err != nil {
		t.Fatalf("Scoop.Search: %v", err)
	}
	if len(names) < 1 {
		t.Fatalf("Scoop.Search = %v", names)
	}

	installFakeBinary(t, "choco", `
if [ "$1" = "search" ]; then
  echo "Chocolatey v2.0.0"
  echo "git 2.40.0"
  echo "git.install 2.40.0"
fi`)
	names, err = Choco{}.Search("git")
	if err != nil {
		t.Fatalf("Choco.Search: %v", err)
	}
	if len(names) < 1 {
		t.Fatalf("Choco.Search = %v", names)
	}

	installFakeBinary(t, "paru", `
if [ "$1" = "-Ss" ]; then
  echo "extra/git 2.40.0"
  echo "    the stupid content tracker"
  echo "aur/git-extras 1.0"
  echo "    extra git helpers"
fi`)
	names, err = Paru{}.Search("git")
	if err != nil {
		t.Fatalf("Paru.Search: %v", err)
	}
	if len(names) < 1 {
		t.Fatalf("Paru.Search = %v", names)
	}

	installFakeBinary(t, "yay", `
if [ "$1" = "-Ss" ]; then
  echo "extra/git 2.40.0"
  echo "    the stupid content tracker"
fi`)
	names, err = Yay{}.Search("git")
	if err != nil {
		t.Fatalf("Yay.Search: %v", err)
	}
	if len(names) < 1 {
		t.Fatalf("Yay.Search = %v", names)
	}
}

func TestVersionListerHelpersWithFakes(t *testing.T) {
	installFakeBinary(t, "pacman", `
if [ "$1" = "-Q" ]; then
  echo "git 2.40.0"
  echo "curl 8.0.0"
fi`)
	versions, err := Paru{}.ListInstalledVersions()
	if err != nil {
		t.Fatalf("Paru.ListInstalledVersions: %v", err)
	}
	if versions["git"] != "2.40.0" || versions["curl"] != "8.0.0" {
		t.Fatalf("paru versions = %#v", versions)
	}
	versions, err = Yay{}.ListInstalledVersions()
	if err != nil {
		t.Fatalf("Yay.ListInstalledVersions: %v", err)
	}
	if versions["git"] != "2.40.0" {
		t.Fatalf("yay versions = %#v", versions)
	}

	installFakeBinary(t, "mas", `
if [ "$1" = "list" ]; then
  echo "497799835 Xcode (15.0)"
fi`)
	versions, err = Mas{}.ListInstalledVersions()
	if err != nil {
		t.Fatalf("Mas.ListInstalledVersions: %v", err)
	}
	if versions["497799835"] == "" {
		t.Fatalf("mas versions = %#v", versions)
	}

	installFakeBinary(t, "pipx", `
if [ "$1" = "list" ]; then
  cat <<'EOF'
{"venvs":{"ruff":{"metadata":{"main_package":{"package":"ruff","package_version":"0.5.0"}}}}}
EOF
fi`)
	versions, err = Pipx{}.ListInstalledVersions()
	if err != nil || versions["ruff"] != "0.5.0" {
		t.Fatalf("pipx versions = %#v, err=%v", versions, err)
	}

	installFakeBinary(t, "pixi", `
if [ "$1" = "global" ] && [ "$2" = "list" ]; then
  echo "Package Version"
  echo "ruff 0.5.0"
fi`)
	versions, err = Pixi{}.ListInstalledVersions()
	if err != nil || versions["ruff"] != "0.5.0" {
		t.Fatalf("pixi versions = %#v, err=%v", versions, err)
	}

	installFakeBinary(t, "poetry", `
if [ "$1" = "self" ] && [ "$2" = "show" ]; then
  echo "poetry-plugin-export (1.8.0)"
fi`)
	versions, err = Poetry{}.ListInstalledVersions()
	if err != nil || versions["poetry-plugin-export"] == "" {
		t.Fatalf("poetry versions = %#v, err=%v", versions, err)
	}

	installFakeBinary(t, "dotnet", `
if [ "$1" = "tool" ] && [ "$2" = "list" ]; then
  echo "Package Id      Version      Commands"
  echo "------------------------------------"
  echo "dotnet-ef       8.0.0        dotnet-ef"
fi`)
	versions, err = DotnetTool{}.ListInstalledVersions()
	if err != nil || versions["dotnet-ef"] != "8.0.0" {
		t.Fatalf("dotnet versions = %#v, err=%v", versions, err)
	}

	installFakeBinary(t, "composer", `
if [ "$1" = "global" ] && [ "$2" = "show" ]; then
  echo '{"installed":[{"name":"phpunit/phpunit","version":"v11.0.0"}]}'
fi`)
	versions, err = Composer{}.ListInstalledVersions()
	if err != nil || versions["phpunit/phpunit"] != "11.0.0" {
		t.Fatalf("composer versions = %#v, err=%v", versions, err)
	}
}
