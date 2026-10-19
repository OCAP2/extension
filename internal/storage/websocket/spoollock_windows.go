//go:build windows

package websocket

import (
	"errors"
	"os"

	"golang.org/x/sys/windows"
)

var errLockHeld = windows.ERROR_LOCK_VIOLATION

// lockFile takes an exclusive lock on the first byte of f without blocking.
// Windows locks belong to the handle, so two opens conflict even within one
// process.
func lockFile(f *os.File) error {
	var ol windows.Overlapped
	err := windows.LockFileEx(windows.Handle(f.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, &ol)
	if errors.Is(err, windows.ERROR_LOCK_VIOLATION) || errors.Is(err, windows.ERROR_IO_PENDING) {
		return errLockHeld
	}
	return err
}

func unlockFile(f *os.File) error {
	var ol windows.Overlapped
	return windows.UnlockFileEx(windows.Handle(f.Fd()), 0, 1, 0, &ol)
}
