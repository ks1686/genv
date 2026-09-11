package external

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/ks1686/genv/internal/schema"
)

func validateInstallScope(install schema.ExternalInstall, home string) error {
	if install.Scope == "system" || install.Type == "script" {
		return nil
	}
	var destinations []string
	if install.Destination != "" {
		destinations = append(destinations, install.Destination)
	}
	for _, file := range install.Files {
		destinations = append(destinations, file.To)
	}
	for _, destination := range destinations {
		if destination == "~" {
			destination = home
		} else if strings.HasPrefix(destination, "~/") || strings.HasPrefix(destination, `~\`) {
			destination = filepath.Join(home, destination[2:])
		}
		absolute, err := filepath.Abs(destination)
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(home, absolute)
		if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return fmt.Errorf("user-scope external destination %q is outside home %q; declare scope system", absolute, home)
		}
	}
	return nil
}
