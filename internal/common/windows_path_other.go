//go:build !windows

package common

import "errors"

// IsInWindowsUserPath is a Windows-only check against the persistent user
// Path in the registry. On other platforms it always errors — callers
// should branch on runtime.GOOS before calling.
func IsInWindowsUserPath(installDir string) (bool, error) {
	return false, errors.New("IsInWindowsUserPath is only supported on Windows")
}

// AddToWindowsUserPath is a Windows-only writer for the persistent user
// Path in the registry. On other platforms it always errors — callers
// should branch on runtime.GOOS before calling.
func AddToWindowsUserPath(installDir string) error {
	return errors.New("AddToWindowsUserPath is only supported on Windows")
}
