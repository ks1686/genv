package adapter

// listJSOutdated compares installed name->version against npm registry latest.
// Missing installs are skipped. Transport errors flag the package conservatively
// (map value = installed version). 404/empty latest => up to date.
func listJSOutdated(installed map[string]string, pkgNames []string) (map[string]string, error) {
	names := pkgNames
	if len(names) == 0 {
		names = make([]string, 0, len(installed))
		for n := range installed {
			names = append(names, n)
		}
	}
	outdated := make(map[string]string)
	for _, raw := range names {
		base := jsBasePackageName(raw)
		cur, ok := installed[base]
		if !ok {
			continue
		}
		latest, err := npmLatestVersion(base)
		if err != nil {
			outdated[base] = cur
			continue
		}
		if latest != "" && latest != cur {
			outdated[base] = latest
		}
	}
	if len(outdated) == 0 {
		return nil, nil
	}
	return outdated, nil
}
