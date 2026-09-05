package schema

import (
	"fmt"
	"io/fs"
	"strconv"
)

// ParseFilePerm parses a Unix permission string as 3 or 4 octal digits
// (for example "644" or "0700"). Empty input means unset.
func ParseFilePerm(perm string) (fs.FileMode, error) {
	if perm == "" {
		return 0, nil
	}
	if !validOctalPerm(perm) {
		return 0, fmt.Errorf("invalid perm %q; expected octal string like \"0644\" or \"0700\"", perm)
	}
	n, err := strconv.ParseUint(perm, 8, 32)
	if err != nil {
		return 0, fmt.Errorf("invalid perm %q; expected octal string like \"0644\" or \"0700\"", perm)
	}
	return fs.FileMode(n), nil
}

func validOctalPerm(s string) bool {
	if len(s) < 3 || len(s) > 4 {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '7' {
			return false
		}
	}
	return true
}

func validateFilePerm(perm, field string) []ValidationError {
	if perm == "" {
		return nil
	}
	if _, err := ParseFilePerm(perm); err != nil {
		return []ValidationError{{Field: field + ".perm", Message: err.Error()}}
	}
	return nil
}
