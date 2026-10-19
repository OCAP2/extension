//go:build !windows

package websocket

import (
	"errors"
	"os"
	"syscall"
)

var errLockHeld = syscall.EWOULDBLOCK

// lockFile takes an exclusive flock on f without blocking. flock locks belong
// to the open file, so two opens conflict even within one process.
func lockFile(f *os.File) error {
	err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if errors.Is(err, syscall.EAGAIN) {
		return errLockHeld
	}
	return err
}

func unlockFile(f *os.File) error {
	return syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
}
