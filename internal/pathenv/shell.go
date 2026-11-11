package pathenv

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// RCFile returns the rc file to edit for a login shell: ~/.zshrc (or
// $ZDOTDIR/.zshrc) for zsh; ~/.bash_profile on macOS and ~/.bashrc on Linux
// for bash; ~/.profile for anything else.
func RCFile(shell, home, zdotdir, goos string) string {
	switch filepath.Base(shell) {
	case "zsh":
		if zdotdir != "" {
			return filepath.Join(zdotdir, ".zshrc")
		}
		return filepath.Join(home, ".zshrc")
	case "bash":
		if goos == "darwin" {
			return filepath.Join(home, ".bash_profile")
		}
		return filepath.Join(home, ".bashrc")
	default:
		return filepath.Join(home, ".profile")
	}
}

// DefaultShell is the shell to use when $SHELL is empty.
func DefaultShell(goos string) string {
	if goos == "darwin" {
		return "/bin/zsh"
	}
	return "/bin/bash"
}

// freshPath is the PATH that macOS gives a new Terminal window before any
// startup file runs.
const freshPath = "/usr/bin:/bin:/usr/sbin:/sbin"

const (
	beginOut = "__KUBE_ONRAMP_BEGIN__"
	endOut   = "__KUBE_ONRAMP_END__"
)

// NewShellLookup starts the user's shell the way a new terminal does
// (login + interactive) and asks it for its PATH and for where each name
// resolves. Markers around the output keep noisy rc files from breaking the
// parsing.
func NewShellLookup(ctx context.Context, shell string, names []string) (dirs []string, found map[string]string, err error) {
	var script strings.Builder
	script.WriteString("printf '%s\\n' " + beginOut + "; printf 'PATH=%s\\n' \"$PATH\"; ")
	for _, n := range names {
		script.WriteString("printf '%s=%s\\n' '" + n + "' \"$(command -v '" + n + "' 2>/dev/null)\"; ")
	}
	script.WriteString("printf '%s\\n' " + endOut)

	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, shell, "-l", "-i", "-c", script.String())
	cmd.Env = withEnv(os.Environ(), "PATH", freshPath)
	var out bytes.Buffer
	cmd.Stdout = &out
	runErr := cmd.Run()
	path, found, ok := ParseShellOutput(out.String(), names)
	if !ok {
		if runErr == nil {
			runErr = errors.New("the shell printed no result")
		}
		return nil, nil, runErr
	}
	return SplitList(path, "darwin"), found, nil
}

// ParseShellOutput reads the text between the markers.
func ParseShellOutput(out string, names []string) (path string, found map[string]string, ok bool) {
	begin := strings.LastIndex(out, beginOut+"\n")
	if begin < 0 {
		return "", nil, false
	}
	body := out[begin+len(beginOut)+1:]
	end := strings.Index(body, endOut)
	if end < 0 {
		return "", nil, false
	}
	found = map[string]string{}
	for _, line := range strings.Split(body[:end], "\n") {
		k, v, cut := strings.Cut(strings.TrimRight(line, "\r"), "=")
		if !cut {
			continue
		}
		if k == "PATH" {
			path = v
			continue
		}
		for _, n := range names {
			if k == n && v != "" {
				found[n] = v
			}
		}
	}
	return path, found, true
}

func withEnv(env []string, key, value string) []string {
	out := make([]string, 0, len(env)+1)
	for _, e := range env {
		if !strings.HasPrefix(e, key+"=") {
			out = append(out, e)
		}
	}
	return append(out, key+"="+value)
}
