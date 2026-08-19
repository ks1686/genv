package genvfile

import (
	"errors"
	"fmt"
	"os"
	"time"
)

// RotateBackup renames path to a timestamped sibling. A missing file is not an error.
func RotateBackup(path string) error {
	if path == "" {
		return nil
	}
	backupPath := path + ".bak-" + time.Now().UTC().Format("20060102T150405.000000000Z")
	if err := os.Rename(path, backupPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("backing up %s: %w", path, err)
	}
	return nil
}
