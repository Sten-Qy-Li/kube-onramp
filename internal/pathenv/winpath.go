// Package pathenv changes PATH so that new terminals find kube-onramp's bin
// folder first: the user PATH in the Windows registry, or a marked block in
// the shell's rc file on macOS. The string editing is pure and tested; the OS
// access lives in small platform files.
package pathenv

import (
	"strings"
)

// EnsureFirst returns a Windows PATH value with dir as the first entry.
// Other copies of dir are removed; every other entry is kept exactly as it was
// (including empty entries and unexpanded %VARIABLES%). expand expands
// environment variables for comparison; it may be nil.
func EnsureFirst(value, dir string, expand func(string) string) (string, bool) {
	entries := splitWindows(value)
	var kept []string
	own := 0
	for _, e := range entries {
		if SameWindowsDir(expandEntry(e, expand), dir) {
			own++
			continue
		}
		kept = append(kept, e)
	}
	if own == 1 && len(entries) > 0 && SameWindowsDir(expandEntry(entries[0], expand), dir) {
		return value, false
	}
	if len(kept) == 0 {
		return dir, true
	}
	return dir + ";" + strings.Join(kept, ";"), true
}

// RemoveDir returns a Windows PATH value without dir. Other entries are kept
// exactly as they were.
func RemoveDir(value, dir string, expand func(string) string) (string, bool) {
	entries := splitWindows(value)
	var kept []string
	for _, e := range entries {
		if SameWindowsDir(expandEntry(e, expand), dir) {
			continue
		}
		kept = append(kept, e)
	}
	if len(kept) == len(entries) {
		return value, false
	}
	return strings.Join(kept, ";"), true
}

// SplitWindows splits a PATH value into non-empty directories.
func SplitWindows(value string) []string {
	var out []string
	for _, e := range splitWindows(value) {
		if e = strings.Trim(strings.TrimSpace(e), `"`); e != "" {
			out = append(out, e)
		}
	}
	return out
}

// SameWindowsDir compares two Windows directories: case-insensitive, with
// either slash, and ignoring a trailing slash.
func SameWindowsDir(a, b string) bool {
	norm := func(s string) string {
		s = strings.Trim(strings.TrimSpace(s), `"`)
		s = strings.ReplaceAll(s, "/", `\`)
		s = strings.TrimRight(s, `\`)
		return strings.ToLower(s)
	}
	na := norm(a)
	return na != "" && na == norm(b)
}

func splitWindows(value string) []string {
	if value == "" {
		return nil
	}
	return strings.Split(value, ";")
}

func expandEntry(e string, expand func(string) string) string {
	if expand == nil {
		return e
	}
	return expand(e)
}
