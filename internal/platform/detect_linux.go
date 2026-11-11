package platform

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var kernelVersionRe = regexp.MustCompile(`^[0-9]+(\.[0-9]+)*`)

func detect() Info {
	release := readTrim("/proc/sys/kernel/osrelease")
	info := Info{Version: kernelVersionRe.FindString(release)}
	info.InWSL = strings.Contains(strings.ToLower(release), "microsoft") ||
		os.Getenv("WSL_DISTRO_NAME") != "" || os.Getenv("WSL_INTEROP") != ""
	name := "Linux"
	if data, err := os.ReadFile("/etc/os-release"); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			if v, ok := strings.CutPrefix(line, "PRETTY_NAME="); ok {
				name = strings.Trim(v, `"`)
			}
		}
	}
	info.Name = name + " (kernel " + release + ")"
	if info.InWSL {
		info.Name += " in WSL"
	}
	info.Shell = filepath.Base(os.Getenv("SHELL"))
	return info
}

func readTrim(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}
