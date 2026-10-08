//go:build !windows

package compose

import (
	"os"
	"syscall"
)

// openNoFollow opens path for reading and fails if the final path component is a
// symlink. Closing the gap between an Lstat check and a later ReadFile matters
// because module documents come from a cloned repository: without this, a
// concurrent swap could redirect a "safe" read outside the spec root after the
// check already passed.
func openNoFollow(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
}
