//go:build windows

package resolver

import "golang.org/x/sys/windows"

func processElevated() bool {
	var token windows.Token
	err := windows.OpenProcessToken(windows.CurrentProcess(), windows.TOKEN_QUERY, &token)
	if err != nil {
		return false
	}
	defer token.Close()
	return token.IsElevated()
}

func sudoNoninteractiveOK() bool {
	return false
}
