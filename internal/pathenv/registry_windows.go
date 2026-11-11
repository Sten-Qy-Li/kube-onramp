package pathenv

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// TestKeyEnv points PATH reads and writes at HKCU\Software\<value> instead
// of HKCU\Environment, so local tests never touch the real PATH (D19).
const TestKeyEnv = "KUBE_ONRAMP_TEST_ENV_KEY"

// UserPath is the user PATH value from the registry.
type UserPath struct {
	Value  string
	Expand bool // REG_EXPAND_SZ (true) or REG_SZ (false)
	Exists bool
}

// UsingTestKey reports whether the test registry key is used.
func UsingTestKey() bool { return os.Getenv(TestKeyEnv) != "" }

func userEnvKey() string {
	if k := os.Getenv(TestKeyEnv); k != "" {
		return `Software\` + k
	}
	return "Environment"
}

// ReadUserPath reads HKCU\Environment\Path without expanding it.
func ReadUserPath() (UserPath, error) {
	k, err := registry.OpenKey(registry.CURRENT_USER, userEnvKey(), registry.QUERY_VALUE)
	if errors.Is(err, fs.ErrNotExist) || errors.Is(err, registry.ErrNotExist) {
		return UserPath{Expand: true}, nil
	}
	if err != nil {
		return UserPath{}, err
	}
	defer func() { _ = k.Close() }()
	v, typ, err := k.GetStringValue("Path")
	if errors.Is(err, registry.ErrNotExist) {
		return UserPath{Expand: true}, nil
	}
	if err != nil {
		return UserPath{}, fmt.Errorf("reading the user PATH: %w", err)
	}
	return UserPath{Value: v, Expand: typ == registry.EXPAND_SZ, Exists: true}, nil
}

// WriteUserPath writes the user PATH and keeps its value type. It never
// shortens the value (unlike setx, which cuts it at 1024 characters).
func WriteUserPath(p UserPath) error {
	k, _, err := registry.CreateKey(registry.CURRENT_USER, userEnvKey(), registry.SET_VALUE|registry.QUERY_VALUE)
	if err != nil {
		return err
	}
	defer func() { _ = k.Close() }()
	if p.Expand {
		return k.SetExpandStringValue("Path", p.Value)
	}
	return k.SetStringValue("Path", p.Value)
}

// DeleteUserPath removes the user PATH value (used by reset when
// kube-onramp created it).
func DeleteUserPath() error {
	k, err := registry.OpenKey(registry.CURRENT_USER, userEnvKey(), registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer func() { _ = k.Close() }()
	return k.DeleteValue("Path")
}

// SystemPath reads the machine PATH (not expanded).
func SystemPath() (string, error) {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, `SYSTEM\CurrentControlSet\Control\Session Manager\Environment`, registry.QUERY_VALUE)
	if err != nil {
		return "", err
	}
	defer func() { _ = k.Close() }()
	v, _, err := k.GetStringValue("Path")
	return v, err
}

// Expand expands %VARIABLES% the way Windows does for REG_EXPAND_SZ.
func Expand(s string) string {
	out, err := registry.ExpandString(s)
	if err != nil {
		return s
	}
	return out
}

// NewShellDirs returns the PATH that a newly opened terminal gets: the
// system PATH followed by the user PATH, both expanded.
func NewShellDirs() ([]string, error) {
	sys, err := SystemPath()
	if err != nil {
		return nil, err
	}
	user, err := ReadUserPath()
	if err != nil {
		return nil, err
	}
	value := Expand(sys)
	if user.Value != "" {
		value += ";" + Expand(user.Value)
	}
	return SplitWindows(value), nil
}

var (
	user32                  = windows.NewLazySystemDLL("user32.dll")
	procSendMessageTimeoutW = user32.NewProc("SendMessageTimeoutW")
)

// Broadcast tells running programs (Explorer, new terminals started from it)
// that the environment changed. Skipped when the test key is used.
func Broadcast() {
	if UsingTestKey() {
		return
	}
	const (
		hwndBroadcast   = 0xFFFF
		wmSettingChange = 0x001A
		smtoAbortIfHung = 0x0002
	)
	env, err := windows.UTF16PtrFromString("Environment")
	if err != nil {
		return
	}
	var result uintptr
	_, _, _ = procSendMessageTimeoutW.Call(hwndBroadcast, wmSettingChange, 0,
		uintptr(unsafe.Pointer(env)), smtoAbortIfHung, 5000, uintptr(unsafe.Pointer(&result)))
}
