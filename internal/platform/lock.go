package platform

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"

	"golang.org/x/term"
)

// ErrLocked means another kube-onramp run holds the lock.
var ErrLocked = errors.New("kube-onramp is already running")

// Lock is held for the whole run so that two windows can't change the same
// files at once. It is an OS file lock: if the process dies, the OS releases
// it, so a stale lock can't block later runs. The PID inside is only for
// people reading the file.
type Lock struct{ f *os.File }

// AcquireLock takes the lock or returns ErrLocked.
func AcquireLock(path string) (*Lock, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, err
	}
	if err := tryLock(f); err != nil {
		_ = f.Close()
		return nil, err
	}
	// Only the lock holder rewrites the file.
	if err := f.Truncate(0); err == nil {
		_, _ = f.WriteAt([]byte(strconv.Itoa(os.Getpid())+"\n"), 0)
	}
	return &Lock{f: f}, nil
}

// Release frees the lock.
func (l *Lock) Release() {
	if l != nil && l.f != nil {
		_ = l.f.Close() // closing releases the OS lock
		l.f = nil
	}
}

// IsTerminal reports whether f is a terminal (or a Windows console).
func IsTerminal(f *os.File) bool { return term.IsTerminal(int(f.Fd())) }
