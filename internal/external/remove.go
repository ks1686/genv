package external

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/ks1686/genv/internal/genvfile"
)

// Remove deletes only unchanged paths owned by a genv external receipt.
func Remove(receipt *genvfile.ExternalReceipt) error {
	if receipt == nil || !receipt.Owned {
		return fmt.Errorf("external installation is not owned by genv")
	}
	for _, pathReceipt := range receipt.Paths {
		digest, err := fileSHA256(pathReceipt.Path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return fmt.Errorf("inspect %s: %w", pathReceipt.Path, err)
		}
		if err := VerifySHA256(digest, pathReceipt.SHA256); err != nil {
			return fmt.Errorf("refusing to remove modified external path %s", pathReceipt.Path)
		}
	}
	type stagedRemoval struct {
		original string
		backup   string
		dir      string
	}
	var backups []stagedRemoval
	restore := func() {
		for i := len(backups) - 1; i >= 0; i-- {
			_ = os.Rename(backups[i].backup, backups[i].original)
			_ = os.Remove(backups[i].dir)
		}
	}
	for _, pathReceipt := range receipt.Paths {
		if _, err := os.Lstat(pathReceipt.Path); os.IsNotExist(err) {
			continue
		}
		backupDir, err := os.MkdirTemp(filepath.Dir(pathReceipt.Path), ".genv-remove-*")
		if err != nil {
			restore()
			return fmt.Errorf("stage removal of %s: %w", pathReceipt.Path, err)
		}
		backup := filepath.Join(backupDir, filepath.Base(pathReceipt.Path))
		if err := os.Rename(pathReceipt.Path, backup); err != nil {
			_ = os.Remove(backupDir)
			restore()
			return fmt.Errorf("stage removal of %s: %w", pathReceipt.Path, err)
		}
		backups = append(backups, stagedRemoval{original: pathReceipt.Path, backup: backup, dir: backupDir})
	}
	for _, staged := range backups {
		if err := os.Remove(staged.backup); err != nil {
			restore()
			return fmt.Errorf("remove %s: %w", staged.original, err)
		}
		_ = os.Remove(staged.dir)
	}
	return nil
}
