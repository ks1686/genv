//go:build windows

package compose

import "os"

// openNoFollow opens path for reading. Windows has no O_NOFOLLOW equivalent
// that applies here, so the symlink defense on this platform is the
// component-wise Lstat check plus the regular-file check in LoadDocument, both
// of which run before this call.
func openNoFollow(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_RDONLY, 0)
}
