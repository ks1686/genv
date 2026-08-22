//go:build unix

package genvfile

import "os"

// syncDir fsyncs the directory after the publishing rename so the rename
// itself is durable. Best effort: some filesystems refuse directory fsync,
// and a failed durability hint must never fail the write that already
// succeeded.
func syncDir(dir string) {
	if d, err := os.Open(dir); err == nil {
		_ = d.Sync()
		_ = d.Close()
	}
}
