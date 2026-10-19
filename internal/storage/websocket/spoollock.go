package websocket

import (
	"errors"
	"os"
	"path/filepath"
)

// spoolLockFile is locked by the process that writes or recovers a spool.
const spoolLockFile = "lock"

// errSpoolLocked means another process (or another backend in this one)
// holds the spool.
var errSpoolLocked = errors.New("spool is in use by another process")

// spoolLock is an exclusive operating system lock on a spool directory. The
// process writing a mission holds it from start to end, and a process
// recovering a mission holds it while it resends. The operating system
// releases it when its holder exits for any reason, so a crashed holder never
// leaves a stale lock behind, and two processes can never both hold it.
type spoolLock struct {
	f *os.File
}

// tryLockSpool takes the lock on dir without waiting.
func tryLockSpool(dir string) (*spoolLock, error) {
	f, err := os.OpenFile(filepath.Join(dir, spoolLockFile), os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, err
	}
	if err := lockFile(f); err != nil {
		_ = f.Close()
		if errors.Is(err, errLockHeld) {
			return nil, errSpoolLocked
		}
		return nil, err
	}
	return &spoolLock{f: f}, nil
}

// release gives the lock up. The lock file stays; it is removed with the
// spool directory.
func (l *spoolLock) release() {
	if l == nil || l.f == nil {
		return
	}
	_ = unlockFile(l.f)
	_ = l.f.Close()
	l.f = nil
}
