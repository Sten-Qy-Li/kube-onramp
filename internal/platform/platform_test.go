package platform

import (
	"errors"
	"path/filepath"
	"runtime"
	"testing"
)

func TestWindowsName(t *testing.T) {
	tests := []struct {
		product, display string
		build            int
		arch             string
		want             string
	}{
		// ProductName still says "Windows 10" on Windows 11 machines.
		{"Windows 10 Education", "25H2", 26300, "amd64", "Windows 11 Education 25H2 x64 (build 26300)"},
		{"Windows 10 Pro", "22H2", 19045, "amd64", "Windows 10 Pro 22H2 x64 (build 19045)"},
		{"Windows 11 Home", "24H2", 26100, "amd64", "Windows 11 Home 24H2 x64 (build 26100)"},
		{"Windows 10 Pro", "", 22000, "amd64", "Windows 11 Pro x64 (build 22000)"},
		{"Windows 10 Pro", "21H2", 21999, "amd64", "Windows 10 Pro 21H2 x64 (build 21999)"},
		{"", "", 26100, "arm64", "Windows 11 ARM64 (build 26100)"},
	}
	for _, tt := range tests {
		if got := WindowsName(tt.product, tt.display, tt.build, tt.arch); got != tt.want {
			t.Errorf("WindowsName(%q, %q, %d) = %q, want %q", tt.product, tt.display, tt.build, got, tt.want)
		}
	}
}

func TestCheckSupported(t *testing.T) {
	tests := []struct {
		name string
		info Info
		want string // "" means supported
	}{
		{"windows 10 22H2", Info{GOOS: "windows", GOARCH: "amd64", Build: 19045}, ""},
		{"windows 11", Info{GOOS: "windows", GOARCH: "amd64", Build: 26300}, ""},
		{"windows 10 21H2", Info{GOOS: "windows", GOARCH: "amd64", Build: 19044}, "E-PRE-OS"},
		{"windows arm64", Info{GOOS: "windows", GOARCH: "arm64", Build: 26100}, "E-PRE-OS"},
		{"macOS 13 Intel", Info{GOOS: "darwin", GOARCH: "amd64", Version: "13.7.8"}, ""},
		{"macOS 15 arm", Info{GOOS: "darwin", GOARCH: "arm64", Version: "15.1"}, ""},
		{"macOS 12", Info{GOOS: "darwin", GOARCH: "amd64", Version: "12.7.6"}, "E-PRE-OS"},
		{"macOS unknown version", Info{GOOS: "darwin", GOARCH: "amd64"}, "E-PRE-OS"},
		{"linux", Info{GOOS: "linux", GOARCH: "amd64", Version: "6.8.0"}, ""},
		{"wsl", Info{GOOS: "linux", GOARCH: "amd64", InWSL: true}, "E-PRE-WSL"},
	}
	for _, tt := range tests {
		p := CheckSupported(tt.info)
		got := ""
		if p != nil {
			got = p.Code
		}
		if got != tt.want {
			t.Errorf("%s: CheckSupported = %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestDetectThisMachine(t *testing.T) {
	info := Detect()
	if info.Name == "" || info.Version == "" {
		t.Errorf("Detect() = %+v", info)
	}
	if runtime.GOOS == "windows" && info.Build < 10240 {
		t.Errorf("Windows build = %d", info.Build)
	}
}

func TestLock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "lock")
	l1, err := AcquireLock(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := AcquireLock(path); !errors.Is(err, ErrLocked) {
		t.Fatalf("second AcquireLock = %v, want ErrLocked", err)
	}
	l1.Release()
	l2, err := AcquireLock(path)
	if err != nil {
		t.Fatalf("AcquireLock after Release = %v", err)
	}
	l2.Release()
}

func TestFreeDiskBytes(t *testing.T) {
	n, err := FreeDiskBytes(filepath.Join(t.TempDir(), "does", "not", "exist"))
	if err != nil || n <= 0 {
		t.Errorf("FreeDiskBytes = %d, %v", n, err)
	}
}
