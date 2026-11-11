package pathenv

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const bin = `C:\Users\test\AppData\Local\kube-onramp\bin`

func expandFake(s string) string {
	return strings.NewReplacer("%LOCALAPPDATA%", `C:\Users\test\AppData\Local`, "%USERPROFILE%", `C:\Users\test`).Replace(s)
}

func TestEnsureFirst(t *testing.T) {
	long := strings.Repeat(`C:\Some\Very\Long\Directory\Name;`, 300) + `C:\last`
	tests := []struct {
		name, in, want string
		changed        bool
	}{
		{"empty", "", bin, true},
		{"prepend", `%USERPROFILE%\go\bin;C:\tools`, bin + `;%USERPROFILE%\go\bin;C:\tools`, true},
		{"already first", bin + `;C:\tools`, bin + `;C:\tools`, false},
		{"already first, other spelling", strings.ToUpper(bin) + `\;C:\tools`, strings.ToUpper(bin) + `\;C:\tools`, false},
		{"already first as variable", `%LOCALAPPDATA%\kube-onramp\bin;C:\tools`, `%LOCALAPPDATA%\kube-onramp\bin;C:\tools`, false},
		{"move to front", `C:\tools;` + bin, bin + `;C:\tools`, true},
		{"remove duplicates", bin + `;C:\tools;` + bin + `\`, bin + `;C:\tools`, true},
		{"keep empty entries and quotes", `C:\a;;"C:\Program Files\b";`, bin + `;C:\a;;"C:\Program Files\b";`, true},
		{"similar but different dir", bin + `2;C:\x`, bin + `;` + bin + `2;C:\x`, true},
		{"very long PATH is never truncated", long, bin + ";" + long, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, changed := EnsureFirst(tt.in, bin, expandFake)
			if got != tt.want || changed != tt.changed {
				t.Errorf("EnsureFirst(%q)\n got %q, %v\nwant %q, %v", tt.in, got, changed, tt.want, tt.changed)
			}
			// Idempotent: a second call changes nothing.
			if again, ch := EnsureFirst(got, bin, expandFake); ch || again != got {
				t.Errorf("second EnsureFirst changed the value: %q", again)
			}
		})
	}
}

func TestRemoveDir(t *testing.T) {
	tests := []struct {
		name, in, want string
		changed        bool
	}{
		{"only ours", bin, "", true},
		{"ours first", bin + `;C:\tools;%USERPROFILE%\go\bin`, `C:\tools;%USERPROFILE%\go\bin`, true},
		{"ours as variable", `%LOCALAPPDATA%\kube-onramp\bin;C:\tools`, `C:\tools`, true},
		{"not there", `C:\tools;;C:\x`, `C:\tools;;C:\x`, false},
		{"empty", "", "", false},
	}
	for _, tt := range tests {
		got, changed := RemoveDir(tt.in, bin, expandFake)
		if got != tt.want || changed != tt.changed {
			t.Errorf("%s: RemoveDir(%q) = %q, %v; want %q, %v", tt.name, tt.in, got, changed, tt.want, tt.changed)
		}
	}
	// EnsureFirst then RemoveDir gives the original value back.
	orig := `%USERPROFILE%\go\bin;C:\tools;`
	added, _ := EnsureFirst(orig, bin, expandFake)
	if back, _ := RemoveDir(added, bin, expandFake); back != orig {
		t.Errorf("round trip = %q, want %q", back, orig)
	}
}

func TestBlock(t *testing.T) {
	got := Block("/Users/test/.kube-onramp/bin", "/Users/test")
	want := "# >>> kube-onramp >>>\n# Added by kube-onramp. Undo with: kube-onramp reset\nexport PATH=\"$HOME/.kube-onramp/bin:$PATH\"\n# <<< kube-onramp <<<\n"
	if got != want {
		t.Errorf("Block =\n%s\nwant\n%s", got, want)
	}
	if got := Block(`/opt/my "tools"/$bin`, "/Users/test"); !strings.Contains(got, `export PATH="/opt/my \"tools\"/\$bin:$PATH"`) {
		t.Errorf("Block outside home = %q", got)
	}
}

func TestEnsureAndRemoveBlock(t *testing.T) {
	block := Block("/Users/test/.kube-onramp/bin", "/Users/test")
	brew := "eval \"$(/usr/local/bin/brew shellenv)\"\n"
	tests := []struct {
		name, in, want string
		changed        bool
	}{
		{"empty file", "", block, true},
		{"append after content", brew, brew + "\n" + block, true},
		{"no final newline", "alias k=kubectl", "alias k=kubectl\n\n" + block, true},
		{"already at end", brew + "\n" + block, brew + "\n" + block, false},
		{"move to end", block + brew, brew + "\n" + block, true},
		{"update old block", brew + "\n" + strings.Replace(block, "$HOME", "/old", 1), brew + "\n" + block, true},
		{"two blocks become one", brew + "\n" + block + "\n" + block, brew + "\n" + block, true},
		{"CRLF markers", strings.ReplaceAll(block, "\n", "\r\n") + brew, brew + "\n" + block, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, changed := EnsureBlock(tt.in, block)
			if got != tt.want || changed != tt.changed {
				t.Errorf("EnsureBlock(%q)\n got %q, %v\nwant %q, %v", tt.in, got, changed, tt.want, tt.changed)
			}
			if again, ch := EnsureBlock(got, block); ch || again != got {
				t.Errorf("second EnsureBlock changed the file: %q", again)
			}
		})
	}
	// Insert then remove restores the original exactly.
	for _, orig := range []string{"", brew, brew + "\n", "export A=1\n\n"} {
		added, _ := EnsureBlock(orig, block)
		back, changed := RemoveBlock(added)
		if back != orig || !changed {
			t.Errorf("round trip of %q = %q (changed %v)", orig, back, changed)
		}
	}
	if _, changed := RemoveBlock(brew); changed {
		t.Error("RemoveBlock changed a file without a block")
	}
	if got, _ := RemoveBlock("a\n" + BeginMarker + "\nb\n"); got != "a\nb\n" {
		t.Errorf("begin marker without end: %q", got)
	}
}

func TestFindAll(t *testing.T) {
	d1, d2, d3 := t.TempDir(), t.TempDir(), t.TempDir()
	write := func(dir, name string, mode os.FileMode) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), mode); err != nil {
			t.Fatal(err)
		}
	}
	if runtime.GOOS == "windows" {
		write(d1, "kubectl.cmd", 0o644)
		write(d2, "kubectl.exe", 0o644)
		write(d3, "kubectl", 0o644) // no extension: not runnable on Windows
		got := FindAll([]string{d3, d1, d2}, "kubectl", "windows", ".EXE;.CMD")
		want := []string{filepath.Join(d1, "kubectl.cmd"), filepath.Join(d2, "kubectl.exe")}
		if strings.Join(got, "|") != strings.Join(want, "|") {
			t.Errorf("FindAll = %q, want %q", got, want)
		}
		return
	}
	write(d1, "kubectl", 0o644) // not executable
	write(d2, "kubectl", 0o755)
	write(d3, "kubectl", 0o755)
	got := FindAll([]string{d1, d2, "", d3, d2}, "kubectl", runtime.GOOS, "")
	want := []string{filepath.Join(d2, "kubectl"), filepath.Join(d3, "kubectl")}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("FindAll = %q, want %q", got, want)
	}
}
