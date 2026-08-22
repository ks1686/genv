//go:build windows

package genvfile

// syncDir is a no-op on Windows: there is no supported handle-level directory
// flush, and NTFS metadata journaling already orders the rename.
func syncDir(string) {}
