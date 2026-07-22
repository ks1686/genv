package adapter

// listRegistryOutdated compares installed package versions against a registry's
// latest versions. Missing installs are skipped. Transport errors flag the
// package conservatively with its installed version; an empty latest version
// means the package is not known by the registry and is treated as up to date.
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
