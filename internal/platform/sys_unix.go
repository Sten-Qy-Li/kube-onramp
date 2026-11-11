//go:build !windows

package platform

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
	"golang.org/x/term"
)

func freeDiskBytes(dir string) (int64, error) {
	var st unix.Statfs_t
	if err := unix.Statfs(dir, &st); err != nil {
		return 0, err
	}
	return int64(st.Bavail) * int64(st.Bsize), nil //nolint:unconvert // field types differ between systems
}

func tryLock(f *os.File) error {
	err := unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB)
	if errors.Is(err, unix.EWOULDBLOCK) {
		return ErrLocked
	}
	return err
}

// EnableColor reports whether f is a terminal that shows colors.
func EnableColor(f *os.File) bool {
	return term.IsTerminal(int(f.Fd())) && os.Getenv("TERM") != "dumb"
}

// OwnsConsole is only meaningful on Windows.
func OwnsConsole() bool { return false }
