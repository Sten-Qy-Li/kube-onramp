package pathenv

import (
	"path"
	"path/filepath"
	"strings"
)

// Markers around the block that kube-onramp adds to shell rc files.
const (
	BeginMarker = "# >>> kube-onramp >>>"
	EndMarker   = "# <<< kube-onramp <<<"
)

// Block returns the rc-file block that puts dir first on PATH. A dir inside
// home is written as "$HOME/...", so the file stays readable and portable.
//
// The prepend is unconditional (no "already on PATH?" check): on macOS,
// path_helper in login shells moves inherited entries behind /usr/local/bin,
// so only prepending again in every shell keeps kube-onramp first.
func Block(dir, home string) string {
	value := shellDoubleQuote(dir)
	if home != "" {
		if rel, err := filepath.Rel(home, dir); err == nil && rel != "." && !strings.HasPrefix(rel, "..") {
			value = "$HOME/" + shellDoubleQuote(path.Clean(filepath.ToSlash(rel)))
		}
	}
	return BeginMarker + "\n" +
		"# Added by kube-onramp. Undo with: kube-onramp reset\n" +
		`export PATH="` + value + `:$PATH"` + "\n" +
		EndMarker + "\n"
}

// EnsureBlock returns content with exactly one block, at the end of the file
// (so it comes after lines like `eval "$(brew shellenv)"`). If that is
// already true, content is returned unchanged.
func EnsureBlock(content, block string) (string, bool) {
	rest, _ := RemoveBlock(content)
	want := appendBlock(rest, block)
	return want, want != content
}

// RemoveBlock removes every kube-onramp block, plus the blank line that
// EnsureBlock put before it. Other lines are not changed.
func RemoveBlock(content string) (string, bool) {
	changed := false
	for {
		begin := markerLine(content, BeginMarker, 0)
		if begin < 0 {
			return content, changed
		}
		end := markerLine(content, EndMarker, begin)
		if end < 0 {
			// A begin marker without an end: remove only that line.
			end = begin
		}
		stop := strings.IndexByte(content[end:], '\n')
		if stop < 0 {
			stop = len(content)
		} else {
			stop += end + 1
		}
		start := begin
		if start >= 2 && content[start-1] == '\n' && content[start-2] == '\n' {
			start-- // the blank line added by appendBlock
		}
		content = content[:start] + content[stop:]
		changed = true
	}
}

// HasBlock reports whether content has a kube-onramp block.
func HasBlock(content string) bool { return markerLine(content, BeginMarker, 0) >= 0 }

func appendBlock(rest, block string) string {
	switch {
	case rest == "":
		return block
	case strings.HasSuffix(rest, "\n"):
		return rest + "\n" + block
	default:
		return rest + "\n\n" + block
	}
}

// markerLine returns the index of the line that is exactly marker (ignoring
// surrounding spaces and a CR), searching from 'from'; -1 if none.
func markerLine(content, marker string, from int) int {
	i := from
	for i <= len(content) {
		end := strings.IndexByte(content[i:], '\n')
		line := content[i:]
		if end >= 0 {
			line = content[i : i+end]
		}
		if strings.TrimSpace(line) == marker {
			return i
		}
		if end < 0 {
			return -1
		}
		i += end + 1
	}
	return -1
}

// shellDoubleQuote escapes s for use inside double quotes in sh/zsh/bash.
func shellDoubleQuote(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "$", `\$`, "`", "\\`")
	return r.Replace(s)
}
