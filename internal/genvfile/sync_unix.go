//go:build unix

package genvfile

import "os"

// syncFile flushes a written temp file to stable storage before the rename
// that publishes it. Without this, a power loss can journal the rename before
// the data blocks land, leaving an empty or partial spec/lock behind.
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

// syncDir fsyncs the directory so the rename itself is durable. Best effort:
// some filesystems refuse directory fsync, and a failed durability hint must
// never fail the write that already succeeded.
func syncDir(dir string) {
	if d, err := os.Open(dir); err == nil {
		_ = d.Sync()
		_ = d.Close()
	}
}
