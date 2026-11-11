package platform

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func detect() Info {
	version := run("sw_vers", "-productVersion")
	info := Info{Version: version}
	hw := "Intel"
	if run("sysctl", "-n", "hw.optional.arm64") == "1" {
		hw = "Apple silicon"
	}
	name := "macOS " + version + " (" + hw
	if run("sysctl", "-n", "sysctl.proc_translated") == "1" {
		name += ", running under Rosetta"
	}
	info.Name = name + ")"
	info.Shell = filepath.Base(os.Getenv("SHELL"))
	if info.Shell == "." || info.Shell == "" {
		info.Shell = "unknown shell"
	}
	return info
}

func run(name string, args ...string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, name, args...).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
