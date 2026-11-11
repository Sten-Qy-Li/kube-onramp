//go:build !windows

package app

import "os"

// removeBinDir deletes the bin folder. On macOS and Linux a running binary
// can be deleted.
func removeBinDir(bin string, _ func(string, ...any)) error {
	return os.RemoveAll(bin)
}
