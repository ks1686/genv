package external

import (
	"fmt"
	"regexp"

	"github.com/ks1686/genv/internal/schema"
)

// TemplateValues are the only values available to installer argv and environment templates.
type TemplateValues struct {
	Version     string
	Tag         string
	OS          string
	Arch        string
	Script      string
	Destination string
}

func expandInstallTemplates(install schema.ExternalInstall, values TemplateValues) (schema.ExternalInstall, error) {
	var err error
	if install.Destination != "" {
		install.Destination, err = ExpandInstallTemplate(install.Destination, values)
		if err != nil {
			return install, err
		}
	}
	for i := range install.Files {
		install.Files[i].To, err = ExpandInstallTemplate(install.Files[i].To, values)
		if err != nil {
			return install, err
		}
	}
	return install, nil
}

// ExpandInstallTemplate substitutes only documented, non-executable placeholders.
func ExpandInstallTemplate(template string, values TemplateValues) (string, error) {
	allowed := map[string]string{
		"version": values.Version, "tag": values.Tag, "os": values.OS, "arch": values.Arch,
		"script": values.Script, "destination": values.Destination,
	}
	re := regexp.MustCompile(`\{([A-Za-z][A-Za-z0-9]*)\}`)
	var unknown string
	result := re.ReplaceAllStringFunc(template, func(match string) string {
		name := match[1 : len(match)-1]
		value, ok := allowed[name]
		if !ok {
			unknown = name
			return match
		}
		return value
	})
	if unknown != "" {
		return "", fmt.Errorf("unknown installer placeholder %q", unknown)
	}
	if regexp.MustCompile(`[{}]`).MatchString(result) {
		return "", fmt.Errorf("invalid installer template")
	}
	return result, nil
}
