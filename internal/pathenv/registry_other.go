//go:build !windows

package pathenv

import "errors"

// TestKeyEnv is only used on Windows.
const TestKeyEnv = "KUBE_ONRAMP_TEST_ENV_KEY"

var errNotWindows = errors.New("the Windows registry is only available on Windows")

// UserPath is the user PATH value from the Windows registry.
type UserPath struct {
	Value  string
	Expand bool
	Exists bool
}

// UsingTestKey is only meaningful on Windows.
func UsingTestKey() bool { return false }

// ReadUserPath is only available on Windows.
func ReadUserPath() (UserPath, error) { return UserPath{}, errNotWindows }

// WriteUserPath is only available on Windows.
func WriteUserPath(UserPath) error { return errNotWindows }

// DeleteUserPath is only available on Windows.
func DeleteUserPath() error { return errNotWindows }

// NewShellDirs is only available on Windows.
func NewShellDirs() ([]string, error) { return nil, errNotWindows }

// Expand returns s unchanged outside Windows.
func Expand(s string) string { return s }

// Broadcast does nothing outside Windows.
func Broadcast() {}
