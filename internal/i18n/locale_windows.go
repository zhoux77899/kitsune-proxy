//go:build windows

package i18n

import (
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

func detectLocale() string {
	kernel32 := windows.NewLazySystemDLL("kernel32.dll")
	procedure := kernel32.NewProc("GetUserDefaultLocaleName")
	buffer := make([]uint16, 85)
	result, _, _ := procedure.Call(
		uintptr(unsafe.Pointer(&buffer[0])),
		uintptr(len(buffer)),
	)
	if result == 0 {
		return ""
	}
	return syscall.UTF16ToString(buffer)
}
