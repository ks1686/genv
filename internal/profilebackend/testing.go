package profilebackend

import "os/exec"

// SetLookPathForTest replaces LookPath used by DetectEngine. The returned
// restore function puts the previous implementation back.
func SetLookPathForTest(fn func(file string) (string, error)) (restore func()) {
	prev := lookPath
	if fn == nil {
		lookPath = exec.LookPath
	} else {
		lookPath = fn
	}
	return func() { lookPath = prev }
}
