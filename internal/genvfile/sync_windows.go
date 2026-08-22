//go:build windows

package genvfile

import "os"

// syncFile flushes a written temp file to stable storage before the rename
// that publishes it. Windows FlushFileBuffers is the analogue of fsync; the
// same power-loss window applies to NTFS metadata journaling.
func syncFile(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

// syncDir is a no-op on Windows: there is no supported handle-level directory
// flush, and NTFS metadata journaling already orders the rename.
func syncDir(string) {}
