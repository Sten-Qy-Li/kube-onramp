package report

import (
	"errors"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// The Win32 clipboard API is used instead of clip.exe, which can mangle
// non-ASCII text.
var (
	user32               = windows.NewLazySystemDLL("user32.dll")
	kernel32             = windows.NewLazySystemDLL("kernel32.dll")
	procOpenClipboard    = user32.NewProc("OpenClipboard")
	procCloseClipboard   = user32.NewProc("CloseClipboard")
	procEmptyClipboard   = user32.NewProc("EmptyClipboard")
	procSetClipboardData = user32.NewProc("SetClipboardData")
	procGlobalAlloc      = kernel32.NewProc("GlobalAlloc")
	procGlobalFree       = kernel32.NewProc("GlobalFree")
	procGlobalLock       = kernel32.NewProc("GlobalLock")
	procGlobalUnlock     = kernel32.NewProc("GlobalUnlock")
	procRtlMoveMemory    = kernel32.NewProc("RtlMoveMemory")
)

const (
	cfUnicodeText = 13
	gmemMoveable  = 0x0002
)

// CopyToClipboard puts text on the Windows clipboard as Unicode text.
func CopyToClipboard(text string) error {
	data, err := windows.UTF16FromString(text)
	if err != nil {
		return err
	}
	// The clipboard can be busy for a moment (another program holds it).
	var opened bool
	for i := 0; i < 10; i++ {
		if r, _, _ := procOpenClipboard.Call(0); r != 0 {
			opened = true
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !opened {
		return errors.New("the clipboard is busy")
	}
	defer func() { _, _, _ = procCloseClipboard.Call() }()

	if r, _, err := procEmptyClipboard.Call(); r == 0 {
		return err
	}
	size := uintptr(len(data) * 2)
	h, _, err := procGlobalAlloc.Call(gmemMoveable, size)
	if h == 0 {
		return err
	}
	p, _, err := procGlobalLock.Call(h)
	if p == 0 {
		_, _, _ = procGlobalFree.Call(h)
		return err
	}
	_, _, _ = procRtlMoveMemory.Call(p, uintptr(unsafe.Pointer(&data[0])), size)
	_, _, _ = procGlobalUnlock.Call(h)
	if r, _, err := procSetClipboardData.Call(cfUnicodeText, h); r == 0 {
		_, _, _ = procGlobalFree.Call(h)
		return err
	}
	// The system owns h now.
	return nil
}
