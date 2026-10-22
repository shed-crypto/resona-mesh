//go:build !windows

package storage

import (
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

// AvailableDiskSpace queries the underlying filesystem for free bytes available to the current user.
func AvailableDiskSpace(dir string) (uint64, error) {
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

	var stat unix.Statfs_t
	if err := unix.Statfs(target, &stat); err != nil {
		return 0, err
	}
	return stat.Bavail * uint64(stat.Bsize), nil
}
