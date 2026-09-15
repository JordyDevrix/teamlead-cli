//go:build windows

package storage

import (
	"fmt"
	"os"
	"syscall"
	"unsafe"
)

var (
	modkernel32      = syscall.NewLazyDLL("kernel32.dll")
	procLockFileEx   = modkernel32.NewProc("LockFileEx")
	procUnlockFileEx = modkernel32.NewProc("UnlockFileEx")
)

const (
	lockfileExclusiveLock = 0x00000002
)

type osLockHandle struct {
	file *os.File
}

func acquireOSLock(path string) (osLockHandle, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0666)
	if err != nil {
		return osLockHandle{}, fmt.Errorf("failed to open lock file %s: %w", path, err)
	}

	var ol syscall.Overlapped
	// Lock 1 byte exclusively
	r1, _, err := procLockFileEx.Call(
		file.Fd(),
		uintptr(lockfileExclusiveLock),
		0,
		1,
		0,
		uintptr(unsafe.Pointer(&ol)),
	)
	if r1 == 0 {
		_ = file.Close()
		return osLockHandle{}, fmt.Errorf("failed to acquire file lock: %w", err)
	}

	return osLockHandle{file: file}, nil
}

func releaseOSLock(h osLockHandle) error {
	if h.file == nil {
		return nil
	}
	var ol syscall.Overlapped
	_, _, _ = procUnlockFileEx.Call(
		h.file.Fd(),
		0,
		1,
		0,
		uintptr(unsafe.Pointer(&ol)),
	)
	return h.file.Close()
}
