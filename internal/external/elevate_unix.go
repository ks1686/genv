//go:build unix

package external

import "golang.org/x/sys/unix"

func dirWritable(path string) bool {
	return unix.Access(path, unix.W_OK) == nil
}
