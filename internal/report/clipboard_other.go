//go:build !windows

package report

import (
	"context"
	"errors"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// CopyToClipboard puts text on the clipboard with pbcopy on macOS.
// Other systems are not supported; the report file is still saved.
func CopyToClipboard(text string) error {
	if runtime.GOOS != "darwin" {
		return errors.New("copying to the clipboard is only supported on Windows and macOS")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "pbcopy")
	cmd.Stdin = strings.NewReader(text)
	return cmd.Run()
}
