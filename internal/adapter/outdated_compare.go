package adapter

// listRegistryOutdated compares installed package versions against a registry's
// latest versions. Missing installs are skipped. Transport errors flag the
// package conservatively with its installed version; an empty latest version
// means the package is not known by the registry and is treated as up to date.
//
// When pkgNames is empty, every installed name is checked — one registry call
// per name — so callers should prefer passing the tracked name list.
func listRegistryOutdated(
	installed map[string]string,
	pkgNames []string,
	baseName func(string) string,
	latestFn func(string) (string, error),
) (map[string]string, error) {
	names := pkgNames
	if len(names) == 0 {
		names = make([]string, 0, len(installed))
		for name := range installed {
			names = append(names, name)
		}
	}

	outdated := make(map[string]string)
	for _, raw := range names {
		base := baseName(raw)
		current, ok := installed[base]
		if !ok {
			continue
		}

		latest, err := latestFn(base)
		if err != nil {
			outdated[base] = current
			continue
		}
		if latest != "" && latest != current {
			outdated[base] = latest
		}
	}
	if len(outdated) == 0 {
		return nil, nil
	}
	return outdated, nil
}

// listJSOutdated compares installed name->version against the npm registry.
func listJSOutdated(installed map[string]string, pkgNames []string) (map[string]string, error) {
	return listRegistryOutdated(installed, pkgNames, jsBasePackageName, npmLatestVersion)
}

// intersectNameMap limits all to the manager-native package names genv tracks.
// An empty pkgNames means every outdated package should be returned.
func intersectNameMap(all map[string]string, pkgNames []string) map[string]string {
	if len(all) == 0 {
		return nil
	}
	if len(pkgNames) == 0 {
		return all
	}
	want := make(map[string]bool, len(pkgNames))
	for _, name := range pkgNames {
		want[name] = true
	}
	out := make(map[string]string)
	for name, version := range all {
		if want[name] {
			out[name] = version
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// versionMapOf builds a name->version map from typed install-list entries.
func versionMapOf[E any](entries []E, nameVer func(E) (string, string)) map[string]string {
	out := make(map[string]string, len(entries))
	for _, entry := range entries {
		name, version := nameVer(entry)
		out[name] = version
	}
	return out
}
