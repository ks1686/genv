package adapter

import (
	"maps"
	"slices"
	"testing"
)

func TestGem_ParseListEntries_whenOutputHasMultipleVersionsAndDefault(t *testing.T) {
	// Given
	out := "rake (13.0.6)\njson (default: 2.3.0, 2.6.1)\nbundler (2.5.4)\nnot a gem line\n"

	// When
	got := parseGemListEntries(out)

	// Then
	want := map[string]string{"rake": "13.0.6", "json": "2.3.0", "bundler": "2.5.4"}
	if !maps.Equal(entriesVersionsFromGem(got), want) {
		t.Errorf("parseGemListEntries = %v, want %v", entriesVersionsFromGem(got), want)
	}
}

func TestGem_PlanCommands_whenGemTracked(t *testing.T) {
	// Given
	a := Gem{}

	// When / Then
	if got, want := a.PlanInstall("rake"), []string{"gem", "install", "rake"}; !slices.Equal(got, want) {
		t.Errorf("PlanInstall = %v, want %v", got, want)
	}
	if got, want := a.PlanUninstall("rake@13.0.6"), []string{"gem", "uninstall", "-x", "-a", "rake"}; !slices.Equal(got, want) {
		t.Errorf("PlanUninstall = %v, want %v", got, want)
	}
	if got, want := a.PlanUpgrade("rake"), []string{"gem", "install", "rake"}; !slices.Equal(got, want) {
		t.Errorf("PlanUpgrade = %v, want %v", got, want)
	}
}

func TestGem_ListInstalledVersions_whenGemOutputsLocalGems(t *testing.T) {
	// Given
	installFakeBinary(t, "gem", `if [ "$1" = "list" ] && [ "$2" = "--local" ]; then
  echo 'rake (13.0.6)'
  echo 'json (default: 2.3.0, 2.6.1)'
fi`)

	// When
	versions, err := Gem{}.ListInstalledVersions()

	// Then
	if err != nil {
		t.Fatalf("Gem.ListInstalledVersions: %v", err)
	}
	if want := map[string]string{"rake": "13.0.6", "json": "2.3.0"}; !maps.Equal(versions, want) {
		t.Errorf("ListInstalledVersions = %v, want %v", versions, want)
	}
}

func TestGem_ListInstalled_whenInstallDirNotWritable_returnsNothing(t *testing.T) {
	// Given: a gem install dir genv cannot manage (e.g. macOS system Ruby).
	installFakeBinary(t, "gem", `if [ "$1" = "list" ] && [ "$2" = "--local" ]; then
  echo 'rake (13.0.6)'
  echo 'nokogiri (1.13.8)'
fi`)
	orig := gemManageable
	gemManageable = func() bool { return false }
	t.Cleanup(func() { gemManageable = orig })

	// When
	got, err := Gem{}.ListInstalled()

	// Then: nothing is reported, so scan won't adopt unmanageable system gems.
	if err != nil {
		t.Fatalf("Gem.ListInstalled: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("ListInstalled = %v, want empty (non-writable install dir)", got)
	}
}

func TestGem_ListInstalled_whenInstallDirWritable_listsGems(t *testing.T) {
	// Given: a writable install dir — gems should be reported normally.
	installFakeBinary(t, "gem", `if [ "$1" = "list" ] && [ "$2" = "--local" ]; then
  echo 'rake (13.0.6)'
  echo 'nokogiri (1.13.8)'
fi`)
	orig := gemManageable
	gemManageable = func() bool { return true }
	t.Cleanup(func() { gemManageable = orig })

	// When
	got, err := Gem{}.ListInstalled()

	// Then
	if err != nil {
		t.Fatalf("Gem.ListInstalled: %v", err)
	}
	if want := []string{"rake", "nokogiri"}; !slices.Equal(got, want) {
		t.Errorf("ListInstalled = %v, want %v", got, want)
	}
}

func TestGem_DirWritable_whenDirMissing_returnsFalse(t *testing.T) {
	if dirWritable(t.TempDir() + "/does-not-exist") {
		t.Error("dirWritable = true for a missing directory, want false")
	}
}

func TestGem_DirWritable_whenTempDir_returnsTrue(t *testing.T) {
	if !dirWritable(t.TempDir()) {
		t.Error("dirWritable = false for a writable temp directory, want true")
	}
}

func TestComposer_ParseShowJSON_whenVendorPackagesInstalled(t *testing.T) {
	// Given
	data := []byte(`{"installed":[{"name":"laravel/installer","version":"5.7.0"},{"name":"friendsofphp/php-cs-fixer","version":"v3.52.1"}]}`)

	// When
	got, err := parseComposerShowJSON(data)

	// Then
	if err != nil {
		t.Fatalf("parseComposerShowJSON: %v", err)
	}
	want := map[string]string{"laravel/installer": "5.7.0", "friendsofphp/php-cs-fixer": "3.52.1"}
	if !maps.Equal(entriesVersionsFromComposer(got), want) {
		t.Errorf("parseComposerShowJSON = %v, want %v", entriesVersionsFromComposer(got), want)
	}
}

func TestComposer_PlanCommands_whenVendorPackageHasConstraint(t *testing.T) {
	// Given
	a := Composer{}

	// When / Then: vendor/package prefix must be preserved; only the ":constraint" is stripped.
	if got, want := a.PlanInstall("laravel/installer:^5.0"), []string{"composer", "global", "require", "laravel/installer:^5.0"}; !slices.Equal(got, want) {
		t.Errorf("PlanInstall = %v, want %v", got, want)
	}
	if got, want := a.PlanUninstall("laravel/installer:^5.0"), []string{"composer", "global", "remove", "laravel/installer"}; !slices.Equal(got, want) {
		t.Errorf("PlanUninstall = %v, want %v", got, want)
	}
	if got, want := a.PlanUpgrade("laravel/installer"), []string{"composer", "global", "require", "laravel/installer"}; !slices.Equal(got, want) {
		t.Errorf("PlanUpgrade = %v, want %v", got, want)
	}
}

func TestDotnetTool_ParseList_whenGlobalToolsInstalled(t *testing.T) {
	// Given
	out := "Package Id      Version      Commands\n--------------------------------------\ndotnet-ef       8.0.0        dotnet-ef\nCsvHelper.Cli   2.1.0        csvhelper\n"

	// When
	got := parseDotnetToolList(out)

	// Then: NuGet ids are compared case-insensitively, so names are lowercased.
	want := map[string]string{"dotnet-ef": "8.0.0", "csvhelper.cli": "2.1.0"}
	if !maps.Equal(entriesVersionsFromDotnet(got), want) {
		t.Errorf("parseDotnetToolList = %v, want %v", entriesVersionsFromDotnet(got), want)
	}
}

func TestDotnetTool_PlanCommands_whenToolTracked(t *testing.T) {
	// Given
	a := DotnetTool{}

	// When / Then: uninstall/update must always be --global.
	if got, want := a.PlanInstall("dotnet-ef"), []string{"dotnet", "tool", "install", "--global", "dotnet-ef"}; !slices.Equal(got, want) {
		t.Errorf("PlanInstall = %v, want %v", got, want)
	}
	if got, want := a.PlanUninstall("dotnet-ef@8.0.0"), []string{"dotnet", "tool", "uninstall", "--global", "dotnet-ef"}; !slices.Equal(got, want) {
		t.Errorf("PlanUninstall = %v, want %v", got, want)
	}
	if got, want := a.PlanUpgrade("dotnet-ef"), []string{"dotnet", "tool", "update", "--global", "dotnet-ef"}; !slices.Equal(got, want) {
		t.Errorf("PlanUpgrade = %v, want %v", got, want)
	}
}

func entriesVersionsFromGem(entries []gemEntry) map[string]string {
	m := make(map[string]string, len(entries))
	for _, e := range entries {
		m[e.name] = e.version
	}
	return m
}

func entriesVersionsFromComposer(entries []composerEntry) map[string]string {
	m := make(map[string]string, len(entries))
	for _, e := range entries {
		m[e.name] = e.version
	}
	return m
}

func entriesVersionsFromDotnet(entries []dotnetEntry) map[string]string {
	m := make(map[string]string, len(entries))
	for _, e := range entries {
		m[e.name] = e.version
	}
	return m
}
