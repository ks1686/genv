package adapter

// listJSOutdated compares installed name->version against the npm registry.
func listJSOutdated(installed map[string]string, pkgNames []string) (map[string]string, error) {
	return listRegistryOutdated(installed, pkgNames, jsBasePackageName, npmLatestVersion)
}
