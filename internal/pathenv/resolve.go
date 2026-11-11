package pathenv

import (
	"os"
	"path/filepath"
	"strings"
)

// DefaultPathExt is used when PATHEXT is not set.
const DefaultPathExt = ".COM;.EXE;.BAT;.CMD"

// FindAll returns every file that a shell on goos would run for name, in
// PATH order. The first one is the one that wins.
func FindAll(dirs []string, name, goos, pathext string) []string {
	var exts []string
	if goos == "windows" {
		if pathext == "" {
			pathext = DefaultPathExt
		}
		if filepath.Ext(name) != "" {
			exts = []string{""}
		}
		for _, e := range strings.Split(pathext, ";") {
			if e = strings.TrimSpace(e); e != "" {
				exts = append(exts, strings.ToLower(e))
			}
		}
	} else {
		exts = []string{""}
	}
	var found []string
	seen := map[string]bool{}
	for _, dir := range dirs {
		if dir == "" {
			continue
		}
		for _, ext := range exts {
			p := filepath.Join(dir, name+ext)
			key := p
			if goos == "windows" {
				key = strings.ToLower(p)
			}
			if seen[key] {
				continue
			}
			if isExecutable(p, goos) {
				seen[key] = true
				found = append(found, p)
				break // one hit per directory, like a shell
			}
		}
	}
	return found
}

func isExecutable(p, goos string) bool {
	fi, err := os.Stat(p)
	if err != nil || fi.IsDir() {
		return false
	}
	if goos == "windows" {
		return true
	}
	return fi.Mode().Perm()&0o111 != 0
}

// SplitList splits a PATH value for goos.
func SplitList(value, goos string) []string {
	if goos == "windows" {
		return SplitWindows(value)
	}
	var out []string
	for _, d := range strings.Split(value, ":") {
		if d != "" {
			out = append(out, d)
		}
	}
	return out
}

// SameFile reports whether two paths name the same file on goos.
func SameFile(a, b, goos string) bool {
	if goos == "windows" {
		return strings.EqualFold(filepath.Clean(a), filepath.Clean(b))
	}
	return filepath.Clean(a) == filepath.Clean(b)
}
