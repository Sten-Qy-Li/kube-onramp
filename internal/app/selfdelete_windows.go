package app

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"golang.org/x/sys/windows"

	"github.com/Sten-Qy-Li/kube-onramp/internal/inspect"
)

// removeBinDir deletes the bin folder. Windows can't delete a running .exe,
// but it can rename it: the running kube-onramp.exe is moved to %TEMP% and a
// short hidden helper deletes it after this process has exited (D16).
func removeBinDir(bin string, logf func(string, ...any)) error {
	exe, err := inspect.Executable()
	inBin := err == nil && strings.HasPrefix(strings.ToLower(exe), strings.ToLower(filepath.Clean(bin)+`\`))
	if !inBin {
		return os.RemoveAll(bin)
	}
	moved := filepath.Join(os.TempDir(), fmt.Sprintf("kube-onramp-old-%d.exe", os.Getpid()))
	if err := os.Rename(exe, moved); err != nil {
		// %TEMP% is on another drive: delete the rest now, the folder later.
		logf("moving the running exe: %v", err)
		for _, e := range mustReadDir(bin) {
			if p := filepath.Join(bin, e.Name()); !strings.EqualFold(p, exe) {
				_ = os.RemoveAll(p)
			}
		}
		return deleteLater(`rmdir /s /q "` + bin + `"`)
	}
	if err := os.RemoveAll(bin); err != nil {
		return err
	}
	return deleteLater(`del /f /q "` + moved + `"`)
}

// deleteLater runs a hidden cmd.exe that waits about 3 seconds, then runs
// the delete command.
func deleteLater(command string) error {
	// The helper must outlive this process, so it gets no cancelable context.
	cmd := exec.CommandContext(context.Background(), "cmd.exe")
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CmdLine:       `cmd.exe /d /c ping -n 4 127.0.0.1 >nul & ` + command,
		HideWindow:    true,
		CreationFlags: windows.CREATE_NO_WINDOW | windows.CREATE_NEW_PROCESS_GROUP,
	}
	return cmd.Start()
}

func mustReadDir(dir string) []os.DirEntry {
	entries, _ := os.ReadDir(dir)
	return entries
}
