package adapter

import (
	"encoding/json"
	"strings"
)

type pythonEntry struct {
	name    string
	version string
}

func parsePipxListJSON(data []byte) ([]pythonEntry, error) {
	var root struct {
		Venvs map[string]struct {
			Metadata struct {
				MainPackage struct {
					Package        string `json:"package"`
					PackageOrURL   string `json:"package_or_url"`
					PackageVersion string `json:"package_version"`
				} `json:"main_package"`
			} `json:"metadata"`
		} `json:"venvs"`
	}
	if err := json.Unmarshal(data, &root); err != nil {
		return nil, err
	}
	var entries []pythonEntry
	for _, venv := range root.Venvs {
		pkg := venv.Metadata.MainPackage.Package
		if pkg == "" {
			pkg = venv.Metadata.MainPackage.PackageOrURL
		}
		if pkg != "" {
			entries = append(entries, pythonEntry{
				name:    pkg,
				version: venv.Metadata.MainPackage.PackageVersion,
			})
		}
	}
	return entries, nil
}

func parsePipListJSON(data []byte) ([]pythonEntry, error) {
	var list []struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	}
	if err := json.Unmarshal(data, &list); err != nil {
		return nil, err
	}
	var entries []pythonEntry
	for _, item := range list {
		if item.Name != "" {
			entries = append(entries, pythonEntry{
				name:    item.Name,
				version: item.Version,
			})
		}
	}
	return entries, nil
}

func parseCondaListJSON(data []byte) ([]pythonEntry, error) {
	var list []struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	}
	if err := json.Unmarshal(data, &list); err != nil {
		return nil, err
	}
	var entries []pythonEntry
	for _, item := range list {
		if item.Name != "" {
			entries = append(entries, pythonEntry{
				name:    item.Name,
				version: item.Version,
			})
		}
	}
	return entries, nil
}

func parsePixiListText(out string) ([]pythonEntry, error) {
	var entries []pythonEntry
	lines := nonEmptyLines(out)
	for _, line := range lines {
		if isIndented(line) {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) >= 2 {
			if strings.ToLower(fields[0]) == "package" && strings.ToLower(fields[1]) == "version" {
				continue
			}
			entries = append(entries, pythonEntry{name: fields[0], version: fields[1]})
		}
	}
	return entries, nil
}

func parsePoetryPluginsText(out string) ([]pythonEntry, error) {
	var entries []pythonEntry
	for line := range strings.SplitSeq(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, " ") || strings.HasPrefix(line, "•") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) >= 2 {
			entries = append(entries, pythonEntry{
				name:    fields[0],
				version: strings.Trim(fields[1], "()"),
			})
		}
	}
	return entries, nil
}
