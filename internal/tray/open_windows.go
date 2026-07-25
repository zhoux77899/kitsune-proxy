//go:build windows

package tray

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

var shellExecute = windows.NewLazySystemDLL("shell32.dll").NewProc("ShellExecuteW")

// OpenPath asks Explorer to open a file or directory.
func OpenPath(path string) error {
	operation, err := windows.UTF16PtrFromString("open")
	if err != nil {
		return err
	}
	target, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	result, _, callErr := shellExecute.Call(
		0,
		uintptr(unsafe.Pointer(operation)),
		uintptr(unsafe.Pointer(target)),
		0,
		0,
		1,
	)
	if result <= 32 {
		return fmt.Errorf("open path: ShellExecuteW result %d: %w", result, callErr)
	}
	return nil
}
