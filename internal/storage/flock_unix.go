//go:build !windows

package storage

import (
	"fmt"
	"os"
	"syscall"
)

type osLockHandle struct {
	file *os.File
}

func acquireOSLock(path string) (osLockHandle, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return osLockHandle{}, fmt.Errorf("failed to open lock file %s: %w", path, err)
	}

	fd := int(file.Fd())
	if err := syscall.Flock(fd, syscall.LOCK_EX); err != nil {
		_ = file.Close()
		return osLockHandle{}, fmt.Errorf("failed to acquire file lock: %w", err)
	}

	return osLockHandle{file: file}, nil
}

func releaseOSLock(h osLockHandle) error {
	if h.file == nil {
		return nil
	}
	fd := int(h.file.Fd())
	_ = syscall.Flock(fd, syscall.LOCK_UN)
	return h.file.Close()
}
