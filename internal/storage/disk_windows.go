//go:build windows

package storage

import (
	"os"
	"path/filepath"
	"syscall"
	"unsafe"
)

// AvailableDiskSpace queries the underlying filesystem for free bytes available to the current user.
func AvailableDiskSpace(dir string) (uint64, error) {
	// If the directory does not exist yet, traverse up until an existing ancestor is found
	target := dir
	for {
		if fi, err := os.Stat(target); err == nil && fi.IsDir() {
			break
		}
		parent := filepath.Dir(target)
		if parent == target {
			break
		}
		target = parent
	}

	dirPtr, err := syscall.UTF16PtrFromString(target)
	if err != nil {
		return 0, err
	}

	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	getDiskFreeSpaceEx := kernel32.NewProc("GetDiskFreeSpaceExW")

	var freeBytesAvailable, totalNumberOfBytes, totalNumberOfFreeBytes int64
	r1, _, callErr := getDiskFreeSpaceEx.Call(
		uintptr(unsafe.Pointer(dirPtr)),
		uintptr(unsafe.Pointer(&freeBytesAvailable)),
		uintptr(unsafe.Pointer(&totalNumberOfBytes)),
		uintptr(unsafe.Pointer(&totalNumberOfFreeBytes)),
	)
	if r1 == 0 {
		return 0, callErr
	}
	if freeBytesAvailable < 0 {
		return 0, nil
	}
	return uint64(freeBytesAvailable), nil
}
