//go:build windows

package external

import "os"

func dirWritable(path string) bool {
	f, err := os.CreateTemp(path, ".genv-writecheck-*")
	if err != nil {
		return false
	}
	name := f.Name()
	_ = f.Close()
	_ = os.Remove(name)
	return true
}
