package files

import (
	"fmt"
	"os"
	"time"
)

func backupExisting(target string) error {
	_, err := backupExistingTo(target)
	return err
}

func backupExistingTo(target string) (string, error) {
	backupPath := backupPathFor(target)
	if err := os.Rename(target, backupPath); err != nil {
		return "", fmt.Errorf("backup %s -> %s: %w", target, backupPath, err)
	}
	return backupPath, nil
}

func backupPathFor(target string) string {
	ts := time.Now().UTC().Format("20060102150405")
	base := target + ".backup." + ts
	if _, err := os.Lstat(base); os.IsNotExist(err) {
		return base
	}
	for i := 1; ; i++ {
		candidate := fmt.Sprintf("%s.%d", base, i)
		if _, err := os.Lstat(candidate); os.IsNotExist(err) {
			return candidate
		}
	}
}
