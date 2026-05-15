//go:build windows

package common

import (
	"errors"
	"fmt"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows/registry"
)

const windowsUserPathRegistryPath = "Environment"

// windowsUserPathContainsDir reports whether dir is already an element of
// the semicolon-separated userPath, comparing case-insensitively and
// ignoring trailing path separators.
func windowsUserPathContainsDir(userPath, dir string) bool {
	want := strings.ToLower(strings.TrimRight(dir, `\/`))
	for entry := range strings.SplitSeq(userPath, ";") {
		got := strings.ToLower(strings.TrimRight(entry, `\/`))
		if got == want {
			return true
		}
	}
	return false
}

// IsInWindowsUserPath reports whether installDir is present in either the
// current session PATH or the persistent user Path stored in the registry.
// The session check matters when a previous run already updated the
// registry but the broadcast hasn't reached this process.
func IsInWindowsUserPath(installDir string) (bool, error) {
	if installDir == "" {
		return false, nil
	}
	if IsInPath(installDir) {
		return true, nil
	}

	key, err := registry.OpenKey(registry.CURRENT_USER, windowsUserPathRegistryPath, registry.QUERY_VALUE)
	if err != nil {
		return false, fmt.Errorf("failed to open user Environment registry key: %w", err)
	}
	defer key.Close()

	userPath, _, err := key.GetStringValue("Path")
	if errors.Is(err, registry.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("failed to read user Path: %w", err)
	}
	return windowsUserPathContainsDir(userPath, installDir), nil
}

// AddToWindowsUserPath appends installDir to the user's Path environment
// variable in the registry, preserving the existing value's type (REG_SZ
// vs REG_EXPAND_SZ so that entries like %USERPROFILE%\bin remain dynamic).
// Broadcasts WM_SETTINGCHANGE so Explorer and shells launched after this
// call see the new value; the current process's environment is not updated.
func AddToWindowsUserPath(installDir string) error {
	if installDir == "" {
		return errors.New("installDir is empty")
	}

	key, err := registry.OpenKey(registry.CURRENT_USER, windowsUserPathRegistryPath, registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("failed to open user Environment registry key: %w", err)
	}
	defer key.Close()

	userPath, valueType, err := key.GetStringValue("Path")
	if errors.Is(err, registry.ErrNotExist) {
		userPath = ""
		valueType = registry.SZ
	} else if err != nil {
		return fmt.Errorf("failed to read user Path: %w", err)
	}

	if windowsUserPathContainsDir(userPath, installDir) {
		return nil
	}

	var newPath string
	switch {
	case userPath == "":
		newPath = installDir
	case strings.HasSuffix(userPath, ";"):
		newPath = userPath + installDir
	default:
		newPath = userPath + ";" + installDir
	}

	if valueType == registry.EXPAND_SZ {
		if err := key.SetExpandStringValue("Path", newPath); err != nil {
			return fmt.Errorf("failed to write user Path: %w", err)
		}
	} else {
		if err := key.SetStringValue("Path", newPath); err != nil {
			return fmt.Errorf("failed to write user Path: %w", err)
		}
	}

	broadcastEnvironmentChange()
	return nil
}

// broadcastEnvironmentChange sends WM_SETTINGCHANGE with lParam pointing to
// the string "Environment". This is the same notification PowerShell's
// [Environment]::SetEnvironmentVariable emits so Explorer and listening
// processes refresh their environment. Errors are intentionally ignored —
// the registry write has already succeeded and new processes will pick up
// the change regardless.
func broadcastEnvironmentChange() {
	user32 := syscall.NewLazyDLL("user32.dll")
	sendMessageTimeoutW := user32.NewProc("SendMessageTimeoutW")

	const (
		HWND_BROADCAST   = uintptr(0xFFFF)
		WM_SETTINGCHANGE = uintptr(0x001A)
		SMTO_ABORTIFHUNG = uintptr(0x0002)
		timeoutMs        = uintptr(5000)
	)

	envStr, err := syscall.UTF16PtrFromString("Environment")
	if err != nil {
		return
	}

	var result uintptr
	_, _, _ = sendMessageTimeoutW.Call(
		HWND_BROADCAST,
		WM_SETTINGCHANGE,
		0,
		uintptr(unsafe.Pointer(envStr)),
		SMTO_ABORTIFHUNG,
		timeoutMs,
		uintptr(unsafe.Pointer(&result)),
	)
}
