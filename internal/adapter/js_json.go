package adapter

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"sort"
)

type jsPackageEntry struct {
	name    string
	version string
}

type jsListPackage struct {
	Version      string                   `json:"version"`
	Dependencies map[string]jsListPackage `json:"dependencies"`
}

func runJSONPackageList(cmd string, args ...string) ([]jsPackageEntry, error) {
	out, err := exec.Command(cmd, args...).Output()
	if err != nil {
		return nil, commandOutputError(err)
	}
	entries, err := parseJSListJSON(out)
	if err != nil {
		return nil, fmt.Errorf("parse %s global package list: %w", cmd, err)
	}
	return entries, nil
}

func parseJSListJSON(data []byte) ([]jsPackageEntry, error) {
	var root jsListPackage
	if err := json.Unmarshal(data, &root); err == nil && root.Dependencies != nil {
		return jsEntriesFromDependencies(root.Dependencies), nil
	}

	var roots []jsListPackage
	if err := json.Unmarshal(data, &roots); err != nil {
		return nil, err
	}
	if len(roots) == 0 || roots[0].Dependencies == nil {
		return nil, nil
	}
	return jsEntriesFromDependencies(roots[0].Dependencies), nil
}

func jsEntriesFromDependencies(deps map[string]jsListPackage) []jsPackageEntry {
	names := make([]string, 0, len(deps))
	for name := range deps {
		names = append(names, name)
	}
	sort.Strings(names)

	entries := make([]jsPackageEntry, 0, len(names))
	for _, name := range names {
		entries = append(entries, jsPackageEntry{name: name, version: deps[name].Version})
	}
	return entries
}

func commandOutputError(err error) error {
	if _, ok := err.(*exec.ExitError); ok {
		return nil
	}
	return err
}

func entriesNames(entries []jsPackageEntry) []string {
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.name)
	}
	return names
}

func entriesVersions(entries []jsPackageEntry) map[string]string {
	versions := make(map[string]string, len(entries))
	for _, entry := range entries {
		versions[entry.name] = entry.version
	}
	return versions
}

func findEntry(entries []jsPackageEntry, pkgName string) (jsPackageEntry, bool) {
	base := jsBasePackageName(pkgName)
	for _, entry := range entries {
		if entry.name == base {
			return entry, true
		}
	}
	return jsPackageEntry{}, false
}
