// Package platform detects the operating system and handles OS details:
// support checks, WSL, the console, disk space, the run lock and ports.
package platform

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/Sten-Qy-Li/kube-onramp/internal/catalog"
	"github.com/Sten-Qy-Li/kube-onramp/internal/problem"
)

// Info describes the machine.
type Info struct {
	GOOS, GOARCH string
	// Version is a dotted version for catalog minOS checks: the Windows NT
	// version ("10.0.26300"), the macOS version ("13.7.8") or the Linux kernel.
	Version string
	// Build is the Windows build number (0 elsewhere).
	Build int
	// Name is for people and reports: "Windows 11 Education 25H2 x64 (build 26300)".
	Name string
	// Shell is for the error report: "PowerShell 5.1", "zsh".
	Shell string
	InWSL bool
}

// Minimum versions in the support matrix (docs/SPEC.md, section 4).
const (
	MinWindowsBuild = 19045 // Windows 10 22H2
	MinMacOS        = "13.0"
)

// Detect returns information about this machine.
func Detect() Info {
	info := detect()
	info.GOOS, info.GOARCH = runtime.GOOS, runtime.GOARCH
	return info
}

// CheckSupported returns E-PRE-WSL or E-PRE-OS if this machine is not
// supported, else nil.
func CheckSupported(info Info) *problem.Problem {
	const supported = "Windows 10 22H2 or newer (64-bit), macOS 13 or newer"
	switch {
	case info.InWSL:
		return problem.PreWSL()
	case info.GOOS == "windows" && info.GOARCH == "amd64":
		if info.Build >= MinWindowsBuild {
			return nil
		}
	case info.GOOS == "darwin" && (info.GOARCH == "amd64" || info.GOARCH == "arm64"):
		if c, err := catalog.CompareVersions(info.Version, MinMacOS); err == nil && c >= 0 {
			return nil
		}
	case info.GOOS == "linux" && info.GOARCH == "amd64":
		return nil // best effort, used by CI
	}
	return problem.PreOS(info.Name, supported)
}

// WindowsName builds a correct Windows name. The registry ProductName can
// still say "Windows 10" on Windows 11, so the family comes from the build
// number (22000 and higher is Windows 11) and only the edition from
// ProductName (docs/DECISIONS.md, D15).
func WindowsName(productName, displayVersion string, build int, goarch string) string {
	family := "Windows 10"
	if build >= 22000 {
		family = "Windows 11"
	}
	edition := strings.TrimSpace(productName)
	for _, prefix := range []string{"Windows 11", "Windows 10", "Windows"} {
		if strings.HasPrefix(edition, prefix) {
			edition = strings.TrimSpace(strings.TrimPrefix(edition, prefix))
			break
		}
	}
	parts := []string{family}
	for _, p := range []string{edition, displayVersion, archName(goarch)} {
		if p != "" {
			parts = append(parts, p)
		}
	}
	return fmt.Sprintf("%s (build %d)", strings.Join(parts, " "), build)
}

func archName(goarch string) string {
	switch goarch {
	case "amd64":
		return "x64"
	case "arm64":
		return "ARM64"
	}
	return goarch
}

// Layout holds kube-onramp's folders and files.
type Layout struct {
	Home, Bin, Logs, Backups, Cache, OIDCCache, Tmp string
	StateFile, LockFile                             string
}

// NewLayout uses KUBE_ONRAMP_HOME if set, else %LOCALAPPDATA%\kube-onramp
// on Windows and ~/.kube-onramp elsewhere.
func NewLayout() (Layout, error) {
	home := os.Getenv("KUBE_ONRAMP_HOME")
	if home == "" {
		var base string
		var err error
		if runtime.GOOS == "windows" {
			base, err = os.UserCacheDir() // %LOCALAPPDATA%
			if err != nil {
				return Layout{}, err
			}
			home = filepath.Join(base, "kube-onramp")
		} else {
			base, err = os.UserHomeDir()
			if err != nil {
				return Layout{}, err
			}
			home = filepath.Join(base, ".kube-onramp")
		}
	}
	abs, err := filepath.Abs(home)
	if err != nil {
		return Layout{}, err
	}
	return LayoutAt(abs), nil
}

// LayoutAt returns the layout for a home folder.
func LayoutAt(home string) Layout {
	return Layout{
		Home:      home,
		Bin:       filepath.Join(home, "bin"),
		Logs:      filepath.Join(home, "logs"),
		Backups:   filepath.Join(home, "backups"),
		Cache:     filepath.Join(home, "cache"),
		OIDCCache: filepath.Join(home, "cache", "oidc-login"),
		Tmp:       filepath.Join(home, "tmp"),
		StateFile: filepath.Join(home, "state.json"),
		LockFile:  filepath.Join(home, "lock"),
	}
}

// ExeName adds ".exe" on Windows.
func ExeName(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}
	return name
}

// LoginPorts are the addresses kubelogin tries, in order.
var LoginPorts = []string{"127.0.0.1:8000", "127.0.0.1:18000"}

// PortFree reports whether a TCP address can be bound right now.
func PortFree(addr string) bool {
	l, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", addr)
	if err != nil {
		return false
	}
	_ = l.Close()
	return true
}

// FreeDiskBytes returns the free space for the folder (or its nearest
// existing parent).
func FreeDiskBytes(dir string) (int64, error) {
	for {
		if _, err := os.Stat(dir); err == nil {
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return freeDiskBytes(dir)
}
